package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"
)

// autoStartHotspotOnBoot religa o hotspot sozinho quando o backend
// sobe (container recriado, ou reboot da maquina), mas so se a ultima
// intencao do admin (POST /api/hotspot/start ou /stop) foi ligar - ver
// hotspotDesiredStateRunning em hotspot_config_store.go. Sem isso, o
// container do hotspot sobe em modo "manager" e fica ocioso ate
// alguem clicar em "Iniciar" no painel de novo, mesmo que o hotspot
// estivesse ligado antes do restart.
//
// Roda em goroutine de fundo (chamada assim em main.go) com retry
// curto: o worker/container do hotspot podem demorar alguns segundos
// para ficar prontos logo apos o backend subir.
func autoStartHotspotOnBoot(db *sql.DB, worker *workerClient, audit *auditClient) {
	ctx := context.Background()

	desired, err := hotspotDesiredStateRunning(ctx, db)
	if err != nil {
		log.Printf("[backend] autostart do hotspot: falha ao ler estado desejado: %v", err)
		return
	}
	if !desired {
		return
	}

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(3 * time.Second)
		}

		var status struct {
			Running bool `json:"running"`
		}
		if err := worker.call(ctx, http.MethodGet, "/hotspot/status", nil, &status); err != nil {
			lastErr = err
			continue
		}
		if status.Running {
			return
		}

		iface, err := currentHotspotInterface(ctx, db)
		if err != nil {
			lastErr = err
			continue
		}
		if err := startHotspotRuntimeConfig(ctx, db, worker); err != nil {
			lastErr = err
			continue
		}
		reapplyHotspotBlocklist(ctx, db, worker, iface)
		reapplyHotspotShaping(ctx, db, worker, iface)
		audit.record(ctx, "hotspot_started", "sistema (autostart)", nil)
		log.Println("[backend] hotspot religado automaticamente apos reinicio do backend")
		return
	}
	log.Printf("[backend] autostart do hotspot desistiu apos varias tentativas: %v", lastErr)
}
