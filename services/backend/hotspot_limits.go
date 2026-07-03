package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// hotspotLimits representa tanto o limite global (singleton) quanto o
// limite de um dispositivo especifico - mesmo shape de colunas em
// hotspot_global_limits/hotspot_device_limits. Campos nil = "sem
// limite desse tipo".
type hotspotLimits struct {
	DownloadRateMbps          *int    `json:"downloadRateMbps"`
	UploadRateMbps            *int    `json:"uploadRateMbps"`
	QuotaBytes                *int64  `json:"quotaBytes"`
	QuotaPeriod               *string `json:"quotaPeriod"`
	QuotaThrottleDownloadMbps *int    `json:"quotaThrottleDownloadMbps"`
	QuotaThrottleUploadMbps   *int    `json:"quotaThrottleUploadMbps"`
}

func registerHotspotLimitRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, worker *workerClient) {
	mux.HandleFunc("GET /api/hotspot/limits/global", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		limits, err := getGlobalLimits(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(limits)
	}))

	mux.HandleFunc("PATCH /api/hotspot/limits/global", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var limits hotspotLimits
		if err := json.NewDecoder(r.Body).Decode(&limits); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if err := upsertGlobalLimits(db, limits); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyGlobalShapingLive(r.Context(), db, worker)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/hotspot/devices/{mac}/limits", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		limits, _, err := getDeviceLimits(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(limits)
	}))

	mux.HandleFunc("PATCH /api/hotspot/devices/{mac}/limits", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		var limits hotspotLimits
		if err := json.NewDecoder(r.Body).Decode(&limits); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if err := upsertDeviceLimits(db, mac, limits); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyDeviceShapingLive(r.Context(), db, worker, mac)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("DELETE /api/hotspot/devices/{mac}/limits", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		if err := deleteDeviceLimits(db, mac); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyDeviceShapingLive(r.Context(), db, worker, mac)
		w.WriteHeader(http.StatusNoContent)
	}))
}

func getGlobalLimits(db *sql.DB) (hotspotLimits, error) {
	var limits hotspotLimits
	err := db.QueryRow(`
		SELECT download_rate_mbps, upload_rate_mbps, quota_bytes, quota_period,
		       quota_throttle_download_mbps, quota_throttle_upload_mbps
		FROM hotspot_global_limits WHERE id = 'global'
	`).Scan(&limits.DownloadRateMbps, &limits.UploadRateMbps, &limits.QuotaBytes, &limits.QuotaPeriod,
		&limits.QuotaThrottleDownloadMbps, &limits.QuotaThrottleUploadMbps)
	if err != nil && err != sql.ErrNoRows {
		return hotspotLimits{}, err
	}
	return limits, nil
}

func upsertGlobalLimits(db *sql.DB, limits hotspotLimits) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_global_limits (id, download_rate_mbps, upload_rate_mbps, quota_bytes, quota_period,
		                                    quota_throttle_download_mbps, quota_throttle_upload_mbps, updated_at)
		VALUES ('global', $1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE
		SET download_rate_mbps = EXCLUDED.download_rate_mbps,
		    upload_rate_mbps = EXCLUDED.upload_rate_mbps,
		    quota_bytes = EXCLUDED.quota_bytes,
		    quota_period = EXCLUDED.quota_period,
		    quota_throttle_download_mbps = EXCLUDED.quota_throttle_download_mbps,
		    quota_throttle_upload_mbps = EXCLUDED.quota_throttle_upload_mbps,
		    updated_at = CURRENT_TIMESTAMP
	`, limits.DownloadRateMbps, limits.UploadRateMbps, limits.QuotaBytes, limits.QuotaPeriod,
		limits.QuotaThrottleDownloadMbps, limits.QuotaThrottleUploadMbps)
	return err
}

func getDeviceLimits(db *sql.DB, mac string) (hotspotLimits, bool, error) {
	var limits hotspotLimits
	err := db.QueryRow(`
		SELECT download_rate_mbps, upload_rate_mbps, quota_bytes, quota_period,
		       quota_throttle_download_mbps, quota_throttle_upload_mbps
		FROM hotspot_device_limits WHERE mac_address = $1
	`, mac).Scan(&limits.DownloadRateMbps, &limits.UploadRateMbps, &limits.QuotaBytes, &limits.QuotaPeriod,
		&limits.QuotaThrottleDownloadMbps, &limits.QuotaThrottleUploadMbps)
	if err == sql.ErrNoRows {
		return hotspotLimits{}, false, nil
	}
	if err != nil {
		return hotspotLimits{}, false, err
	}
	return limits, true, nil
}

func upsertDeviceLimits(db *sql.DB, mac string, limits hotspotLimits) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_device_limits (mac_address, download_rate_mbps, upload_rate_mbps, quota_bytes, quota_period,
		                                    quota_throttle_download_mbps, quota_throttle_upload_mbps, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET download_rate_mbps = EXCLUDED.download_rate_mbps,
		    upload_rate_mbps = EXCLUDED.upload_rate_mbps,
		    quota_bytes = EXCLUDED.quota_bytes,
		    quota_period = EXCLUDED.quota_period,
		    quota_throttle_download_mbps = EXCLUDED.quota_throttle_download_mbps,
		    quota_throttle_upload_mbps = EXCLUDED.quota_throttle_upload_mbps,
		    updated_at = CURRENT_TIMESTAMP
	`, mac, limits.DownloadRateMbps, limits.UploadRateMbps, limits.QuotaBytes, limits.QuotaPeriod,
		limits.QuotaThrottleDownloadMbps, limits.QuotaThrottleUploadMbps)
	return err
}

func deleteDeviceLimits(db *sql.DB, mac string) error {
	_, err := db.Exec(`DELETE FROM hotspot_device_limits WHERE mac_address = $1`, mac)
	return err
}
