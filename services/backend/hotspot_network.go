package main

import (
	"context"
	"database/sql"
	"net/http"
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
