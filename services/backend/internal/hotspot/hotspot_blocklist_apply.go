// hotspot_blocklist_apply.go aplica ao vivo (worker) o bloqueio
// persistido em hotspot_blocked_devices - separado de
// hotspot_blocklist.go (rotas HTTP + CRUD no Postgres) so pra manter
// cada arquivo focado numa responsabilidade.
package hotspot

import (
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"
)

// reapplyHotspotBlocklist reaplica todo bloqueio persistido depois que
// o hotspot sobe - a ACL do hostapd (modo "deauth") so existe na
// memoria do processo, e as regras DROP do modo "traffic" ficam no
// chain BINDNET-HOTSPOT, que o proprio hotspot recria (flush) a cada
// start/apply - nenhum dos dois sobrevive a um restart sozinho.
func reapplyHotspotBlocklist(ctx context.Context, db *sql.DB, worker *workerapi.Client, iface string) {
	blocked, err := listHotspotBlockedDevices(db)
	if err != nil {
		log.Printf("[backend] falha ao listar blocklist do hotspot: %v", err)
		return
	}
	for _, device := range blocked {
		var lastErr error
		for attempt := 0; attempt < 6; attempt++ {
			lastErr = applyLiveBlockRequest(ctx, worker, iface, device.MACAddress, device.Mode, true)
			if lastErr == nil {
				break
			}
			time.Sleep(time.Second)
		}
		if lastErr != nil {
			log.Printf("[backend] bloqueio de %s persistido, mas aplicacao ao vivo falhou: %v", device.MACAddress, lastErr)
		}
	}
}

// applyLiveBlockForMode escolhe o mecanismo certo conforme o modo:
// "deauth" usa hostapd (deny_acl+deauth, derruba do Wi-Fi); "traffic"
// usa DROP via iptables (ver services/worker/controller/traffic_block.go),
// dispositivo continua associado. So loga em caso de falha - a
// blocklist ja foi persistida no Postgres de qualquer forma.
func applyLiveBlockForMode(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac, mode string, block bool) {
	iface, err := store.HotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] blocklist persistida, mas nao foi possivel ler WIFI_INTERFACE: %v", err)
		return
	}
	if err := applyLiveBlockRequest(ctx, worker, iface, mac, mode, block); err != nil {
		log.Printf("[backend] blocklist persistida, mas aplicacao ao vivo de %s falhou: %v", mac, err)
	}
}

// applyLiveBlockRequest e o nivel baixo compartilhado por
// applyLiveBlockForMode (best-effort, so loga) e reapplyHotspotBlocklist
// (com retry, precisa do erro). Resolve o IP atual quando
// mode="traffic" e block=true (a regra de download precisa dele) - se
// o dispositivo nao estiver conectado agora, o upload ja bloqueado
// basta por enquanto; o proximo ciclo de reconciliacao completa o
// download assim que ele reconectar.
func applyLiveBlockRequest(ctx context.Context, worker *workerapi.Client, iface, mac, mode string, block bool) error {
	if mode == "traffic" {
		ip := ""
		if block {
			ip, _ = liveHotspotClientIP(ctx, worker, iface, mac)
		}
		return applyLiveTrafficBlockRequest(ctx, worker, iface, mac, ip, block)
	}
	path := "/hotspot/unblock"
	if block {
		path = "/hotspot/block"
	}
	return worker.Call(ctx, http.MethodPost, path, map[string]string{"interface": iface, "mac": mac}, nil)
}

// applyLiveTrafficBlock aplica (ou remove) o bloqueio de trafego -
// diferente do modo "deauth" (hostapd deny_acl+deauth, ver
// applyLiveBlockRequest), aqui o dispositivo continua associado ao
// Wi-Fi, so o trafego para de passar (DROP via iptables no worker, ver
// services/worker/controller/traffic_block.go). Usado tanto pelo
// bloqueio manual em modo "traffic" quanto pelo bloqueio automatico
// por falta de credito (hotspot_reconcile.go/hotspot_credit_recharge.go).
// ip so e necessario para block=true (a regra de download precisa do
// IP atual); no unblock a remocao e so por comentario, sem IP.
func applyLiveTrafficBlock(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac, ip string, block bool) {
	iface, err := store.HotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] bloqueio de trafego persistido, mas nao foi possivel ler WIFI_INTERFACE: %v", err)
		return
	}
	if err := applyLiveTrafficBlockRequest(ctx, worker, iface, mac, ip, block); err != nil {
		log.Printf("[backend] bloqueio de trafego persistido, mas aplicacao ao vivo de %s falhou: %v", mac, err)
	}
}

func applyLiveTrafficBlockRequest(ctx context.Context, worker *workerapi.Client, iface, mac, ip string, block bool) error {
	path := "/hotspot/trafficunblock"
	payload := map[string]string{"interface": iface, "mac": mac}
	if block {
		path = "/hotspot/trafficblock"
		payload["ip"] = ip
	}
	return worker.Call(ctx, http.MethodPost, path, payload, nil)
}
