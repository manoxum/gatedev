package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func RegisterHotspotRoutes(mux *http.ServeMux, worker *workerapi.Client, admin *auth.Administrator, audit *audit.Client, db *sql.DB) {
	mux.HandleFunc("GET /api/hotspot/config", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		config, err := store.GetHotspotConfig(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}))

	mux.HandleFunc("PATCH /api/hotspot/config", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if err := store.SaveHotspotConfig(r.Context(), db, config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "config_changed", username, map[string]any{"section": "hotspot"})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/hotspot/interfaces", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var interfaces json.RawMessage
		if err := worker.Call(r.Context(), http.MethodGet, "/network/interfaces", nil, &interfaces); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(interfaces)
	}))

	mux.HandleFunc("POST /api/hotspot/apply", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		if err := applyHotspotRuntimeConfig(r.Context(), db, worker); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		iface, err := currentHotspotInterface(r.Context(), db)
		if err == nil {
			reapplyHotspotBlocklist(r.Context(), db, worker, iface)
			reapplyHotspotShaping(r.Context(), worker, iface)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/start", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, err := currentHotspotInterface(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// O worker desgerencia a placa fisica no NetworkManager (quando
		// nao ha STA associada) internamente, bem em cima do "docker exec
		// ... start" - ver unmanageWifiInterfaceIfIdle em
		// services/worker/controller/compose.go. Fazer essa checagem
		// aqui tambem, bem mais cedo, so alargava a janela entre a
		// checagem e a tentativa real do create_ap (o hotspot ainda leva
		// alguns segundos pra rodar de fato: ensureHotspotContainer,
		// restart do dns-provider, espera do banco), dando tempo de sobra
		// pra uma associacao Wi-Fi marginal cair entre as duas.
		// O container do hotspot fica vivo; ligar/desligar controla apenas o
		// servico AP interno. A configuracao operacional e lida pelo proprio
		// hotspot na tabela hotspot_config, no momento do start/restart.
		if err := startHotspotRuntimeConfig(r.Context(), db, worker); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		reapplyHotspotBlocklist(r.Context(), db, worker, iface)
		reapplyHotspotShaping(r.Context(), worker, iface)
		if err := store.SetHotspotDesiredState(r.Context(), db, true); err != nil {
			log.Printf("[backend] falha ao gravar estado desejado do hotspot (ligado): %v", err)
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "hotspot_started", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/stop", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, ifaceErr := currentHotspotInterface(r.Context(), db)
		if err := stopHotspotService(r.Context(), worker); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if ifaceErr == nil {
			// wifi-manage e idempotente (ver handleWifiManage no worker)
			// mesmo quando o /start anterior nao chegou a desgerenciar a
			// placa (cenario AP+STA, ver unmanageWifiInterfaceIfIdle em
			// services/worker/controller/compose.go) - chamar sempre aqui
			// garante que a placa nunca fique presa "unmanaged" no
			// NetworkManager depois que o hotspot para.
			if err := worker.Call(r.Context(), http.MethodPost, "/network/wifi-manage", map[string]string{"interface": iface}, nil); err != nil {
				log.Printf("[backend] aviso: falha ao devolver %s ao NetworkManager: %v", iface, err)
			}
		}
		if err := store.SetHotspotDesiredState(r.Context(), db, false); err != nil {
			log.Printf("[backend] falha ao gravar estado desejado do hotspot (parado): %v", err)
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "hotspot_stopped", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/recover-wifi", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		iface, err := currentHotspotInterface(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := recoverWifiAdapter(r.Context(), worker, iface); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "wifi_adapter_recovered", username, map[string]any{"interface": iface})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/hotspot/status", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var status struct {
			Running   bool   `json:"running"`
			Status    string `json:"status"`
			StartedAt string `json:"startedAt"`
		}
		if err := worker.Call(r.Context(), http.MethodGet, "/hotspot/status", nil, &status); err != nil {
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
			if err := worker.CallText(r.Context(), "/containers/hotspot/logs?tail=200", &logs); err == nil {
				channel, band := parseHotspotChannelBand(logs.String())
				if channel != "" {
					response["channel"] = channel
				}
				if band != "" {
					response["band"] = band
				}
				if internetInterface := parseHotspotInternetInterface(logs.String()); internetInterface != "" {
					response["internetInterface"] = internetInterface
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))

	mux.HandleFunc("GET /api/hotspot/clients", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		clients, err := listEnrichedHotspotClients(r, db, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(clients)
	}))

	RegisterHotspotLogsRoutes(mux, worker, admin, audit, db)
	RegisterHotspotUplinkRoute(mux, admin, audit, db)
}

func applyHotspotRuntimeConfig(ctx context.Context, db *sql.DB, worker *workerapi.Client) error {
	config, err := store.HotspotRuntimeConfig(ctx, db)
	if err != nil {
		return err
	}
	return worker.Call(ctx, http.MethodPost, "/hotspot/apply", config, nil)
}

func startHotspotRuntimeConfig(ctx context.Context, db *sql.DB, worker *workerapi.Client) error {
	config, err := store.HotspotRuntimeConfig(ctx, db)
	if err != nil {
		return err
	}
	return worker.Call(ctx, http.MethodPost, "/hotspot/start", config, nil)
}
