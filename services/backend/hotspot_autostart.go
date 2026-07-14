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
		if err := startHotspotAndReapply(ctx, db, worker, audit, iface, "sistema (autostart)"); err != nil {
			lastErr = err
			continue
		}
		log.Println("[backend] hotspot religado automaticamente apos reinicio do backend")
		return
	}
	log.Printf("[backend] autostart do hotspot desistiu apos varias tentativas: %v", lastErr)
}

// startHotspotAndReapply religa o servico do hotspot e reaplica
// bloqueios/shaping por cima - mesma sequencia usada tanto aqui
// (boot do backend) quanto pela recuperacao automatica em
// reconcileHotspotOnce (hotspot_reconcile.go) quando o hotspot cai
// sozinho (ex.: watchdog de falha de beacon em
// services/worker/hotspot/watchdog.sh) com o admin ainda querendo ele
// ligado.
func startHotspotAndReapply(ctx context.Context, db *sql.DB, worker *workerClient, audit *auditClient, iface, username string) error {
	if err := startHotspotRuntimeConfig(ctx, db, worker); err != nil {
		return err
	}
	reapplyHotspotBlocklist(ctx, db, worker, iface)
	reapplyHotspotShaping(ctx, db, worker, iface)
	audit.record(ctx, "hotspot_started", username, nil)
	return nil
}
