package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

// currentHotspotInterface busca WIFI_INTERFACE configurado pelo painel -
// usada tanto para ligar/desligar o hotspot quanto para listar clientes.
func currentHotspotInterface(ctx context.Context, db *sql.DB) (string, error) {
	return hotspotWifiInterface(ctx, db)
}

func stopHotspotService(ctx context.Context, worker *workerClient) error {
	return worker.call(ctx, http.MethodPost, "/hotspot/stop", nil, nil)
}

// recoverWifiAdapter para o servico do hotspot e devolve a placa Wi-Fi
// ao NetworkManager - usada tanto por POST /api/hotspot/recover-wifi
// (recuperacao manual) quanto poderia ser reusada por qualquer outro
// fluxo que precise devolver o controle da placa. Desgerenciar a placa
// antes do hotspot subir (quando ela nao esta associada como cliente)
// e feito pelo proprio worker, bem em cima do "docker exec ... start" -
// ver unmanageWifiInterfaceIfIdle em
// services/worker/controller/compose.go.
func recoverWifiAdapter(ctx context.Context, worker *workerClient, iface string) error {
	if err := stopHotspotService(ctx, worker); err != nil {
		return err
	}
	return worker.call(ctx, http.MethodPost, "/network/wifi-manage", map[string]string{"interface": iface}, nil)
}

// registerHotspotUplinkRoute troca SO a fonte de internet
// (INTERNET_INTERFACE) sem reiniciar o hotspot: grava a chave no banco
// e deixa o monitor de uplink do runner
// (services/worker/hotspot/uplink.sh) detectar a mudanca e alternar o
// NAT ao vivo em ate UPLINK_MONITOR_INTERVAL segundos - clientes
// conectados nao caem. Com o hotspot parado, o valor simplesmente vale
// no proximo start. Usada pelo quick-switch do card de resumo do
// painel; o formulario completo ("Salvar e aplicar") continua
// reiniciando, porque as demais chaves (SSID, senha, canal...) exigem
// subir o hostapd de novo.
func registerHotspotUplinkRoute(mux *http.ServeMux, admin *administrator, audit *auditClient, db *sql.DB) {
	mux.HandleFunc("POST /api/hotspot/uplink", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Interface string `json:"interface"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Interface) == "" {
			http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
			return
		}
		if err := saveHotspotConfig(r.Context(), db, map[string]string{"INTERNET_INTERFACE": req.Interface}); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "hotspot_uplink_switched", username, map[string]any{"interface": req.Interface})
		w.WriteHeader(http.StatusNoContent)
	}))
}
