// Package store concentra o acesso ao Postgres do dns-provider: alocacao
// persistente de offset de loopback por hostname, tabela de descoberta,
// identidade do no e leitura de config operacional do hotspot. O schema e
// criado pelo services/migration antes deste servico subir.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"bindnet/dns-provider/internal/config"
)

// OpenPostgres segue o mesmo padrao de services/backend/db.go.
func OpenPostgres() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		config.Getenv("POSTGRES_USER", "bindnet"),
		config.Getenv("POSTGRES_PASSWORD", ""),
		config.Getenv("POSTGRES_HOST", "postgres"),
		config.Getenv("POSTGRES_PORT", "5432"),
		config.Getenv("POSTGRES_DB", "bindnet"),
	)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func LoadHotspotGateway(ctx context.Context, db *sql.DB, fallback string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM hotspot_config WHERE key = 'HOTSPOT_GATEWAY'`).Scan(&value)
	if err == sql.ErrNoRows {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

// LoadAllRecords le todos os registros existentes - usado para hidratar o
// cache Redis na inicializacao (ver pacote cache).
func LoadAllRecords(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT hostname, loopback_offset FROM local_dns_records`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := map[string]int64{}
	for rows.Next() {
		var hostname string
		var offset int64
		if err := rows.Scan(&hostname, &offset); err != nil {
			return nil, err
		}
		records[hostname] = offset
	}
	return records, rows.Err()
}

// GetOrAllocateOffset devolve o offset de loopback ja atribuido a
// "hostname", ou aloca um novo (via local_dns_records_offset_seq) na
// primeira vez que esse hostname e consultado. Concorrencia-segura via
// INSERT ... ON CONFLICT DO NOTHING seguido de SELECT.
func GetOrAllocateOffset(ctx context.Context, db *sql.DB, hostname string) (int64, error) {
	var offset int64
	err := db.QueryRowContext(ctx, `SELECT loopback_offset FROM local_dns_records WHERE hostname = $1`, hostname).Scan(&offset)
	if err == nil {
		return offset, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO local_dns_records (hostname, loopback_offset)
		 VALUES ($1, nextval('local_dns_records_offset_seq'))
		 ON CONFLICT (hostname) DO NOTHING`,
		hostname,
	)
	if err != nil {
		return 0, err
	}

	err = db.QueryRowContext(ctx, `SELECT loopback_offset FROM local_dns_records WHERE hostname = $1`, hostname).Scan(&offset)
	return offset, err
}
