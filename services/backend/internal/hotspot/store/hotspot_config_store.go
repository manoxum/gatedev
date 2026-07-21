package store

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
	"WIFI_OPEN",
	"WIFI_COUNTRY",
	"WIFI_CHANNEL",
	"WIFI_FREQ_BAND",
	"WIFI_CHANNEL_CANDIDATES",
	"HOTSPOT_GATEWAY",
	"HOTSPOT_CIDR",
	"HOTSPOT_DNS_FALLBACKS",
	"BINDNET_UPLINK_INTERFACE",
	"UPLINK_MONITOR_INTERVAL",
	"CLIENT_ISOLATION",
}

var hotspotConfigDefaults = map[string]string{
	"INTERNET_INTERFACE":       "auto",
	"WIFI_OPEN":                "false",
	"WIFI_COUNTRY":             "ST",
	"WIFI_CHANNEL":             "auto",
	"WIFI_FREQ_BAND":           "auto",
	"HOTSPOT_GATEWAY":          "192.168.12.1",
	"HOTSPOT_CIDR":             "192.168.12.0/24",
	"HOTSPOT_DNS_FALLBACKS":    "1.1.1.1,8.8.8.8",
	"BINDNET_UPLINK_INTERFACE": "bn-uplink",
	"UPLINK_MONITOR_INTERVAL":  "10",
	// Interruptor geral do isolamento de clientes (ap_isolate no
	// create_ap + chain BINDNET-ISOLATION no worker) - mudar exige
	// reiniciar o hotspot, ver hotspot_isolation.go.
	"CLIENT_ISOLATION": "false",
}

// requiredHotspotRuntimeKeys nao inclui WIFI_PASSWORD: quando
// WIFI_OPEN=true (hotspot livre, sem autenticacao) o create_ap sobe sem
// passphrase de proposito (ver try_create_ap em
// services/worker/hotspot/entrypoint.sh) - a senha so e obrigatoria no
// caso contrario, checado a parte por MissingHotspotRuntimeKey.
var requiredHotspotRuntimeKeys = []string{
	"WIFI_INTERFACE",
	"INTERNET_INTERFACE",
	"WIFI_SSID",
}

// MissingHotspotRuntimeKey devolve o nome da primeira chave obrigatoria
// ausente num config ja resolvido (com defaults aplicados, ver
// GetHotspotConfig), ou "" se completo. Compartilhada por
// HotspotRuntimeConfig (usada antes de start/apply) e hotspotConfigPresent
// (services/backend/setup.go, tela de configuracao inicial) para as duas
// nunca divergirem sobre o que conta como "hotspot configurado".
func MissingHotspotRuntimeKey(config map[string]string) string {
	for _, key := range requiredHotspotRuntimeKeys {
		if strings.TrimSpace(config[key]) == "" {
			return key
		}
	}
	if config["WIFI_OPEN"] != "true" && strings.TrimSpace(config["WIFI_PASSWORD"]) == "" {
		return "WIFI_PASSWORD"
	}
	return ""
}

func hotspotConfigAllowedSet() map[string]bool {
	allowed := make(map[string]bool, len(hotspotConfigKeys))
	for _, key := range hotspotConfigKeys {
		allowed[key] = true
	}
	return allowed
}

func GetHotspotConfig(ctx context.Context, db *sql.DB) (map[string]string, error) {
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

func SaveHotspotConfig(ctx context.Context, db *sql.DB, values map[string]string) error {
	allowed := hotspotConfigAllowedSet()
	clean := make(map[string]string, len(values))
	for key, value := range values {
		if !allowed[key] {
			return fmt.Errorf("chave '%s' nao pode ser alterada na configuracao do hotspot", key)
		}
		clean[key] = strings.TrimSpace(value)
	}
	if open, ok := clean["WIFI_OPEN"]; ok && open != "true" && open != "false" {
		return errors.New("WIFI_OPEN deve ser 'true' ou 'false'")
	}
	if isolation, ok := clean["CLIENT_ISOLATION"]; ok && isolation != "true" && isolation != "false" {
		return errors.New("CLIENT_ISOLATION deve ser 'true' ou 'false'")
	}
	// A validacao de tamanho minimo so vale para hotspot com senha - um
	// PATCH que liga WIFI_OPEN e WIFI_PASSWORD (vazio ou nao) na mesma
	// chamada e o fluxo normal do painel, ver useHotspotMutations.ts
	// (o formulario sempre envia o objeto de config inteiro).
	if password, ok := clean["WIFI_PASSWORD"]; ok && clean["WIFI_OPEN"] != "true" && len(password) < 8 {
		return errors.New("WIFI_PASSWORD deve ter ao menos 8 caracteres (requisito WPA2), a menos que o hotspot esteja marcado como livre (WIFI_OPEN)")
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

func HotspotRuntimeConfig(ctx context.Context, db *sql.DB) (map[string]string, error) {
	config, err := GetHotspotConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	if key := MissingHotspotRuntimeKey(config); key != "" {
		return nil, fmt.Errorf("%s nao configurado pelo painel", key)
	}
	return config, nil
}
