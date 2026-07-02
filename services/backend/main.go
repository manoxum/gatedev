// Comando backend e a API publica consumida pelo services/frontend.
// Guarda autenticacao e regras de negocio, mas nunca toca o host ou o
// Docker diretamente - qualquer operacao privilegiada e delegada ao
// services/worker via socket Unix (ver workerclient.go).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags)
	log.Println("[backend] iniciando API do painel de gestao")

	admin, err := loadOrCreateAdmin()
	if err != nil {
		log.Fatalf("[backend] erro ao preparar credenciais de administrador: %v", err)
	}

	db, err := openPostgres()
	if err != nil {
		log.Fatalf("[backend] erro ao conectar no Postgres: %v", err)
	}
	defer db.Close()

	ca, err := loadOrImportCA(db)
	if err != nil {
		log.Fatalf("[backend] erro ao preparar CA local: %v", err)
	}

	audit, err := openMongo()
	if err != nil {
		log.Fatalf("[backend] erro ao conectar no Mongo: %v", err)
	}
	defer audit.disconnect(context.Background())

	worker := newWorkerClient(getenv("WORKER_SOCKET", "/run/bindnet-admin/worker.sock"))

	mux := http.NewServeMux()
	registerAuthRoutes(mux, admin, audit)
	registerDashboardRoutes(mux, worker, admin)
	registerHotspotRoutes(mux, worker, admin, audit)
	registerDNSRoutes(mux, worker, admin, audit)
	registerCertificateRoutes(mux, admin, db, ca, audit)
	registerNginxUIRoutes(mux, admin)

	port := getenv("BACKEND_PORT", "8090")
	log.Println("[backend] ouvindo em :" + port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("[backend] erro no servidor: %v", err)
	}
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
