// hotspot_isolation.go expoe as rotas HTTP do isolamento de clientes:
// o interruptor geral (chave CLIENT_ISOLATION em hotspot_config -
// mudar exige reiniciar o hotspot, o ap_isolate do hostapd nao e
// ajustavel em runtime) e o CRUD das regras de comunicacao
// (hotspot_comm_rules). A avaliacao de precedencia mora em
// hotspot_isolation_policy.go; a aplicacao ao vivo em
// hotspot_isolation_apply.go.
package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"database/sql"
	"encoding/json"
	"net/http"
)

type isolationStateRequest struct {
	Enabled bool `json:"enabled"`
}

type isolationStateResponse struct {
	Enabled bool `json:"enabled"`
	// O interruptor so vale de verdade no proximo start do hotspot
	// (ap_isolate e fixado no hostapd.conf pelo create_ap) - o painel
	// avisa o operador; nunca reiniciamos o hotspot automaticamente.
	RestartRequired bool `json:"restartRequired"`
}

// normalizeCommRuleRefs valida e normaliza as referencias da regra:
// MAC canonico nas extremidades device, perfil existente nas
// extremidades profile. Devolve mensagem vazia quando valida.
func normalizeCommRuleRefs(db *sql.DB, req *store.CommRuleRequest) string {
	if req.SourceKind == store.CommEndpointDevice {
		mac, err := normalizeHotspotMAC(req.SourceRef)
		if err != nil {
			return "mac de origem invalido"
		}
		req.SourceRef = mac
	}
	if req.TargetKind == store.CommEndpointDevice {
		mac, err := normalizeHotspotMAC(*req.TargetRef)
		if err != nil {
			return "mac de destino invalido"
		}
		req.TargetRef = &mac
	}
	for _, ref := range commRuleProfileRefs(req) {
		if _, found, err := store.GetProfile(db, ref); err != nil {
			return err.Error()
		} else if !found {
			return "perfil nao encontrado: " + ref
		}
	}
	return ""
}

func commRuleProfileRefs(req *store.CommRuleRequest) []string {
	refs := []string{}
	if req.SourceKind == store.CommEndpointProfile {
		refs = append(refs, req.SourceRef)
	}
	if req.TargetKind == store.CommEndpointProfile && req.TargetRef != nil {
		refs = append(refs, *req.TargetRef)
	}
	return refs
}

func decodeCommRuleRequest(w http.ResponseWriter, r *http.Request, db *sql.DB) (store.CommRuleRequest, bool) {
	var req store.CommRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return req, false
	}
	store.NormalizeCommRuleDefaults(&req)
	if err := store.ValidateCommRuleShape(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return req, false
	}
	if message := normalizeCommRuleRefs(db, &req); message != "" {
		http.Error(w, message, http.StatusBadRequest)
		return req, false
	}
	// Revalida apos a normalizacao do MAC: "AA-BB..." e "aa:bb..."
	// viram o mesmo MAC canonico e nao podem formar regra consigo mesmo.
	if err := store.ValidateCommRuleShape(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func RegisterHotspotIsolationRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, worker *workerapi.Client, audit *audit.Client) {
	mux.HandleFunc("GET /api/hotspot/isolation", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		enabled, err := isolationEnabled(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(isolationStateResponse{Enabled: enabled})
	}))

	mux.HandleFunc("PUT /api/hotspot/isolation", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req isolationStateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		value := "false"
		if req.Enabled {
			value = "true"
		}
		if err := store.SaveHotspotConfig(r.Context(), db, map[string]string{"CLIENT_ISOLATION": value}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyIsolationLive(r.Context(), db, worker)
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "isolation_toggled", username, map[string]any{"enabled": req.Enabled})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(isolationStateResponse{Enabled: req.Enabled, RestartRequired: true})
	}))

	mux.HandleFunc("GET /api/hotspot/isolation/rules", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		rules, err := store.ListCommRules(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rules)
	}))

	mux.HandleFunc("POST /api/hotspot/isolation/rules", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeCommRuleRequest(w, r, db)
		if !ok {
			return
		}
		rule, err := store.InsertCommRule(db, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyIsolationLive(r.Context(), db, worker)
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "comm_rule_created", username, map[string]any{"id": rule.ID, "action": rule.Action})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(rule)
	}))

	mux.HandleFunc("PATCH /api/hotspot/isolation/rules/{id}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeCommRuleRequest(w, r, db)
		if !ok {
			return
		}
		rule, found, err := store.UpdateCommRule(db, r.PathValue("id"), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "regra nao encontrada", http.StatusNotFound)
			return
		}
		applyIsolationLive(r.Context(), db, worker)
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "comm_rule_updated", username, map[string]any{"id": rule.ID, "action": rule.Action})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rule)
	}))

	mux.HandleFunc("DELETE /api/hotspot/isolation/rules/{id}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		found, err := store.DeleteCommRule(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "regra nao encontrada", http.StatusNotFound)
			return
		}
		applyIsolationLive(r.Context(), db, worker)
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "comm_rule_deleted", username, map[string]any{"id": id})
		w.WriteHeader(http.StatusNoContent)
	}))
}
