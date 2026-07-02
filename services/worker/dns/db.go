package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// openPostgres segue o mesmo padrao de services/backend/db.go - o schema
// (tabela local_dns_records + sequence de offsets) e criado pelo
// services/migration antes deste servico subir.
func openPostgres() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		getenv("POSTGRES_USER", "bindnet"),
		getenv("POSTGRES_PASSWORD", ""),
		getenv("POSTGRES_HOST", "postgres"),
		getenv("POSTGRES_PORT", "5432"),
		getenv("POSTGRES_DB", "bindnet"),
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

// loadAllRecords le todos os registros existentes - usado para hidratar o
// cache Redis na inicializacao (ver cache.go:hydrateCache).
func loadAllRecords(ctx context.Context, db *sql.DB) (map[string]int64, error) {
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

// getOrAllocateOffset devolve o offset de loopback ja atribuido a
// "hostname", ou aloca um novo (via local_dns_records_offset_seq) na
// primeira vez que esse hostname e consultado. Concorrencia-segura via
// INSERT ... ON CONFLICT DO NOTHING seguido de SELECT.
func getOrAllocateOffset(ctx context.Context, db *sql.DB, hostname string) (int64, error) {
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
