package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"time"
)

// startHotspotReconciliationLoop e o unico lugar que decide "quando"
// aplicar shaping/cota/credito - o worker so executa comandos
// idempotentes sob demanda, mesma filosofia de reapplyHotspotBlocklist.
// Roda para sempre (goroutine de fundo, iniciada em main.go); erros por
// dispositivo so logam e nao abortam o ciclo inteiro.
func startHotspotReconciliationLoop(db *sql.DB, worker *workerClient, interval time.Duration) {
	ctx := context.Background()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		reconcileHotspotOnce(ctx, db, worker)
	}
}

func reconcileHotspotOnce(ctx context.Context, db *sql.DB, worker *workerClient) {
	var status struct {
		Running bool `json:"running"`
	}
	if err := worker.call(ctx, http.MethodGet, "/hotspot/status", nil, &status); err != nil || !status.Running {
		return
	}

	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] reconciliacao: falha ao ler WIFI_INTERFACE: %v", err)
		return
	}
	clients, err := liveHotspotClients(ctx, worker, iface)
	if err != nil {
		log.Printf("[backend] reconciliacao: falha ao listar clientes do hotspot: %v", err)
		return
	}

	for _, client := range clients {
		if err := reconcileDevice(ctx, db, worker, iface, client.MAC, client.IP); err != nil {
			log.Printf("[backend] reconciliacao do dispositivo %s falhou: %v", client.MAC, err)
		}
	}

	if err := reconcileGlobal(ctx, db, worker, iface); err != nil {
		log.Printf("[backend] reconciliacao global falhou: %v", err)
	}
	if err := applyAutomaticRecharges(db); err != nil {
		log.Printf("[backend] recarga automatica de credito falhou: %v", err)
	}
}

// reconcileDevice reaplica shaping (resolve renovacao de DHCP sozinho,
// reenviando o IP atual), atualiza o acumulado de cota/credito do
// dispositivo e ativa/desativa throttle e bloqueio conforme necessario.
func reconcileDevice(ctx context.Context, db *sql.DB, worker *workerClient, iface, mac, ip string) error {
	if err := ensureDeviceShaping(ctx, db, worker, iface, mac, ip); err != nil {
		return err
	}

	download, upload, err := readDeviceShapingStats(ctx, worker, mac)
	if err != nil {
		return err
	}
	deltaDown, deltaUp, err := recordDeviceUsage(db, mac, download, upload)
	if err != nil {
		return err
	}

	limits, _, err := getDeviceLimits(db, mac)
	if err != nil {
		return err
	}
	if limits.QuotaPeriod != nil {
		if err := resetDevicePeriodIfExpired(db, mac, *limits.QuotaPeriod); err != nil {
			return err
		}
	}
	traffic, err := ensureDeviceTrafficRow(db, mac)
	if err != nil {
		return err
	}
	exceeded := deviceQuotaExceeded(limits, traffic)
	if exceeded != traffic.Throttled {
		if err := setDeviceThrottled(db, mac, exceeded); err != nil {
			return err
		}
		if err := ensureDeviceShaping(ctx, db, worker, iface, mac, ip); err != nil {
			return err
		}
	}

	return reconcileDeviceCredit(ctx, db, worker, mac, ip, deltaDown+deltaUp)
}

// reconcileDeviceCredit desconta o trafego deste ciclo do saldo de
// credito (so quando habilitado) e bloqueia ao vivo assim que o saldo
// zera - desbloquear e responsabilidade exclusiva de uma recarga
// (manual ou automatica), nunca deste loop. Enquanto o dispositivo
// continuar bloqueado por credito, reforca o bloqueio a cada ciclo
// (auto-cura): o hotspot flusha o chain BINDNET-HOTSPOT a cada
// start/apply, o que apagaria as regras DROP junto - reenviar aqui,
// mesmo idioma ja usado por ensureDeviceShaping.
func reconcileDeviceCredit(ctx context.Context, db *sql.DB, worker *workerClient, mac, ip string, totalBytes int64) error {
	credit, err := ensureDeviceCreditRow(db, mac)
	if err != nil {
		return err
	}
	if !credit.Enabled {
		return nil
	}
	if credit.BlockedByCredit {
		applyLiveCreditBlock(ctx, db, worker, mac, ip, true)
	}
	if totalBytes == 0 {
		return nil
	}
	newBalance, err := debitDeviceCredit(db, mac, totalBytes)
	if err != nil {
		return err
	}
	if newBalance <= 0 && !credit.BlockedByCredit {
		if err := blockDeviceForCredit(db, mac); err != nil {
			return err
		}
		applyLiveCreditBlock(ctx, db, worker, mac, ip, true)
	}
	return nil
}

func reconcileGlobal(ctx context.Context, db *sql.DB, worker *workerClient, iface string) error {
	download, upload, err := readDeviceShapingStats(ctx, worker, "")
	if err != nil {
		return err
	}
	if err := recordGlobalUsage(db, download, upload); err != nil {
		return err
	}

	limits, err := getGlobalLimits(db)
	if err != nil {
		return err
	}
	if limits.QuotaPeriod != nil {
		if err := resetGlobalPeriodIfExpired(db, *limits.QuotaPeriod); err != nil {
			return err
		}
	}
	traffic, err := ensureGlobalTrafficRow(db)
	if err != nil {
		return err
	}
	exceeded := globalQuotaExceeded(limits, traffic)
	if exceeded == traffic.Throttled {
		return nil
	}
	if err := setGlobalThrottled(db, exceeded); err != nil {
		return err
	}
	downloadRate, uploadRate := effectiveGlobalRates(limits, traffic)
	return applyGlobalShaping(ctx, worker, iface, downloadRate, uploadRate)
}

type shapingStatsPayload struct {
	DownloadBytes uint64 `json:"downloadBytes"`
	UploadBytes   uint64 `json:"uploadBytes"`
}

// readDeviceShapingStats le os contadores absolutos do worker - mac
// vazio pede os contadores globais (sem MAC/IP).
func readDeviceShapingStats(ctx context.Context, worker *workerClient, mac string) (download, upload uint64, err error) {
	path := "/hotspot/shaping/stats"
	if mac != "" {
		path += "?mac=" + url.QueryEscape(mac)
	}
	var stats shapingStatsPayload
	if err := worker.call(ctx, http.MethodGet, path, nil, &stats); err != nil {
		return 0, 0, err
	}
	return stats.DownloadBytes, stats.UploadBytes, nil
}
