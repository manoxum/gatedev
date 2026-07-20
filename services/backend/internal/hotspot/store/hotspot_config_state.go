package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func HotspotWifiInterface(ctx context.Context, db *sql.DB) (string, error) {
	config, err := GetHotspotConfig(ctx, db)
	if err != nil {
		return "", err
	}
	iface := strings.TrimSpace(config["WIFI_INTERFACE"])
	if iface == "" {
		return "", errors.New("WIFI_INTERFACE nao configurado pelo painel")
	}
	return iface, nil
}

// hotspotDesiredStateKey guarda a ultima intencao do admin
// (ligar/desligar, ver POST /api/hotspot/start e /stop) na mesma
// tabela hotspot_config, mas fora de hotspotConfigKeys - de proposito,
// pra nao aparecer em GET /api/hotspot/config nem poder ser
// sobrescrita via PATCH (SaveHotspotConfig rejeita chaves fora da
// allowlist). Usada por AutoStartHotspotOnBoot em
// hotspot_autostart.go pra decidir se religa o hotspot sozinho quando
// o backend reinicia, e por recoverHotspotIfDesired em
// hotspot_reconcile.go pra decidir se religa sozinho quando o hotspot
// cai sem pedido do admin (ex.: watchdog de falha de beacon).
const hotspotDesiredStateKey = "_DESIRED_STATE"

func SetHotspotDesiredState(ctx context.Context, db *sql.DB, running bool) error {
	value := "stopped"
	if running {
		value = "running"
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO hotspot_config (key, value, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = CURRENT_TIMESTAMP
	`, hotspotDesiredStateKey, value)
	return err
}

func HotspotDesiredStateRunning(ctx context.Context, db *sql.DB) (bool, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM hotspot_config WHERE key = $1`, hotspotDesiredStateKey).Scan(&value)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value == "running", nil
}
