package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// liveStatsCache guarda a ultima leitura de contadores por dispositivo
// (em memoria, nunca no Postgres - o poll de 2-3s da pagina de detalhe
// nao deve gerar escrita a cada request; so o loop de reconciliacao
// persiste o acumulado do periodo).
var (
	liveStatsMu    sync.Mutex
	liveStatsCache = map[string]liveStatsEntry{}
)

type liveStatsEntry struct {
	at       time.Time
	download uint64
	upload   uint64
}

type hotspotDeviceStatsResponse struct {
	DownloadBps float64 `json:"downloadBps"`
	UploadBps   float64 `json:"uploadBps"`
}

func registerHotspotStatsRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, worker *workerClient) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/stats", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		response, err := deviceLiveStats(r.Context(), db, worker, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

// deviceLiveStats garante a regra de contagem do dispositivo (se ele
// estiver conectado agora) e devolve a taxa calculada desde a ultima
// leitura - devolve zero, sem erro, se o dispositivo nao estiver
// conectado (nada para medir ainda).
func deviceLiveStats(ctx context.Context, db *sql.DB, worker *workerClient, mac string) (hotspotDeviceStatsResponse, error) {
	iface, err := hotspotWifiInterface(ctx, worker)
	if err != nil {
		return hotspotDeviceStatsResponse{}, err
	}
	ip, found := liveHotspotClientIP(ctx, worker, iface, mac)
	if !found {
		return hotspotDeviceStatsResponse{}, nil
	}
	if err := ensureDeviceShaping(ctx, db, worker, iface, mac, ip); err != nil {
		return hotspotDeviceStatsResponse{}, err
	}
	download, upload, err := readDeviceShapingStats(ctx, worker, mac)
	if err != nil {
		return hotspotDeviceStatsResponse{}, err
	}
	return computeLiveRate(mac, download, upload), nil
}

// computeLiveRate compara a leitura atual com a anterior (cache em
// memoria) e devolve bytes/segundo - primeira leitura de um
// dispositivo sempre devolve zero, pois ainda nao ha uma leitura
// anterior para comparar.
func computeLiveRate(key string, download, upload uint64) hotspotDeviceStatsResponse {
	now := time.Now()
	liveStatsMu.Lock()
	defer liveStatsMu.Unlock()
	previous, found := liveStatsCache[key]
	liveStatsCache[key] = liveStatsEntry{at: now, download: download, upload: upload}
	if !found {
		return hotspotDeviceStatsResponse{}
	}
	elapsed := now.Sub(previous.at).Seconds()
	if elapsed <= 0 {
		return hotspotDeviceStatsResponse{}
	}
	return hotspotDeviceStatsResponse{
		DownloadBps: float64(diffUint64(previous.download, download)) / elapsed,
		UploadBps:   float64(diffUint64(previous.upload, upload)) / elapsed,
	}
}

// diffUint64 trata um contador atual menor que o anterior (regra de
// contagem recriada) como zero de diferenca em vez de "negativo" -
// evita um pico de taxa artificial na leitura seguinte a um reset.
func diffUint64(previous, current uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}
