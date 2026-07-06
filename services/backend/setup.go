// setup.go expõe o checklist usado pela tela de configuração inicial
// (hotspot/DNS/status dos serviços) - o administrador em si vem
// exclusivamente de ADMIN_USERNAME/ADMIN_PASSWORD no .env (ver
// admin.go), nunca é criado por esta rota.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

type setupServiceStatus struct {
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

type setupStatusResponse struct {
	HotspotConfigured bool                          `json:"hotspotConfigured"`
	Services          map[string]setupServiceStatus `json:"services"`
}

func registerSetupRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, audit *auditClient) {
	mux.HandleFunc("GET /api/setup/status", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		hotspotConfigured, err := hotspotConfigPresent(ctx, db)
		if err != nil {
			hotspotConfigured = false
		}

		response := setupStatusResponse{
			HotspotConfigured: hotspotConfigured,
			Services: map[string]setupServiceStatus{
				"postgres": pingPostgres(ctx, db),
				"mongo":    pingMongo(ctx, audit),
				"minio":    pingMinio(ctx),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

func hotspotConfigPresent(ctx context.Context, db *sql.DB) (bool, error) {
	config, err := getHotspotConfig(ctx, db)
	if err != nil {
		return false, err
	}
	for _, key := range requiredHotspotRuntimeKeys {
		if strings.TrimSpace(config[key]) == "" {
			return false, nil
		}
	}
	return true, nil
}

func pingPostgres(ctx context.Context, db *sql.DB) setupServiceStatus {
	if err := db.PingContext(ctx); err != nil {
		return setupServiceStatus{Reachable: false, Error: "postgres inacessivel"}
	}
	return setupServiceStatus{Reachable: true}
}

func pingMongo(ctx context.Context, audit *auditClient) setupServiceStatus {
	if audit == nil {
		return setupServiceStatus{Reachable: false, Error: "mongo nao inicializado"}
	}
	if err := audit.ping(ctx); err != nil {
		return setupServiceStatus{Reachable: false, Error: "mongo inacessivel"}
	}
	return setupServiceStatus{Reachable: true}
}

func pingMinio(ctx context.Context) setupServiceStatus {
	host := getenv("MINIO_HOST", "minio")
	port := getenv("MINIO_PORT", "9000")
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return setupServiceStatus{Reachable: false, Error: "minio nao respondeu a tempo"}
		}
		return setupServiceStatus{Reachable: false, Error: "minio inacessivel"}
	}
	_ = conn.Close()
	return setupServiceStatus{Reachable: true}
}
