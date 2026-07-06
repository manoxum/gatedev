package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// rateUnit e a unidade de uma taxa configurada pelo admin: bits/s
// (Kb/Mb/Gb na UI, "kbit"/"mbit"/"gbit" no valor - mesmo sufixo que tc
// usa) ou bytes/s (KB/MB/GB na UI, "kbyte"/"mbyte"/"gbyte" no valor -
// o worker traduz para os sufixos tc kbps/mbps/gbps). Ver rate() em
// services/worker/controller/shaping_tc.go.
type rateUnit = string

const (
	rateUnitKbit  rateUnit = "kbit"
	rateUnitMbit  rateUnit = "mbit"
	rateUnitGbit  rateUnit = "gbit"
	rateUnitKbyte rateUnit = "kbyte"
	rateUnitMbyte rateUnit = "mbyte"
	rateUnitGbyte rateUnit = "gbyte"
)

// hotspotLimits representa tanto o limite global (singleton) quanto o
// limite de um dispositivo especifico - mesmo shape de colunas em
// hotspot_global_limits/hotspot_device_limits. Campos de valor nil =
// "sem limite desse tipo" (a unidade correspondente e ignorada nesse
// caso, mas sempre vem preenchida pelo banco - default "mbit").
type hotspotLimits struct {
	DownloadRateValue          *int     `json:"downloadRateValue"`
	DownloadRateUnit           rateUnit `json:"downloadRateUnit"`
	UploadRateValue            *int     `json:"uploadRateValue"`
	UploadRateUnit             rateUnit `json:"uploadRateUnit"`
	QuotaBytes                 *int64   `json:"quotaBytes"`
	QuotaPeriod                *string  `json:"quotaPeriod"`
	QuotaThrottleDownloadValue *int     `json:"quotaThrottleDownloadValue"`
	QuotaThrottleDownloadUnit  rateUnit `json:"quotaThrottleDownloadUnit"`
	QuotaThrottleUploadValue   *int     `json:"quotaThrottleUploadValue"`
	QuotaThrottleUploadUnit    rateUnit `json:"quotaThrottleUploadUnit"`
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

// normalizeLimitUnits preenche "mbit" nas unidades que vierem vazias
// no corpo do PATCH (frontend antigo, ou campo omitido) - garante que
// nunca violamos o CHECK de unidade nem gravamos "" no Postgres.
func normalizeLimitUnits(limits hotspotLimits) hotspotLimits {
	normalize := func(unit rateUnit) rateUnit {
		if unit == "" {
			return rateUnitMbit
		}
		return unit
	}
	limits.DownloadRateUnit = normalize(limits.DownloadRateUnit)
	limits.UploadRateUnit = normalize(limits.UploadRateUnit)
	limits.QuotaThrottleDownloadUnit = normalize(limits.QuotaThrottleDownloadUnit)
	limits.QuotaThrottleUploadUnit = normalize(limits.QuotaThrottleUploadUnit)
	return limits
}

func getGlobalLimits(db *sql.DB) (hotspotLimits, error) {
	var limits hotspotLimits
	err := db.QueryRow(`
		SELECT download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		       quota_bytes, quota_period,
		       quota_throttle_download_value, quota_throttle_download_unit,
		       quota_throttle_upload_value, quota_throttle_upload_unit
		FROM hotspot_global_limits WHERE id = 'global'
	`).Scan(&limits.DownloadRateValue, &limits.DownloadRateUnit, &limits.UploadRateValue, &limits.UploadRateUnit,
		&limits.QuotaBytes, &limits.QuotaPeriod,
		&limits.QuotaThrottleDownloadValue, &limits.QuotaThrottleDownloadUnit,
		&limits.QuotaThrottleUploadValue, &limits.QuotaThrottleUploadUnit)
	if err != nil && err != sql.ErrNoRows {
		return hotspotLimits{}, err
	}
	return normalizeLimitUnits(limits), nil
}

func upsertGlobalLimits(db *sql.DB, limits hotspotLimits) error {
	limits = normalizeLimitUnits(limits)
	_, err := db.Exec(`
		INSERT INTO hotspot_global_limits (id, download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		                                    quota_bytes, quota_period,
		                                    quota_throttle_download_value, quota_throttle_download_unit,
		                                    quota_throttle_upload_value, quota_throttle_upload_unit, updated_at)
		VALUES ('global', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE
		SET download_rate_value = EXCLUDED.download_rate_value,
		    download_rate_unit = EXCLUDED.download_rate_unit,
		    upload_rate_value = EXCLUDED.upload_rate_value,
		    upload_rate_unit = EXCLUDED.upload_rate_unit,
		    quota_bytes = EXCLUDED.quota_bytes,
		    quota_period = EXCLUDED.quota_period,
		    quota_throttle_download_value = EXCLUDED.quota_throttle_download_value,
		    quota_throttle_download_unit = EXCLUDED.quota_throttle_download_unit,
		    quota_throttle_upload_value = EXCLUDED.quota_throttle_upload_value,
		    quota_throttle_upload_unit = EXCLUDED.quota_throttle_upload_unit,
		    updated_at = CURRENT_TIMESTAMP
	`, limits.DownloadRateValue, limits.DownloadRateUnit, limits.UploadRateValue, limits.UploadRateUnit,
		limits.QuotaBytes, limits.QuotaPeriod,
		limits.QuotaThrottleDownloadValue, limits.QuotaThrottleDownloadUnit,
		limits.QuotaThrottleUploadValue, limits.QuotaThrottleUploadUnit)
	return err
}

func getDeviceLimits(db *sql.DB, mac string) (hotspotLimits, bool, error) {
	var limits hotspotLimits
	err := db.QueryRow(`
		SELECT download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		       quota_bytes, quota_period,
		       quota_throttle_download_value, quota_throttle_download_unit,
		       quota_throttle_upload_value, quota_throttle_upload_unit
		FROM hotspot_device_limits WHERE mac_address = $1
	`, mac).Scan(&limits.DownloadRateValue, &limits.DownloadRateUnit, &limits.UploadRateValue, &limits.UploadRateUnit,
		&limits.QuotaBytes, &limits.QuotaPeriod,
		&limits.QuotaThrottleDownloadValue, &limits.QuotaThrottleDownloadUnit,
		&limits.QuotaThrottleUploadValue, &limits.QuotaThrottleUploadUnit)
	if err == sql.ErrNoRows {
		return hotspotLimits{}, false, nil
	}
	if err != nil {
		return hotspotLimits{}, false, err
	}
	return normalizeLimitUnits(limits), true, nil
}

func upsertDeviceLimits(db *sql.DB, mac string, limits hotspotLimits) error {
	limits = normalizeLimitUnits(limits)
	_, err := db.Exec(`
		INSERT INTO hotspot_device_limits (mac_address, download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		                                    quota_bytes, quota_period,
		                                    quota_throttle_download_value, quota_throttle_download_unit,
		                                    quota_throttle_upload_value, quota_throttle_upload_unit, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET download_rate_value = EXCLUDED.download_rate_value,
		    download_rate_unit = EXCLUDED.download_rate_unit,
		    upload_rate_value = EXCLUDED.upload_rate_value,
		    upload_rate_unit = EXCLUDED.upload_rate_unit,
		    quota_bytes = EXCLUDED.quota_bytes,
		    quota_period = EXCLUDED.quota_period,
		    quota_throttle_download_value = EXCLUDED.quota_throttle_download_value,
		    quota_throttle_download_unit = EXCLUDED.quota_throttle_download_unit,
		    quota_throttle_upload_value = EXCLUDED.quota_throttle_upload_value,
		    quota_throttle_upload_unit = EXCLUDED.quota_throttle_upload_unit,
		    updated_at = CURRENT_TIMESTAMP
	`, mac, limits.DownloadRateValue, limits.DownloadRateUnit, limits.UploadRateValue, limits.UploadRateUnit,
		limits.QuotaBytes, limits.QuotaPeriod,
		limits.QuotaThrottleDownloadValue, limits.QuotaThrottleDownloadUnit,
		limits.QuotaThrottleUploadValue, limits.QuotaThrottleUploadUnit)
	return err
}

func deleteDeviceLimits(db *sql.DB, mac string) error {
	_, err := db.Exec(`DELETE FROM hotspot_device_limits WHERE mac_address = $1`, mac)
	return err
}
