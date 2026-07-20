// config_store.go guarda a configuracao do dns-provider na tabela
// dns_config do Postgres. Esses valores saiam de .env.main: o painel edita
// aqui e o dns-provider le a mesma tabela quando inicia (ver
// services/worker/dns), sem ninguem reescrever arquivo de ambiente.
// DISCOVER_PORT continua vindo do ambiente - e porta de infraestrutura
// definida no compose, nao configuracao de negocio.
package dns

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"bindnet/backend/internal/platform/config"
)

const (
	KeyLocalTLDs    = "DNS_LOCAL_TLDS"
	KeyDomains      = "DOMAINS"
	KeyNodeName     = "DISCOVER_NODE_NAME"
	KeyRemoteRoutes = "DISCOVER_REMOTE_ROUTES"
)

var dnsConfigAllowed = map[string]bool{
	KeyLocalTLDs:    true,
	KeyDomains:      true,
	KeyNodeName:     true,
	KeyRemoteRoutes: true,
}

// dnsConfigDefaults espelha os defaults do proprio dns-provider, para o
// painel mostrar o mesmo que o servico usaria antes de qualquer edicao.
var dnsConfigDefaults = map[string]string{
	KeyLocalTLDs:    "local,test,example",
	KeyRemoteRoutes: "auto",
}

// discoverPort e a unica chave da secao que continua vindo do ambiente:
// e a porta que o compose publica para o dns-provider, nao algo que o
// operador edite no painel.
func discoverPort() string {
	return config.Getenv("DISCOVER_PORT", "8531")
}

func getDNSConfig(ctx context.Context, db *sql.DB) (map[string]string, error) {
	values := map[string]string{}
	for key := range dnsConfigAllowed {
		values[key] = dnsConfigDefaults[key]
	}

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
		if dnsConfigAllowed[key] {
			values[key] = strings.TrimSpace(value)
		}
	}
	return values, rows.Err()
}

func saveDNSConfig(ctx context.Context, db *sql.DB, values map[string]string) error {
	for key := range values {
		if !dnsConfigAllowed[key] {
			return fmt.Errorf("chave '%s' nao pode ser alterada na configuracao de DNS", key)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO dns_config (key, value, updated_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (key) DO UPDATE
			SET value = EXCLUDED.value,
			    updated_at = CURRENT_TIMESTAMP
		`, key, strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
