// Package settings guarda a configuracao do proprio painel (nome comum da
// CA e credenciais da API do nginx-ui) na tabela panel_config do Postgres.
// Esses valores saiam de .env.main: quem edita agora e a tela
// Configuracoes do painel. E a camada mais baixa (so depende do banco), pra
// internal/cert poder ler CA/nginx-ui sem ciclo de import.
package settings

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

const (
	KeyCACommonName    = "CA_COMMON_NAME"
	KeyNginxUIUsername = "NGINX_UI_USERNAME"
	KeyNginxUIPassword = "NGINX_UI_PASSWORD"
)

// DefaultCACommonName e o CN usado quando a CA e gerada pela primeira vez
// sem nada configurado - mesmo default historico do .env.example.
const DefaultCACommonName = "Bindnet Local Development CA"

var allowedKeys = map[string]bool{
	KeyCACommonName:    true,
	KeyNginxUIUsername: true,
	KeyNginxUIPassword: true,
}

// Get devolve o valor da chave, ou "" quando ainda nao foi configurada.
func Get(ctx context.Context, db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM panel_config WHERE key = $1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// GetAll devolve todas as chaves conhecidas (ausentes viram "").
func GetAll(ctx context.Context, db *sql.DB) (map[string]string, error) {
	values := map[string]string{}
	for key := range allowedKeys {
		values[key] = ""
	}

	rows, err := db.QueryContext(ctx, `SELECT key, value FROM panel_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if allowedKeys[key] {
			values[key] = strings.TrimSpace(value)
		}
	}
	return values, rows.Err()
}

// Save grava as chaves informadas. Rejeita chave fora da allowlist para o
// PATCH do painel nunca escrever algo arbitrario na tabela.
func Save(ctx context.Context, db *sql.DB, values map[string]string) error {
	for key := range values {
		if !allowedKeys[key] {
			return fmt.Errorf("chave '%s' nao pode ser alterada nas configuracoes do painel", key)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO panel_config (key, value, updated_at)
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

// ImportFromEnv traz, uma unica vez, os valores que ainda vivem no ambiente
// do container para a tabela - mesmo padrao de importacao do
// cert.LoadOrImportCA (que importou a CA legada do volume antigo). So
// insere chave ausente com env preenchido: depois que o operador editar
// pelo painel, o env deixa de ter efeito e pode ser removido do .env.
func ImportFromEnv(ctx context.Context, db *sql.DB) error {
	for key := range allowedKeys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO panel_config (key, value, updated_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (key) DO NOTHING
		`, key, value); err != nil {
			return err
		}
	}
	return nil
}

// CACommonName devolve o CN a usar ao gerar uma CA nova (banco, senao o
// default). Nunca altera uma CA ja existente - ver cert.LoadOrImportCA.
func CACommonName(ctx context.Context, db *sql.DB) string {
	value, err := Get(ctx, db, KeyCACommonName)
	if err != nil || value == "" {
		return DefaultCACommonName
	}
	return value
}

// NginxUICredentials devolve usuario/senha da API do nginx-ui. Vazio
// significa "nao configurado": a emissao de certificado segue funcionando,
// so cai na importacao local em vez da API (ver cert/nginxui_sync.go).
func NginxUICredentials(ctx context.Context, db *sql.DB) (username, password string) {
	values, err := GetAll(ctx, db)
	if err != nil {
		return "", ""
	}
	return values[KeyNginxUIUsername], values[KeyNginxUIPassword]
}
