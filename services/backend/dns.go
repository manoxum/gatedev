package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func registerDNSRoutes(mux *http.ServeMux, worker *workerClient, admin *administrator, audit *auditClient) {
	mux.HandleFunc("GET /api/dns/config", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := worker.call(r.Context(), http.MethodGet, "/env?section=dns", nil, &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}))

	mux.HandleFunc("PATCH /api/dns/config", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if tlds, ok := config["DNS_LOCAL_TLDS"]; ok && strings.TrimSpace(tlds) == "" {
			http.Error(w, "DNS_LOCAL_TLDS nao pode ficar vazio", http.StatusBadRequest)
			return
		}
		if err := worker.call(r.Context(), http.MethodPatch, "/env?section=dns", config, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "config_changed", username, map[string]any{"section": "dns"})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/dns/apply", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		if err := worker.call(r.Context(), http.MethodPost, "/dns/apply", nil, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "dns_applied", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/dns/logs", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = worker.streamLogs(r.Context(), w, "dns-provider", r.URL.Query().Get("follow") == "true")
	}))

	mux.HandleFunc("POST /api/dns/test", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hostname string `json:"hostname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hostname == "" {
			http.Error(w, "campo 'hostname' obrigatorio", http.StatusBadRequest)
			return
		}
		var hotspotConfig map[string]string
		if err := worker.call(r.Context(), http.MethodGet, "/env?section=hotspot", nil, &hotspotConfig); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
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
