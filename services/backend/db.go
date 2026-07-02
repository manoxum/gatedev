package main

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// openPostgres conecta no banco principal do stack. O schema e criado
// pelo services/migration (prisma migrate deploy) antes do backend subir
// (ver docker-compose.yml: backend depende de migration com condition
// service_completed_successfully) - o backend nunca cria/altera tabelas
// sozinho, so le e escreve linhas.
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
