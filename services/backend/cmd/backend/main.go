// Comando backend e a API publica consumida pelo services/frontend.
// Guarda autenticacao e regras de negocio, mas nunca toca o host ou o
// Docker diretamente - qualquer operacao privilegiada e delegada ao
// services/worker via socket Unix (ver internal/workerapi). A logica fica
// nos pacotes internal/{auth,audit,workerapi,setup,dns,cert,hotspot} e
// internal/platform/{config,db}; este main so faz o wiring.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/cert"
	"bindnet/backend/internal/dns"
	"bindnet/backend/internal/hotspot"
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/platform/config"
	"bindnet/backend/internal/platform/db"
	"bindnet/backend/internal/settings"
	"bindnet/backend/internal/setup"
	"bindnet/backend/internal/workerapi"
)

func main() {
	log.SetFlags(log.LstdFlags)
	log.Println("[backend] iniciando API do painel de gestao")

	admin, err := auth.LoadOrCreate()
	if err != nil {
		log.Fatalf("[backend] erro ao preparar credenciais de administrador: %v", err)
	}

	database, err := db.Open()
	if err != nil {
		log.Fatalf("[backend] erro ao conectar no Postgres: %v", err)
	}
	defer database.Close()

	// Traz para panel_config o que ainda estiver no ambiente do container
	// (CA_COMMON_NAME, NGINX_UI_*). Roda antes de LoadOrImportCA de
	// proposito: numa instalacao nova, o CN importado aqui e o que a CA
	// recem-gerada vai usar. Depois disso o painel e a fonte de verdade e
	// essas variaveis podem sair do .env.
	if err := settings.ImportFromEnv(context.Background(), database); err != nil {
		log.Printf("[backend] aviso: falha ao importar configuracao do painel do ambiente: %v", err)
	}

	ca, err := cert.LoadOrImportCA(database)
	if err != nil {
		log.Fatalf("[backend] erro ao preparar CA local: %v", err)
	}
	if err := cert.SyncIssuedCertificatesToNginxUI(database); err != nil {
		log.Printf("[backend] falha no sync inicial de certificados com nginx-ui: %v", err)
	}

	auditClient, err := audit.Open()
	if err != nil {
		log.Fatalf("[backend] erro ao conectar no Mongo: %v", err)
	}
	defer auditClient.Disconnect(context.Background())

	creditTrace, err := store.OpenCreditTrace(context.Background(), auditClient.MongoClient())
	if err != nil {
		log.Fatalf("[backend] erro ao preparar trace de consumo de credito no Mongo: %v", err)
	}

	worker := workerapi.New(config.Getenv("WORKER_SOCKET", "/run/bindnet-admin/worker.sock"))

	speedHistory := hotspot.NewDeviceSpeedHistoryStore()

	mux := http.NewServeMux()
	setup.RegisterSetupRoutes(mux, admin, database, auditClient)
	settings.RegisterRoutes(mux, admin, database, auditClient)
	auth.RegisterRoutes(mux, admin, auditClient)
	setup.RegisterDashboardRoutes(mux, worker, admin)
	hotspot.RegisterHotspotRoutes(mux, worker, admin, auditClient, database)
	hotspot.RegisterHotspotDeviceRoutes(mux, admin, database, worker)
	hotspot.RegisterHotspotDeviceHistoryRoutes(mux, admin, database, worker)
	hotspot.RegisterHotspotBlocklistRoutes(mux, admin, database, worker)
	hotspot.RegisterHotspotDeviceLimitRoutes(mux, admin, database, worker)
	hotspot.RegisterHotspotDeviceQuotaRoutes(mux, admin, database, worker, auditClient)
	hotspot.RegisterHotspotDeviceSpeedHistoryRoutes(mux, admin, speedHistory)
	hotspot.RegisterHotspotCreditRoutes(mux, admin, database, worker, auditClient)
	hotspot.RegisterHotspotCreditHistoryRoutes(mux, admin, database)
	hotspot.RegisterHotspotSessionRoutes(mux, admin, database, creditTrace)
	hotspot.RegisterHotspotStatsRoutes(mux, admin, database, worker)
	hotspot.RegisterHotspotProfileRoutes(mux, admin, database, worker, auditClient)
	hotspot.RegisterHotspotVoucherRoutes(mux, admin, database, auditClient)
	hotspot.RegisterHotspotPortalRoutes(mux, database, worker, auditClient)
	dns.RegisterDNSRoutes(mux, worker, admin, auditClient, database)
	dns.RegisterDNSRouteRoutes(mux, admin, database, auditClient)
	dns.RegisterDNSPeerRoutes(mux, admin, database, worker, auditClient)
	dns.RegisterDNSRecordRoutes(mux, admin, database, auditClient)
	cert.RegisterCertificateRoutes(mux, admin, database, ca, worker, auditClient)
	cert.RegisterNginxUIRoutes(mux, admin)

	go hotspot.AutoStartHotspotOnBoot(database, worker, auditClient)
	go hotspot.StartHotspotReconciliationLoop(database, worker, auditClient, 15*time.Second)
	go hotspot.StartHotspotUsageSamplingLoop(database, worker, creditTrace, speedHistory, time.Second)

	port := config.Getenv("BACKEND_PORT", "8090")
	log.Println("[backend] ouvindo em :" + port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("[backend] erro no servidor: %v", err)
	}
}
