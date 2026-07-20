package dns

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/platform/config"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

func RegisterDNSRoutes(mux *http.ServeMux, worker *workerapi.Client, admin *auth.Administrator, audit *audit.Client, db *sql.DB) {
	mux.HandleFunc("GET /api/dns/node", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		cfg, err := getDNSConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fingerprint, err := loadLocalNodeFingerprint(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		nodeName := strings.TrimSpace(cfg[KeyNodeName])
		if nodeName == "" {
			nodeName = "este-servidor"
		}
		response := map[string]any{
			"nodeName":    nodeName,
			"fingerprint": fingerprint,
			"domains":     parsePeerList(cfg[KeyDomains]),
			"port":        discoverPort(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))

	mux.HandleFunc("GET /api/dns/config", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		cfg, err := getDNSConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		peers, err := loadConfiguredPeerAddresses(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg["DISCOVER_CONFIGURED_PEERS"] = strings.Join(peers, ",")
		// DISCOVER_PORT segue vindo do ambiente (porta de infraestrutura),
		// mas continua no mesmo payload para o frontend nao precisar saber
		// de onde cada valor vem.
		cfg["DISCOVER_PORT"] = discoverPort()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	}))

	mux.HandleFunc("PATCH /api/dns/config", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var cfg map[string]string
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if tlds, ok := cfg[KeyLocalTLDs]; ok {
			if err := validateLocalTLDs(tlds); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if domains, ok := cfg[KeyDomains]; ok {
			if err := validateDomains(domains); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if peers, ok := cfg["DISCOVER_CONFIGURED_PEERS"]; ok {
			if err := replaceConfiguredPeerAddresses(r.Context(), db, parsePeerList(peers)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			delete(cfg, "DISCOVER_CONFIGURED_PEERS")
		}
		if peers, ok := cfg["DISCOVER_PEERS"]; ok {
			if err := replaceConfiguredPeerAddresses(r.Context(), db, parsePeerList(peers)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			delete(cfg, "DISCOVER_PEERS")
		}
		// DISCOVER_PORT nao e editavel pelo painel (vem do compose); ignora
		// em vez de recusar o PATCH inteiro, para um payload antigo do
		// frontend nao quebrar o salvamento das outras chaves.
		delete(cfg, "DISCOVER_PORT")
		if len(cfg) > 0 {
			if err := saveDNSConfig(r.Context(), db, cfg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "config_changed", username, map[string]any{"section": "dns"})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/dns/apply", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		hotspotConfig, err := store.GetHotspotConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := worker.Call(r.Context(), http.MethodPost, "/dns/apply", hotspotConfig, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "dns_applied", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/dns/logs", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = worker.StreamLogs(r.Context(), w, "dns-provider", r.URL.Query().Get("follow") == "true", "")
	}))

	mux.HandleFunc("GET /api/dns/discovered-servers", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discoveredServersFromNginx(config.Getenv("NGINX_CONFIG_PATH", nginxConfigPath)))
	}))

	mux.HandleFunc("POST /api/dns/test", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hostname string `json:"hostname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hostname == "" {
			http.Error(w, "campo 'hostname' obrigatorio", http.StatusBadRequest)
			return
		}
		hotspotConfig, err := store.GetHotspotConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var result json.RawMessage
		body := map[string]string{"hostname": req.Hostname, "server": hotspotConfig["HOTSPOT_GATEWAY"]}
		if err := worker.Call(r.Context(), http.MethodPost, "/network/dns-test", body, &result); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(result)
	}))
}

func loadLocalNodeFingerprint(ctx context.Context, db *sql.DB) (string, error) {
	var fingerprint string
	err := db.QueryRowContext(ctx, `SELECT fingerprint FROM discover_node_identity WHERE id = 1`).Scan(&fingerprint)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return fingerprint, err
}

func parsePeerList(raw string) []string {
	var peers []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' }) {
		peer := strings.TrimSpace(part)
		if peer == "" || seen[peer] {
			continue
		}
		seen[peer] = true
		peers = append(peers, peer)
	}
	return peers
}
