// hotspot_device_speed_history.go guarda, em memoria do proprio
// processo, uma amostra de velocidade (bytes/s) por dispositivo a cada
// ciclo de reconciliacao - alimenta o grafico de linha do detalhe do
// dispositivo (janela selecionavel, de 1 min a 1 dia). Deliberadamente
// nao persistido (Postgres/Mongo): nao ha motivo pra sobreviver a um
// restart do backend, e o stack roda como instalacao unica (sem
// multiplas replicas do backend, poucos dispositivos) - estado em
// memoria de processo e seguro e barato aqui, mesma logica que ja vale
// pra nao usar Redis como fonte de verdade (ver RULE.md). Sem
// downsampling de proposito: 1 amostra/s por 24h sao ~86400 pontos por
// MAC rastreado, tranquilo pra memoria numa instalacao unica - o
// frontend e quem reduz a cadencia de poll pra janelas longas (ver
// useDeviceSpeedHistory em useHotspotQueries.ts), nao o backend.
package hotspot

import (
	"bindnet/backend/internal/auth"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const deviceSpeedHistoryRetention = 24 * time.Hour

// globalSpeedHistoryKey e a chave sentinela usada pra guardar a
// amostra de velocidade GLOBAL (todo trafego do hotspot, nao um
// dispositivo) no mesmo deviceSpeedHistoryStore usado por dispositivo -
// nunca colide com um MAC real (normalizeHotspotMAC nunca devolve esse
// formato). Gravada por reconcileGlobalUsage (hotspot_usage_sampling.go),
// lida pela rota /api/hotspot/limits/global/speed-history abaixo.
const globalSpeedHistoryKey = "__global__"

// validSpeedHistoryWindows: 1/5/10/15/30 min, 1/6/12h, 1 dia - mesmas
// opcoes do seletor de janela em DeviceSpeedChart.tsx/
// HotspotGlobalSpeedPanel.tsx.
var validSpeedHistoryWindows = map[int]bool{
	1: true, 5: true, 10: true, 15: true, 30: true,
	60: true, 360: true, 720: true, 1440: true,
}

type speedSample struct {
	At          time.Time `json:"at"`
	DownloadBps float64   `json:"downloadBps"`
	UploadBps   float64   `json:"uploadBps"`
}

type macSpeedHistory struct {
	lastAt  time.Time
	samples []speedSample
}

// deviceSpeedHistoryStore e seguro para uso concorrente: o loop de
// reconciliacao escreve (goroutine unica, sequencial por dispositivo)
// enquanto handlers HTTP leem (uma goroutine por requisicao).
type deviceSpeedHistoryStore struct {
	mu      sync.Mutex
	history map[string]*macSpeedHistory
}

func NewDeviceSpeedHistoryStore() *deviceSpeedHistoryStore {
	return &deviceSpeedHistoryStore{history: map[string]*macSpeedHistory{}}
}

// record calcula a taxa a partir do tempo real decorrido desde a
// ultima amostra deste MAC (nao de um intervalo fixo assumido - mais
// robusto a ciclos de reconciliacao que atrasam). A primeira amostra de
// um MAC nunca tem uma taxa valida ainda (sem baseline) - so guarda o
// instante, sem plotar ponto nenhum. Aproveita a chamada pra podar
// amostras e MACs inteiros que passaram da retencao, evitando
// crescimento sem limite do mapa em instalacoes de longa duracao.
func (s *deviceSpeedHistoryStore) record(mac string, deltaDownloadBytes, deltaUploadBytes int64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	h, found := s.history[mac]
	if !found {
		h = &macSpeedHistory{}
		s.history[mac] = h
	}
	if !h.lastAt.IsZero() {
		elapsed := at.Sub(h.lastAt).Seconds()
		if elapsed > 0 {
			h.samples = append(h.samples, speedSample{
				At:          at,
				DownloadBps: float64(deltaDownloadBytes) / elapsed,
				UploadBps:   float64(deltaUploadBytes) / elapsed,
			})
		}
	}
	h.lastAt = at

	cutoff := at.Add(-deviceSpeedHistoryRetention)
	h.samples = pruneOldSamples(h.samples, cutoff)
	for m, entry := range s.history {
		if entry.lastAt.Before(cutoff) {
			delete(s.history, m)
		}
	}
}

func pruneOldSamples(samples []speedSample, cutoff time.Time) []speedSample {
	firstValid := 0
	for firstValid < len(samples) && samples[firstValid].At.Before(cutoff) {
		firstValid++
	}
	return samples[firstValid:]
}

// snapshot devolve uma copia das amostras dentro da janela pedida (do
// mais antigo pro mais recente, mesma ordem cronologica ja usada por
// SessionConsumptionChart.tsx no frontend).
func (s *deviceSpeedHistoryStore) snapshot(mac string, window time.Duration) []speedSample {
	s.mu.Lock()
	defer s.mu.Unlock()

	h, found := s.history[mac]
	if !found {
		return []speedSample{}
	}
	cutoff := time.Now().Add(-window)
	result := make([]speedSample, 0, len(h.samples))
	for _, sample := range h.samples {
		if !sample.At.Before(cutoff) {
			result = append(result, sample)
		}
	}
	return result
}

func RegisterHotspotDeviceSpeedHistoryRoutes(mux *http.ServeMux, admin *auth.Administrator, speedHistory *deviceSpeedHistoryStore) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/speed-history", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		minutes := 15
		if raw := r.URL.Query().Get("minutes"); raw != "" {
			parsed, convErr := parseSpeedHistoryWindow(raw)
			if convErr != nil {
				http.Error(w, "parametro 'minutes' invalido (use 1, 5, 10, 15, 30, 60, 360, 720 ou 1440)", http.StatusBadRequest)
				return
			}
			minutes = parsed
		}
		samples := speedHistory.snapshot(mac, time.Duration(minutes)*time.Minute)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(samples)
	}))

	mux.HandleFunc("GET /api/hotspot/limits/global/speed-history", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		minutes := 15
		if raw := r.URL.Query().Get("minutes"); raw != "" {
			parsed, convErr := parseSpeedHistoryWindow(raw)
			if convErr != nil {
				http.Error(w, "parametro 'minutes' invalido (use 1, 5, 10, 15, 30, 60, 360, 720 ou 1440)", http.StatusBadRequest)
				return
			}
			minutes = parsed
		}
		samples := speedHistory.snapshot(globalSpeedHistoryKey, time.Duration(minutes)*time.Minute)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(samples)
	}))
}

var errInvalidSpeedHistoryWindow = errors.New("janela invalida")

func parseSpeedHistoryWindow(raw string) (int, error) {
	minutes, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if !validSpeedHistoryWindows[minutes] {
		return 0, errInvalidSpeedHistoryWindow
	}
	return minutes, nil
}
