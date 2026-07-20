package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/workerapi"
	"database/sql"
	"encoding/json"
	"net/http"
)

// RegisterHotspotDeviceQuotaRoutes expoe a leitura dos ate 3 periodos
// de cota configurados de um dispositivo e o reset manual por periodo
// (requisito do admin: um botao "Resetar" ao lado de cada cota
// configurada, nao um botao unico que zera tudo).
func RegisterHotspotDeviceQuotaRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, worker *workerapi.Client, audit *audit.Client) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/quota", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		limits, err := effectiveDeviceLimits(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		periods, err := listDeviceQuotaPeriods(db, mac, limits)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(periods)
	}))

	mux.HandleFunc("POST /api/hotspot/devices/{mac}/quota/{period}/reset", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		period := r.PathValue("period")
		if !isValidQuotaPeriodType(period) {
			http.Error(w, "periodo invalido (use daily, weekly ou monthly)", http.StatusBadRequest)
			return
		}
		if err := resetDeviceQuotaPeriodNow(db, mac, period); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Reaplica ao vivo se o dispositivo estiver conectado agora - so
		// resetar 1 periodo nao deve desbloquear o MAC sozinho se outro
		// periodo configurado ainda estiver estourado, por isso reusa o
		// mesmo caminho de reconciliacao em vez de so desbloquear direto.
		applyDeviceQuotaLive(r.Context(), db, worker, mac)
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "device_quota_reset", username, map[string]any{"mac": mac, "period": period})
		w.WriteHeader(http.StatusNoContent)
	}))
}
