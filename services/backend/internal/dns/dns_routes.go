// dns_routes.go expoe a tabela de descoberta (roteamento por proximo
// salto) que o dns-provider mantem na tabela discover_routes -
// leitura e remocao manual de rotas paradas, nunca criacao/edicao
// manual (a tabela e recalculada a cada ciclo de troca com os peers,
// ver services/worker/dns/discover_peer.go).
package dns

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"database/sql"
	"encoding/json"
	"net/http"
)

type discoverRoute struct {
	Domain           string `json:"domain"`
	Owner            string `json:"owner"`
	OwnerFingerprint string `json:"ownerFingerprint,omitempty"`
	NextHop          string `json:"nextHop"`
	Distance         int    `json:"distance"`
	Source           string `json:"source"`
	State            string `json:"state"`
	LastSeenAt       string `json:"lastSeenAt"`
}

func RegisterDNSRouteRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, audit *audit.Client) {
	mux.HandleFunc("GET /api/dns/routes", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(), `
			SELECT domain, owner, COALESCE(owner_fingerprint, ''), next_hop, distance, source, state, last_seen_at
			FROM discover_routes ORDER BY domain
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		routes := []discoverRoute{}
		for rows.Next() {
			var route discoverRoute
			if err := rows.Scan(&route.Domain, &route.Owner, &route.OwnerFingerprint, &route.NextHop, &route.Distance, &route.Source, &route.State, &route.LastSeenAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			routes = append(routes, route)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(routes)
	}))

	mux.HandleFunc("DELETE /api/dns/routes/{domain}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		domain := r.PathValue("domain")
		if domain == "" {
			http.Error(w, "dominio obrigatorio", http.StatusBadRequest)
			return
		}
		if _, err := db.ExecContext(r.Context(), `DELETE FROM discover_routes WHERE domain = $1`, domain); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "discover_route_removed", username, map[string]any{"domain": domain})
		w.WriteHeader(http.StatusNoContent)
	}))
}
