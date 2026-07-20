package store

import (
	"context"
	"database/sql"
	"os"
	"strings"

	"bindnet/dns-provider/internal/config"
)

// Chaves de dns_config - a configuracao do dns-provider administrada pelo
// painel. DISCOVER_PORT nao esta aqui de proposito: continua vindo do
// ambiente (porta publicada pelo compose, nao configuracao de negocio).
const (
	KeyLocalTLDs    = "DNS_LOCAL_TLDS"
	KeyDomains      = "DOMAINS"
	KeyNodeName     = "DISCOVER_NODE_NAME"
	KeyRemoteRoutes = "DISCOVER_REMOTE_ROUTES"
)

var dnsConfigKeys = []string{KeyLocalTLDs, KeyDomains, KeyNodeName, KeyRemoteRoutes}

// LoadDNSConfig le a tabela dns_config. Chave ausente simplesmente nao
// aparece no mapa - quem chama decide o fallback (ver Setting).
func LoadDNSConfig(ctx context.Context, db *sql.DB) (map[string]string, error) {
	values := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM dns_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = strings.TrimSpace(value)
	}
	return values, rows.Err()
}

// ImportDNSConfigFromEnv traz, uma unica vez, o que ainda estiver no
// ambiente do container para a tabela - a instalacao existente ja tem
// DNS_LOCAL_TLDS/DOMAINS/DISCOVER_* no .env, e sem isso a primeira subida
// apos a migracao perderia esses valores para os defaults. So insere chave
// ausente com env preenchido (ON CONFLICT DO NOTHING), entao depois que o
// operador editar pelo painel o ambiente deixa de ter efeito.
func ImportDNSConfigFromEnv(ctx context.Context, db *sql.DB) error {
	for _, key := range dnsConfigKeys {
		value := config.Getenv(key, "")
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO dns_config (key, value, updated_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (key) DO NOTHING
		`, key, strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return nil
}

// Setting resolve um valor na ordem banco -> ambiente -> default. O degrau
// do ambiente e o que mantem a instalacao funcionando entre o deploy desta
// versao e a remocao das variaveis do .env.
func Setting(values map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return config.Getenv(key, fallback)
	}
	return fallback
}
