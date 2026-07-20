package hotspot

import (
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"database/sql"
	"encoding/json"
	"net/http"
)

// hotspotDeviceLimitsResponse e a resposta do GET - sempre os limites
// EFETIVOS (herdados do perfil, ou o override proprio do dispositivo
// quando o perfil e "custom" - ver effectiveDeviceLimits), acompanhados
// do nome/tipo do perfil vinculado para o frontend decidir se mostra um
// resumo so-leitura ("herdado do perfil X") ou o formulario editavel
// (so quando ProfileLimitType == "custom").
type hotspotDeviceLimitsResponse struct {
	store.Limits
	ProfileName      string          `json:"profileName"`
	ProfileLimitType store.LimitType `json:"profileLimitType"`
}

func RegisterHotspotDeviceLimitRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, worker *workerapi.Client) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/limits", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
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
		profileID, err := deviceProfileID(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profile, _, err := store.GetProfile(db, profileID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hotspotDeviceLimitsResponse{
			Limits:           limits,
			ProfileName:      profile.Name,
			ProfileLimitType: profile.LimitType,
		})
	}))

	mux.HandleFunc("PATCH /api/hotspot/devices/{mac}/limits", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		var limits store.Limits
		if err := json.NewDecoder(r.Body).Decode(&limits); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if limits.LimitType != "" && !store.IsValidLimitType(limits.LimitType, false) {
			http.Error(w, "campo 'limitType' invalido", http.StatusBadRequest)
			return
		}
		profileID, err := deviceProfileID(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profile, _, err := store.GetProfile(db, profileID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if profile.LimitType != store.LimitTypeCustom {
			http.Error(w, "so e possivel definir um limite proprio quando o perfil vinculado e 'customizado'", http.StatusConflict)
			return
		}
		if err := store.UpsertDeviceLimits(db, mac, limits); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyDeviceShapingLive(r.Context(), db, worker, mac)
		w.WriteHeader(http.StatusNoContent)
	}))
}
