// hotspot_firewall.go expoe as rotas da politica padrao das zonas
// wan/local do firewall (chaves FW_WAN_POLICY/FW_LOCAL_POLICY em
// hotspot_config). O CRUD das regras em si e o das zonas continua em
// hotspot_isolation.go (mesma tabela hotspot_comm_rules); aqui e so o
// default por zona, aplicado ao vivo (sem reiniciar o hotspot).
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

type firewallPolicyState struct {
	WanPolicy   string `json:"wanPolicy"`
	LocalPolicy string `json:"localPolicy"`
}

func RegisterHotspotFirewallRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, worker *workerapi.Client, audit *audit.Client) {
	mux.HandleFunc("GET /api/hotspot/firewall/policy", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		config, err := store.GetHotspotConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallPolicyState{
			WanPolicy:   zonePolicy(config, "FW_WAN_POLICY"),
			LocalPolicy: zonePolicy(config, "FW_LOCAL_POLICY"),
		})
	}))

	mux.HandleFunc("PUT /api/hotspot/firewall/policy", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req firewallPolicyState
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if err := store.SaveHotspotConfig(r.Context(), db, map[string]string{
			"FW_WAN_POLICY":   req.WanPolicy,
			"FW_LOCAL_POLICY": req.LocalPolicy,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		applyFirewallLive(r.Context(), db, worker)
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "firewall_policy_changed", username, map[string]any{
			"wan": req.WanPolicy, "local": req.LocalPolicy,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(req)
	}))
}
