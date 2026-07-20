// hotspot_logs.go cuida da aba "Logs" da tela de hotspot: repassar o
// stream de "docker compose logs" (GET /api/hotspot/logs, ver
// streamLogs em workerclient.go) e o botao "Limpar logs"
// (POST /api/hotspot/logs/clear).
package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"
)

// hotspotLogsClearedAtKey guarda o timestamp (RFC3339) do ultimo
// "Limpar logs" clicado na aba de Logs do hotspot, na mesma tabela
// hotspot_config mas fora de hotspotConfigKeys - mesmo padrao de
// hotspotDesiredStateKey (hotspot_config_store.go): nao aparece em
// GET /api/hotspot/config nem pode ser sobrescrita via PATCH
// (saveHotspotConfig rejeita chaves fora da allowlist).
//
// Nao da pra truncar de verdade o log nativo do Docker (arquivo
// *-json.log do driver de log, so acessivel no host - o worker nao
// tem e nao deve ganhar esse acesso, ver CLAUDE.md) - "limpar" aqui
// significa lembrar o corte de tempo e sempre pedir os logs ao worker
// com "--since" esse timestamp, tanto no GET abaixo quanto em
// handleContainerLogs (services/worker/controller/containers.go).
const hotspotLogsClearedAtKey = "_LOGS_CLEARED_AT"

func RegisterHotspotLogsRoutes(mux *http.ServeMux, worker *workerapi.Client, admin *auth.Administrator, audit *audit.Client, db *sql.DB) {
	mux.HandleFunc("GET /api/hotspot/logs", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		since, err := hotspotLogsClearedAt(r.Context(), db)
		if err != nil {
			log.Printf("[backend] falha ao ler corte de logs do hotspot: %v", err)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = worker.StreamLogs(r.Context(), w, "hotspot", r.URL.Query().Get("follow") == "true", since)
	}))

	mux.HandleFunc("POST /api/hotspot/logs/clear", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		if err := setHotspotLogsClearedAt(r.Context(), db); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "hotspot_logs_cleared", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))
}

func setHotspotLogsClearedAt(ctx context.Context, db *sql.DB) error {
	value := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		INSERT INTO hotspot_config (key, value, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = CURRENT_TIMESTAMP
	`, hotspotLogsClearedAtKey, value)
	return err
}

func hotspotLogsClearedAt(ctx context.Context, db *sql.DB) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM hotspot_config WHERE key = $1`, hotspotLogsClearedAtKey).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}
