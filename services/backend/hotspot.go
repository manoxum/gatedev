package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

var (
	channelRegex = regexp.MustCompile(`Canal automatico escolhido: (\d+)`)
	bandRegex    = regexp.MustCompile(`Banda Wi-Fi automatica escolhida: ([\d.]+)GHz`)
)

func registerHotspotRoutes(mux *http.ServeMux, worker *workerClient, admin *administrator, audit *auditClient) {
	mux.HandleFunc("GET /api/hotspot/config", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := worker.call(r.Context(), http.MethodGet, "/env?section=hotspot", nil, &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}))

	mux.HandleFunc("PATCH /api/hotspot/config", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if password, ok := config["WIFI_PASSWORD"]; ok && len(password) < 8 {
			http.Error(w, "WIFI_PASSWORD deve ter ao menos 8 caracteres (requisito WPA2)", http.StatusBadRequest)
			return
		}
		if err := worker.call(r.Context(), http.MethodPatch, "/env?section=hotspot", config, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "config_changed", username, map[string]any{"section": "hotspot"})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/hotspot/interfaces", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var interfaces json.RawMessage
		if err := worker.call(r.Context(), http.MethodGet, "/network/interfaces", nil, &interfaces); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(interfaces)
	}))

	mux.HandleFunc("POST /api/hotspot/apply", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		if err := worker.call(r.Context(), http.MethodPost, "/hotspot/apply", nil, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/start", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, err := currentHotspotInterface(r, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if err := worker.call(r.Context(), http.MethodPost, "/network/wifi-unmanage", map[string]string{"interface": iface}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		// Usa "docker compose up" (via /hotspot/apply) em vez de "docker
		// start": cria os containers se ainda nao existirem (1ª subida) e
		// tambem os recria se o .env mudou desde a ultima vez - e a mesma
		// operacao que scripts/hotspot-on.sh fazia, agora só pelo painel.
		if err := worker.call(r.Context(), http.MethodPost, "/hotspot/apply", nil, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "hotspot_started", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/stop", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, err := currentHotspotInterface(r, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		for _, service := range []string{"hotspot", "dns-provider"} {
			if err := worker.call(r.Context(), http.MethodPost, "/containers/"+service+"/stop", nil, nil); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
		if err := worker.call(r.Context(), http.MethodPost, "/network/wifi-manage", map[string]string{"interface": iface}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "hotspot_stopped", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/hotspot/status", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var status struct {
			Running   bool   `json:"running"`
			Status    string `json:"status"`
			StartedAt string `json:"startedAt"`
		}
		if err := worker.call(r.Context(), http.MethodGet, "/containers/hotspot/status", nil, &status); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		response := map[string]any{
			"running":   status.Running,
			"status":    status.Status,
			"startedAt": status.StartedAt,
		}
		if status.Running {
			var logs strings.Builder
			if err := worker.callText(r.Context(), "/containers/hotspot/logs?tail=200", &logs); err == nil {
				if m := channelRegex.FindStringSubmatch(logs.String()); m != nil {
					response["channel"] = m[1]
				}
				if m := bandRegex.FindStringSubmatch(logs.String()); m != nil {
					response["band"] = m[1]
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))

	mux.HandleFunc("GET /api/hotspot/clients", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, err := currentHotspotInterface(r, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		var clients json.RawMessage
		if err := worker.call(r.Context(), http.MethodGet, "/hotspot/clients?interface="+iface, nil, &clients); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(clients)
	}))

	mux.HandleFunc("GET /api/hotspot/logs", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = worker.streamLogs(r.Context(), w, "hotspot", r.URL.Query().Get("follow") == "true")
	}))
}

// currentHotspotInterface busca WIFI_INTERFACE configurado no .env - usada
// tanto para ligar/desligar o hotspot quanto para listar clientes.
func currentHotspotInterface(r *http.Request, worker *workerClient) (string, error) {
	var config map[string]string
	if err := worker.call(r.Context(), http.MethodGet, "/env?section=hotspot", nil, &config); err != nil {
		return "", err
	}
	iface := config["WIFI_INTERFACE"]
	if iface == "" {
		return "", errors.New("WIFI_INTERFACE nao configurado")
	}
	return iface, nil
}
