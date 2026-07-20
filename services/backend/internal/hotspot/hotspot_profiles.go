// hotspot_profiles.go expoe as rotas HTTP de perfis de dispositivo -
// bundle nomeado e reutilizavel de limites de trafego + politica de
// credito que um dispositivo herda por padrao (ver
// hotspot_profiles_apply.go para a resolucao override > perfil >
// global). Nao confundir com o "perfil" heuristico de identificacao
// (inferHotspotDeviceProfile em hotspot_device_identify.go) - sao
// conceitos distintos que so compartilham o nome em portugues.
package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/workerapi"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type hotspotProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
	hotspotLimits
	CreditRechargeAmountBytes *int64  `json:"creditRechargeAmountBytes"`
	CreditRechargePeriod      *string `json:"creditRechargePeriod"`
	CreditPlafondBytes        *int64  `json:"creditPlafondBytes"`
}

type hotspotProfileRequest struct {
	Name string `json:"name"`
	hotspotLimits
	CreditRechargeAmountBytes *int64  `json:"creditRechargeAmountBytes"`
	CreditRechargePeriod      *string `json:"creditRechargePeriod"`
	CreditPlafondBytes        *int64  `json:"creditPlafondBytes"`
}

type hotspotDeviceProfileRequest struct {
	ProfileID string `json:"profileId"`
}

func RegisterHotspotProfileRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, worker *workerapi.Client, audit *audit.Client) {
	mux.HandleFunc("GET /api/hotspot/profiles", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		profiles, err := listProfiles(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(profiles)
	}))

	mux.HandleFunc("POST /api/hotspot/profiles", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req hotspotProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, "campo 'name' obrigatorio", http.StatusBadRequest)
			return
		}
		if req.LimitType != "" && !isValidLimitType(req.LimitType, true) {
			http.Error(w, "campo 'limitType' invalido", http.StatusBadRequest)
			return
		}
		profile, err := insertProfile(db, req)
		if errors.Is(err, errHotspotProfileNameTaken) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "profile_created", username, map[string]any{"id": profile.ID, "name": profile.Name})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(profile)
	}))

	mux.HandleFunc("GET /api/hotspot/profiles/{id}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		profile, found, err := getProfile(db, r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "perfil nao encontrado", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(profile)
	}))

	mux.HandleFunc("PATCH /api/hotspot/profiles/{id}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req hotspotProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, "campo 'name' obrigatorio", http.StatusBadRequest)
			return
		}
		if req.LimitType != "" && !isValidLimitType(req.LimitType, true) {
			http.Error(w, "campo 'limitType' invalido", http.StatusBadRequest)
			return
		}
		profile, found, err := updateProfile(db, id, req)
		if errors.Is(err, errHotspotProfileNameTaken) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "perfil nao encontrado", http.StatusNotFound)
			return
		}
		applyProfileShapingLive(r.Context(), db, worker, id)
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "profile_updated", username, map[string]any{"id": id, "name": profile.Name})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(profile)
	}))

	mux.HandleFunc("DELETE /api/hotspot/profiles/{id}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := deleteProfile(db, id); errors.Is(err, errHotspotProfileIsDefault) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "profile_deleted", username, map[string]any{"id": id})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("PATCH /api/hotspot/devices/{mac}/profile", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		var req hotspotDeviceProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProfileID == "" {
			http.Error(w, "campo 'profileId' obrigatorio", http.StatusBadRequest)
			return
		}
		if _, found, err := getProfile(db, req.ProfileID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, "perfil nao encontrado", http.StatusBadRequest)
			return
		}
		if err := assignDeviceProfile(db, mac, req.ProfileID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyDeviceShapingLive(r.Context(), db, worker, mac)
		if _, err := syncDeviceCreditFromProfile(r.Context(), db, worker, mac); err != nil {
			// best-effort - o vinculo ja foi persistido, so a
			// sincronizacao imediata do credito falhou (ex.: worker
			// inacessivel); o proximo ciclo de reconciliacao cobre isso.
			log.Printf("[backend] perfil do dispositivo %s persistido, mas sincronizacao de credito falhou: %v", mac, err)
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "device_profile_assigned", username, map[string]any{"mac": mac, "profileId": req.ProfileID})
		w.WriteHeader(http.StatusNoContent)
	}))
}
