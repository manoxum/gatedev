package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// hotspotDeviceTraffic guarda so o que e genuinamente singular por
// dispositivo: o fwmark (classe HTB/marca iptables dedicada) e os
// contadores absolutos usados para calcular o delta a cada ciclo de
// reconciliacao. O acumulado por periodo de cota (antes period_start/
// period_end/download_bytes/upload_bytes/throttled aqui) mudou pra uma
// linha por (mac, tipo de periodo) em hotspot_device_quota_periods -
// ver hotspot_device_quota_store.go e o comentario no schema.prisma.
type hotspotDeviceTraffic struct {
	MACAddress          string
	Fwmark              int
	LastDownloadCounter int64
	LastUploadCounter   int64
}

type hotspotGlobalTraffic struct {
	PeriodStart         time.Time
	PeriodEnd           time.Time
	DownloadBytes       int64
	UploadBytes         int64
	LastDownloadCounter int64
	LastUploadCounter   int64
	Throttled           bool
}

type hotspotTrafficResponse struct {
	DownloadBytes int64   `json:"downloadBytes"`
	UploadBytes   int64   `json:"uploadBytes"`
	PeriodStart   string  `json:"periodStart"`
	PeriodEnd     string  `json:"periodEnd"`
	Throttled     bool    `json:"throttled"`
	QuotaBytes    *int64  `json:"quotaBytes"`
	QuotaPeriod   *string `json:"quotaPeriod"`
}

// registerHotspotTrafficRoutes so cobre o acumulado global daqui pra
// frente - o equivalente por dispositivo virou o endpoint de cota por
// periodo (GET /api/hotspot/devices/{mac}/quota, ver
// hotspot_device_quota.go), que ja tinha tudo que este endpoint
// devolvia (bytes usados + teto) e ainda cobre os 3 periodos
// simultaneos em vez de um so.
func registerHotspotTrafficRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB) {
	mux.HandleFunc("GET /api/hotspot/limits/global/traffic", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		traffic, err := ensureGlobalTrafficRow(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		limits, err := getGlobalLimits(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(globalTrafficResponse(traffic, limits))
	}))
}

func globalTrafficResponse(traffic hotspotGlobalTraffic, limits hotspotGlobalLimits) hotspotTrafficResponse {
	return hotspotTrafficResponse{
		DownloadBytes: traffic.DownloadBytes,
		UploadBytes:   traffic.UploadBytes,
		PeriodStart:   traffic.PeriodStart.Format(time.RFC3339),
		PeriodEnd:     traffic.PeriodEnd.Format(time.RFC3339),
		Throttled:     traffic.Throttled,
		QuotaBytes:    limits.QuotaBytes,
		QuotaPeriod:   limits.QuotaPeriod,
	}
}

// getOrCreateDeviceFwmark garante que o dispositivo tenha uma linha em
// hotspot_device_traffic (criada de forma preguicosa, independente de
// limite configurado) e devolve o fwmark atribuido pela sequence -
// nunca hash de MAC, evita colisao.
func getOrCreateDeviceFwmark(db *sql.DB, mac string) (int, error) {
	traffic, err := ensureDeviceTrafficRow(db, mac)
	if err != nil {
		return 0, err
	}
	return traffic.Fwmark, nil
}

func ensureDeviceTrafficRow(db *sql.DB, mac string) (hotspotDeviceTraffic, error) {
	var t hotspotDeviceTraffic
	err := db.QueryRow(`
		INSERT INTO hotspot_device_traffic (mac_address)
		VALUES ($1)
		ON CONFLICT (mac_address) DO UPDATE SET mac_address = EXCLUDED.mac_address
		RETURNING mac_address, fwmark, last_download_counter, last_upload_counter
	`, mac).Scan(&t.MACAddress, &t.Fwmark, &t.LastDownloadCounter, &t.LastUploadCounter)
	return t, err
}

func ensureGlobalTrafficRow(db *sql.DB) (hotspotGlobalTraffic, error) {
	var t hotspotGlobalTraffic
	err := db.QueryRow(`
		INSERT INTO hotspot_global_traffic (id)
		VALUES ('global')
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING period_start, period_end, download_bytes, upload_bytes,
		          last_download_counter, last_upload_counter, throttled
	`).Scan(&t.PeriodStart, &t.PeriodEnd, &t.DownloadBytes, &t.UploadBytes,
		&t.LastDownloadCounter, &t.LastUploadCounter, &t.Throttled)
	return t, err
}
