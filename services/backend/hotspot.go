package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
)

var (
	channelRegex = regexp.MustCompile(`Canal automatico escolhido: (\d+)`)
	bandRegex    = regexp.MustCompile(`Banda Wi-Fi automatica escolhida: ([\d.]+)GHz`)
)

func registerHotspotRoutes(mux *http.ServeMux, worker *workerClient, admin *administrator, audit *auditClient, db *sql.DB) {
	mux.HandleFunc("GET /api/hotspot/config", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		config, err := getHotspotConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		if err := saveHotspotConfig(r.Context(), db, config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
		if err := applyHotspotRuntimeConfig(r.Context(), db, worker); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		iface, err := currentHotspotInterface(r.Context(), db)
		if err == nil {
			reapplyHotspotBlocklist(r.Context(), db, worker, iface)
			reapplyHotspotShaping(r.Context(), db, worker, iface)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/start", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, err := currentHotspotInterface(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// O container do hotspot fica vivo; ligar/desligar controla apenas o
		// servico AP interno. A configuracao operacional e lida pelo proprio
		// hotspot na tabela hotspot_config, no momento do start/restart.
		if err := startHotspotRuntimeConfig(r.Context(), db, worker); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		reapplyHotspotBlocklist(r.Context(), db, worker, iface)
		reapplyHotspotShaping(r.Context(), db, worker, iface)
		if err := setHotspotDesiredState(r.Context(), db, true); err != nil {
			log.Printf("[backend] falha ao gravar estado desejado do hotspot (ligado): %v", err)
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "hotspot_started", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/stop", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		if err := stopHotspotService(r.Context(), worker); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if err := setHotspotDesiredState(r.Context(), db, false); err != nil {
			log.Printf("[backend] falha ao gravar estado desejado do hotspot (parado): %v", err)
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "hotspot_stopped", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/recover-wifi", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, err := currentHotspotInterface(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := recoverWifiAdapter(r.Context(), worker, iface); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "wifi_adapter_recovered", username, map[string]any{"interface": iface})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/hotspot/status", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var status struct {
			Running   bool   `json:"running"`
			Status    string `json:"status"`
			StartedAt string `json:"startedAt"`
		}
		if err := worker.call(r.Context(), http.MethodGet, "/hotspot/status", nil, &status); err != nil {
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
		clients, err := listEnrichedHotspotClients(r, db, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(clients)
	}))

	mux.HandleFunc("GET /api/hotspot/logs", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = worker.streamLogs(r.Context(), w, "hotspot", r.URL.Query().Get("follow") == "true")
	}))
}

func applyHotspotRuntimeConfig(ctx context.Context, db *sql.DB, worker *workerClient) error {
	config, err := hotspotRuntimeConfig(ctx, db)
	if err != nil {
		return err
	}
	return worker.call(ctx, http.MethodPost, "/hotspot/apply", config, nil)
}

func startHotspotRuntimeConfig(ctx context.Context, db *sql.DB, worker *workerClient) error {
	config, err := hotspotRuntimeConfig(ctx, db)
	if err != nil {
		return err
	}
	return worker.call(ctx, http.MethodPost, "/hotspot/start", config, nil)
}

// currentHotspotInterface busca WIFI_INTERFACE configurado pelo painel - usada
// tanto para ligar/desligar o hotspot quanto para listar clientes.
func currentHotspotInterface(ctx context.Context, db *sql.DB) (string, error) {
	return hotspotWifiInterface(ctx, db)
}

func stopHotspotService(ctx context.Context, worker *workerClient) error {
	return worker.call(ctx, http.MethodPost, "/hotspot/stop", nil, nil)
}

func recoverWifiAdapter(ctx context.Context, worker *workerClient, iface string) error {
	if err := stopHotspotService(ctx, worker); err != nil {
		return err
	}
	return worker.call(ctx, http.MethodPost, "/network/wifi-manage", map[string]string{"interface": iface}, nil)
}
