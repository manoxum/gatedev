package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

func registerDNSRoutes(mux *http.ServeMux, worker *workerClient, admin *administrator, audit *auditClient, db *sql.DB) {
	mux.HandleFunc("GET /api/dns/node", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := worker.call(r.Context(), http.MethodGet, "/env?section=dns", nil, &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		fingerprint, err := loadLocalNodeFingerprint(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		nodeName := strings.TrimSpace(config["DISCOVER_NODE_NAME"])
		if nodeName == "" {
			nodeName = "este-servidor"
		}
		port := strings.TrimSpace(config["DISCOVER_PORT"])
		if port == "" {
			port = "8531"
		}
		response := map[string]any{
			"nodeName":    nodeName,
			"fingerprint": fingerprint,
			"domains":     parsePeerList(config["DOMAINS"]),
			"port":        port,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))

	mux.HandleFunc("GET /api/dns/config", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := worker.call(r.Context(), http.MethodGet, "/env?section=dns", nil, &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		peers, err := loadConfiguredPeerAddresses(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		config["DISCOVER_CONFIGURED_PEERS"] = strings.Join(peers, ",")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}))

	mux.HandleFunc("PATCH /api/dns/config", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if tlds, ok := config["DNS_LOCAL_TLDS"]; ok {
			if err := validateLocalTLDs(tlds); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if domains, ok := config["DOMAINS"]; ok {
			if err := validateDomains(domains); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if peers, ok := config["DISCOVER_CONFIGURED_PEERS"]; ok {
			if err := replaceConfiguredPeerAddresses(r.Context(), db, parsePeerList(peers)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			delete(config, "DISCOVER_CONFIGURED_PEERS")
		}
		if peers, ok := config["DISCOVER_PEERS"]; ok {
			if err := replaceConfiguredPeerAddresses(r.Context(), db, parsePeerList(peers)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			delete(config, "DISCOVER_PEERS")
		}
		if len(config) > 0 {
			if err := worker.call(r.Context(), http.MethodPatch, "/env?section=dns", config, nil); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "config_changed", username, map[string]any{"section": "dns"})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/dns/apply", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		hotspotConfig, err := getHotspotConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := worker.call(r.Context(), http.MethodPost, "/dns/apply", hotspotConfig, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "dns_applied", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/dns/logs", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = worker.streamLogs(r.Context(), w, "dns-provider", r.URL.Query().Get("follow") == "true", "")
	}))

	mux.HandleFunc("GET /api/dns/discovered-servers", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discoveredServersFromNginx(getenv("NGINX_CONFIG_PATH", nginxConfigPath)))
	}))

	mux.HandleFunc("POST /api/dns/test", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hostname string `json:"hostname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hostname == "" {
			http.Error(w, "campo 'hostname' obrigatorio", http.StatusBadRequest)
			return
		}
		hotspotConfig, err := getHotspotConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var result json.RawMessage
		body := map[string]string{"hostname": req.Hostname, "server": hotspotConfig["HOTSPOT_GATEWAY"]}
		if err := worker.call(r.Context(), http.MethodPost, "/network/dns-test", body, &result); err != nil {
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
