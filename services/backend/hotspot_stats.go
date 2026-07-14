package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
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

type hotspotClientStatsEntry struct {
	MAC string `json:"mac"`
	hotspotDeviceStatsResponse
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

	mux.HandleFunc("GET /api/hotspot/clients/stats", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		entries, err := allClientsLiveStats(r.Context(), db, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))

	mux.HandleFunc("GET /api/hotspot/stats/global", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		response, err := globalLiveStats(r.Context(), worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

// globalLiveStats devolve a taxa agregada de todo o hotspot (nao um
// dispositivo) desde a ultima leitura - mesmo cache em memoria
// (computeLiveRate) que trackedDeviceRate usa por MAC, so que sob a
// chave sentinela globalSpeedHistoryKey. Ao contrario de
// trackedDeviceRate, nao chama ensureDeviceShaping: a contagem global
// (mangle "bn-global-*") ja fica de pe via applyGlobalShaping desde que
// o hotspot sobe (reapplyHotspotShaping), sem depender de nenhum
// dispositivo especifico estar conectado.
func globalLiveStats(ctx context.Context, worker *workerClient) (hotspotDeviceStatsResponse, error) {
	download, upload, err := readDeviceShapingStats(ctx, worker, "")
	if err != nil {
		return hotspotDeviceStatsResponse{}, err
	}
	return computeLiveRate(globalSpeedHistoryKey, download, upload), nil
}

// deviceLiveStats garante a regra de contagem do dispositivo (se ele
// estiver conectado agora) e devolve a taxa calculada desde a ultima
// leitura - devolve zero, sem erro, se o dispositivo nao estiver
// conectado (nada para medir ainda).
func deviceLiveStats(ctx context.Context, db *sql.DB, worker *workerClient, mac string) (hotspotDeviceStatsResponse, error) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		return hotspotDeviceStatsResponse{}, err
	}
	ip, found := liveHotspotClientIP(ctx, worker, iface, mac)
	if !found {
		return hotspotDeviceStatsResponse{}, nil
	}
	return trackedDeviceRate(ctx, db, worker, iface, mac, ip)
}

// allClientsLiveStats calcula a taxa de todos os clientes conectados
// agora numa unica passada (uma so listagem de clientes ao worker, em
// vez de N requisicoes separadas como a pagina de detalhe faria) -
// usada pela tela de clientes conectados, que mostra um velocimetro
// por linha.
func allClientsLiveStats(ctx context.Context, db *sql.DB, worker *workerClient) ([]hotspotClientStatsEntry, error) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		return nil, err
	}
	clients, err := liveHotspotClients(ctx, worker, iface)
	if err != nil {
		return nil, err
	}
	entries := make([]hotspotClientStatsEntry, 0, len(clients))
	for _, client := range clients {
		rate, err := trackedDeviceRate(ctx, db, worker, iface, client.MAC, client.IP)
		if err != nil {
			log.Printf("[backend] falha ao ler velocidade de %s: %v", client.MAC, err)
			continue
		}
		entries = append(entries, hotspotClientStatsEntry{MAC: client.MAC, hotspotDeviceStatsResponse: rate})
	}
	return entries, nil
}

// trackedDeviceRate garante a regra de contagem do dispositivo e
// devolve a taxa calculada desde a ultima leitura.
func trackedDeviceRate(ctx context.Context, db *sql.DB, worker *workerClient, iface, mac, ip string) (hotspotDeviceStatsResponse, error) {
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
