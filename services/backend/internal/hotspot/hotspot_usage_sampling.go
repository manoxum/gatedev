package hotspot

import (
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"
)

// StartHotspotUsageSamplingLoop mede o trafego de cada dispositivo
// conectado numa cadencia bem mais curta que StartHotspotReconciliationLoop
// (services/backend/hotspot_reconcile.go, 15s - la o custo e reaplicar
// shaping/blocklist, que nao precisa de mais que isso). O grafico de
// velocidade do dispositivo (DeviceSpeedChart.tsx) so mostra uma
// tendencia real se cada ponto vier de uma amostra curta - com uma
// leitura a cada 15s, uma rajada breve de trafego se dissolvia na
// media do intervalo inteiro. Roda para sempre (goroutine de fundo,
// iniciada em main.go); erros por dispositivo so logam e nao abortam o
// ciclo inteiro.
func StartHotspotUsageSamplingLoop(db *sql.DB, worker *workerapi.Client, creditTrace *creditTraceClient, speedHistory *deviceSpeedHistoryStore, interval time.Duration) {
	ctx := context.Background()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		sampleHotspotUsageOnce(ctx, db, worker, creditTrace, speedHistory)
	}
}

func sampleHotspotUsageOnce(ctx context.Context, db *sql.DB, worker *workerapi.Client, creditTrace *creditTraceClient, speedHistory *deviceSpeedHistoryStore) {
	var status struct {
		Running bool `json:"running"`
	}
	if err := worker.Call(ctx, http.MethodGet, "/hotspot/status", nil, &status); err != nil || !status.Running {
		// Sem log aqui de proposito: com o hotspot desligado isso
		// dispara a cada tick desta amostragem curta, o que inundaria o
		// log sem trazer nada novo - StartHotspotReconciliationLoop ja
		// loga o mesmo tipo de falha na sua propria cadencia (15s).
		return
	}

	if err := reconcileGlobalUsage(db, worker, speedHistory); err != nil {
		log.Printf("[backend] amostragem de trafego global falhou: %v", err)
	}

	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		return
	}
	clients, err := liveHotspotClients(ctx, worker, iface)
	if err != nil {
		return
	}

	for _, client := range clients {
		if err := reconcileDeviceUsage(ctx, db, worker, creditTrace, speedHistory, client.MAC, client.IP); err != nil {
			log.Printf("[backend] amostragem de trafego do dispositivo %s falhou: %v", client.MAC, err)
		}
	}
}

// reconcileGlobalUsage e o equivalente global de reconcileDeviceUsage -
// grava o delta acumulado (recordGlobalUsage, tambem usado por
// globalQuotaExceeded via hotspot_global_traffic) e a amostra de
// velocidade geral (sob a chave sentinela globalSpeedHistoryKey no
// mesmo deviceSpeedHistoryStore usado por dispositivo). Roda so aqui, a
// 1s - reconcileGlobal (hotspot_reconcile.go, 15s) so decide
// throttle/shaping a partir do acumulado ja atualizado por esta funcao,
// nunca grava o delta de novo (ver comentario la).
func reconcileGlobalUsage(db *sql.DB, worker *workerapi.Client, speedHistory *deviceSpeedHistoryStore) error {
	ctx := context.Background()
	download, upload, err := readDeviceShapingStats(ctx, worker, "")
	if err != nil {
		return err
	}
	deltaDown, deltaUp, err := recordGlobalUsage(db, download, upload)
	if err != nil {
		return err
	}
	speedHistory.record(globalSpeedHistoryKey, deltaDown, deltaUp, time.Now())
	return nil
}

// reconcileDeviceUsage le os contadores acumulados do dispositivo,
// grava a amostra de velocidade deste ciclo e atualiza o acumulado de
// cota/credito - contraparte de reconcileDeviceShaping
// (hotspot_reconcile.go), que cuida so de shaping/bloqueio no ciclo de
// 15s. Reaplicar shaping aqui tambem seria redundante (ensureDeviceShaping
// ja roda no ciclo de 15s) e caro demais pra rodar a cada segundo.
func reconcileDeviceUsage(ctx context.Context, db *sql.DB, worker *workerapi.Client, creditTrace *creditTraceClient, speedHistory *deviceSpeedHistoryStore, mac, ip string) error {
	download, upload, err := readDeviceShapingStats(ctx, worker, mac)
	if err != nil {
		return err
	}
	deltaDown, deltaUp, err := recordDeviceUsage(db, mac, download, upload)
	if err != nil {
		return err
	}
	speedHistory.record(mac, deltaDown, deltaUp, time.Now())

	// Consumo da sessao e o trafego real do dispositivo, independente
	// do LimitType (ver hotspot_sessions.go) - debitar credito ou
	// incrementar cota sao consequencias separadas, tratadas abaixo.
	if totalDelta := deltaDown + deltaUp; totalDelta != 0 {
		if err := incrementSessionConsumption(db, mac, totalDelta); err != nil {
			return err
		}
	}

	limits, err := effectiveDeviceLimits(db, mac)
	if err != nil {
		return err
	}

	if limits.LimitType == limitTypeQuota {
		if err := reconcileDeviceQuota(ctx, db, worker, mac, ip, limits, deltaDown, deltaUp); err != nil {
			return err
		}
	} else if err := clearStaleDeviceQuotaBlock(ctx, db, worker, mac, ip); err != nil {
		return err
	}

	return reconcileDeviceCredit(ctx, db, worker, creditTrace, mac, ip, limits.LimitType, deltaDown+deltaUp)
}
