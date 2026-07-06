package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var hotspotConfigKeys = []string{
	"WIFI_INTERFACE",
	"INTERNET_INTERFACE",
	"WIFI_SSID",
	"WIFI_PASSWORD",
	"WIFI_COUNTRY",
	"WIFI_CHANNEL",
	"WIFI_FREQ_BAND",
	"WIFI_CHANNEL_CANDIDATES",
	"HOTSPOT_GATEWAY",
	"HOTSPOT_CIDR",
	"HOTSPOT_DNS_FALLBACKS",
	"BINDNET_UPLINK_INTERFACE",
	"UPLINK_MONITOR_INTERVAL",
}

var hotspotConfigDefaults = map[string]string{
	"INTERNET_INTERFACE":       "auto",
	"WIFI_COUNTRY":             "ST",
	"WIFI_CHANNEL":             "auto",
	"WIFI_FREQ_BAND":           "auto",
	"HOTSPOT_GATEWAY":          "192.168.12.1",
	"HOTSPOT_CIDR":             "192.168.12.0/24",
	"HOTSPOT_DNS_FALLBACKS":    "1.1.1.1,8.8.8.8",
	"BINDNET_UPLINK_INTERFACE": "bn-uplink",
	"UPLINK_MONITOR_INTERVAL":  "10",
}

var requiredHotspotRuntimeKeys = []string{
	"WIFI_INTERFACE",
	"INTERNET_INTERFACE",
	"WIFI_SSID",
	"WIFI_PASSWORD",
}

func hotspotConfigAllowedSet() map[string]bool {
	allowed := make(map[string]bool, len(hotspotConfigKeys))
	for _, key := range hotspotConfigKeys {
		allowed[key] = true
	}
	return allowed
}

func getHotspotConfig(ctx context.Context, db *sql.DB) (map[string]string, error) {
	config := make(map[string]string, len(hotspotConfigKeys))
	for key, value := range hotspotConfigDefaults {
		config[key] = value
	}

	rows, err := db.QueryContext(ctx, `SELECT key, value FROM hotspot_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allowed := hotspotConfigAllowedSet()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if allowed[key] {
			config[key] = value
		}
	}
	return config, rows.Err()
}

func saveHotspotConfig(ctx context.Context, db *sql.DB, values map[string]string) error {
	allowed := hotspotConfigAllowedSet()
	clean := make(map[string]string, len(values))
	for key, value := range values {
		if !allowed[key] {
			return fmt.Errorf("chave '%s' nao pode ser alterada na configuracao do hotspot", key)
		}
		clean[key] = strings.TrimSpace(value)
	}
	if password, ok := clean["WIFI_PASSWORD"]; ok && len(password) < 8 {
		return errors.New("WIFI_PASSWORD deve ter ao menos 8 caracteres (requisito WPA2)")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range clean {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO hotspot_config (key, value, updated_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (key) DO UPDATE
			SET value = EXCLUDED.value,
			    updated_at = CURRENT_TIMESTAMP
		`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func hotspotRuntimeConfig(ctx context.Context, db *sql.DB) (map[string]string, error) {
	config, err := getHotspotConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, key := range requiredHotspotRuntimeKeys {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("%s nao configurado pelo painel", key)
		}
	}
	return config, nil
}

func hotspotWifiInterface(ctx context.Context, db *sql.DB) (string, error) {
	config, err := getHotspotConfig(ctx, db)
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
// sobrescrita via PATCH (saveHotspotConfig rejeita chaves fora da
// allowlist). Usada so por autoStartHotspotOnBoot em
// hotspot_autostart.go pra decidir se religa o hotspot sozinho quando
// o backend reinicia.
const hotspotDesiredStateKey = "_DESIRED_STATE"

func setHotspotDesiredState(ctx context.Context, db *sql.DB, running bool) error {
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

func hotspotDesiredStateRunning(ctx context.Context, db *sql.DB) (bool, error) {
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
