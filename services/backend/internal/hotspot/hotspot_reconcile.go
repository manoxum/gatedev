package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"time"
)

// StartHotspotReconciliationLoop e o unico lugar que decide "quando"
// aplicar shaping/cota/credito - o worker so executa comandos
// idempotentes sob demanda, mesma filosofia de reapplyHotspotBlocklist.
// Roda para sempre (goroutine de fundo, iniciada em main.go); erros por
// dispositivo so logam e nao abortam o ciclo inteiro.
func StartHotspotReconciliationLoop(db *sql.DB, worker *workerapi.Client, audit *audit.Client, interval time.Duration) {
	ctx := context.Background()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		reconcileHotspotOnce(ctx, db, worker, audit)
	}
}

func reconcileHotspotOnce(ctx context.Context, db *sql.DB, worker *workerapi.Client, audit *audit.Client) {
	var status struct {
		Running bool `json:"running"`
	}
	if err := worker.Call(ctx, http.MethodGet, "/hotspot/status", nil, &status); err != nil {
		return
	}
	if !status.Running {
		// Hotspot parado: ninguem pode estar conectado, fecha toda
		// sessao em aberto (ver closeStaleSessions em
		// hotspot_sessions.go).
		if err := closeStaleSessions(db, nil); err != nil {
			log.Printf("[backend] reconciliacao: falha ao fechar sessoes com hotspot parado: %v", err)
		}
		recoverHotspotIfDesired(ctx, db, worker, audit)
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

	liveMacs := make([]string, len(clients))
	for i, client := range clients {
		liveMacs[i] = client.MAC
	}
	if err := closeStaleSessions(db, liveMacs); err != nil {
		log.Printf("[backend] reconciliacao: falha ao fechar sessoes desconectadas: %v", err)
	}

	for _, client := range clients {
		if err := reconcileDeviceShaping(ctx, db, worker, iface, client.MAC, client.IP); err != nil {
			log.Printf("[backend] reconciliacao do dispositivo %s falhou: %v", client.MAC, err)
		}
	}

	if err := reconcileGlobal(ctx, worker, iface); err != nil {
		log.Printf("[backend] reconciliacao global falhou: %v", err)
	}
	if err := applyAutomaticRecharges(db); err != nil {
		log.Printf("[backend] recarga automatica de credito falhou: %v", err)
	}
}

// recoverHotspotIfDesired religa o hotspot sozinho quando ele cai sem
// que o admin tenha pedido (ex.: watchdog de falha de beacon em
// services/worker/hotspot/watchdog.sh derrubando o create_ap travado)
// - so age se a ultima intencao registrada foi ligar
// (hotspotDesiredStateRunning, a mesma usada por
// AutoStartHotspotOnBoot), senao um "parar" deliberado pelo painel
// seria desfeito no proximo ciclo deste loop.
func recoverHotspotIfDesired(ctx context.Context, db *sql.DB, worker *workerapi.Client, audit *audit.Client) {
	desired, err := hotspotDesiredStateRunning(ctx, db)
	if err != nil {
		log.Printf("[backend] reconciliacao: falha ao ler estado desejado do hotspot: %v", err)
		return
	}
	if !desired {
		return
	}

	iface, err := currentHotspotInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] reconciliacao: falha ao ler WIFI_INTERFACE para religar o hotspot: %v", err)
		return
	}
	if err := startHotspotAndReapply(ctx, db, worker, audit, iface, "sistema (auto-recuperacao)"); err != nil {
		log.Printf("[backend] reconciliacao: falha ao religar hotspot automaticamente: %v", err)
		return
	}
	log.Println("[backend] hotspot religado automaticamente apos queda detectada pela reconciliacao")
}

// reconcileDeviceShaping reaplica shaping (resolve renovacao de DHCP
// sozinho, reenviando o IP atual) e ativa/desativa bloqueio manual em
// modo "traffic" - roda no ciclo de 15s (StartHotspotReconciliationLoop).
// A leitura de trafego/velocidade/cota/credito, que precisa de cadencia
// bem mais curta pra alimentar o grafico "ao vivo", roda separada em
// reconcileDeviceUsage (hotspot_usage_sampling.go).
func reconcileDeviceShaping(ctx context.Context, db *sql.DB, worker *workerapi.Client, iface, mac, ip string) error {
	if err := ensureOpenSession(db, mac); err != nil {
		return err
	}
	if err := ensureDeviceShaping(ctx, db, worker, iface, mac, ip); err != nil {
		return err
	}

	// Auto-cura do bloqueio manual em modo "traffic": reforca a cada
	// ciclo pelo mesmo motivo do bloqueio por credito (a regra de
	// download some se o hotspot reiniciar, e o IP pode mudar numa
	// renovacao de DHCP sem restart nenhum).
	if mode, blocked, err := getHotspotBlockedDeviceMode(db, mac); err != nil {
		return err
	} else if blocked && mode == "traffic" {
		applyLiveTrafficBlock(ctx, db, worker, mac, ip, true)
	}
	return nil
}

// reconcileGlobal reaplica todo ciclo as regras de contagem agregada
// global (bn-global-up/down) - unica reaplicacao periodica delas: sem
// isso, se elas se perdessem uma vez (ex.: container do hotspot
// reiniciado), o velocimetro/grafico geral (useGlobalStats/
// useGlobalSpeedHistory) travava em 0bps indefinidamente.
// applyGlobalShaping/ensureGlobalCounterRule ja sao idempotentes, mesmo
// espirito de ensureDeviceShaping (chamada todo ciclo por dispositivo,
// ver reconcileDeviceShaping acima). Nao existe mais teto/cota global
// aqui (removido, ver RULE.md) - o acumulado por dispositivo continua
// sendo gravado separado, a 1s, em reconcileGlobalUsage
// (hotspot_usage_sampling.go), so pra alimentar a velocidade ao vivo.
func reconcileGlobal(ctx context.Context, worker *workerapi.Client, iface string) error {
	return applyGlobalShaping(ctx, worker, iface)
}

type shapingStatsPayload struct {
	DownloadBytes uint64 `json:"downloadBytes"`
	UploadBytes   uint64 `json:"uploadBytes"`
}

// readDeviceShapingStats le os contadores absolutos do worker - mac
// vazio pede os contadores globais (sem MAC/IP).
func readDeviceShapingStats(ctx context.Context, worker *workerapi.Client, mac string) (download, upload uint64, err error) {
	path := "/hotspot/shaping/stats"
	if mac != "" {
		path += "?mac=" + url.QueryEscape(mac)
	}
	var stats shapingStatsPayload
	if err := worker.Call(ctx, http.MethodGet, path, nil, &stats); err != nil {
		return 0, 0, err
	}
	return stats.DownloadBytes, stats.UploadBytes, nil
}
