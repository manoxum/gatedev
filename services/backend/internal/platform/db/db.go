package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"

	"bindnet/backend/internal/platform/config"
)

// Open conecta no banco principal do stack. O schema e criado
// pelo services/migration (prisma migrate deploy) antes do backend subir
// (ver docker-compose.yml: backend depende de migration com condition
// service_completed_successfully) - o backend nunca cria/altera tabelas
// sozinho, so le e escreve linhas.
func Open() (*sql.DB, error) {
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
