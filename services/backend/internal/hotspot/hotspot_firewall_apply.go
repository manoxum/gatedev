// hotspot_firewall_apply.go liga o motor das zonas wan/local
// (hotspot_firewall_policy.go) ao worker. Diferente do isolamento
// (zona clients, que depende de ap_isolate/reinicio), as zonas wan e
// local sao puro iptables aplicado ao vivo enquanto o hotspot roda -
// entram no mesmo ciclo de reconciliacao, reaplicacao pos-start e
// teardown no stop.
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

type firewallApplyPayload struct {
	Interface   string             `json:"interface"`
	Enabled     bool               `json:"enabled"`
	WanPolicy   string             `json:"wanPolicy"`
	WanRules    []firewallZoneRule `json:"wanRules"`
	LocalPolicy string             `json:"localPolicy"`
	LocalRules  []firewallZoneRule `json:"localRules"`
}

func zonePolicy(config map[string]string, key string) string {
	if config[key] == "deny" {
		return "deny"
	}
	return "allow"
}

// applyFirewallLive reaplica as zonas wan/local agora, best-effort (so
// loga). Chamada em toda mutacao de regra/politica e a cada ciclo de
// reconciliacao - cobre cliente novo (expansao de regra por perfil) e
// reinicio de container.
func applyFirewallLive(ctx context.Context, db *sql.DB, worker *workerapi.Client) {
	if err := applyFirewall(ctx, db, worker); err != nil {
		log.Printf("[backend] aplicacao ao vivo do firewall (wan/local) falhou: %v", err)
	}
}

// applyCommRuleLive reaplica as tres zonas de uma vez - usado apos criar/
// editar/remover uma regra, que pode ser de qualquer zona (clients, wan
// ou local).
func applyCommRuleLive(ctx context.Context, db *sql.DB, worker *workerapi.Client) {
	applyIsolationLive(ctx, db, worker)
	applyFirewallLive(ctx, db, worker)
}

func applyFirewall(ctx context.Context, db *sql.DB, worker *workerapi.Client) error {
	iface, err := store.HotspotWifiInterface(ctx, db)
	if err != nil {
		return err
	}
	config, err := store.GetHotspotConfig(ctx, db)
	if err != nil {
		return err
	}
	clients, err := liveHotspotClients(ctx, worker, iface)
	if err != nil {
		return err
	}
	isolationClients := make([]isolationClient, len(clients))
	for i, client := range clients {
		isolationClients[i] = isolationClient{MAC: client.MAC, IP: client.IP}
	}

	profileRefs, err := store.HotspotDeviceProfileRefs(db)
	if err != nil {
		return err
	}
	profileOf := make(map[string]string, len(profileRefs))
	for mac, ref := range profileRefs {
		profileOf[mac] = ref.ID
	}

	rules, err := store.ListCommRules(db)
	if err != nil {
		return err
	}

	return worker.Call(ctx, http.MethodPost, "/hotspot/firewall/apply", firewallApplyPayload{
		Interface:   iface,
		Enabled:     true,
		WanPolicy:   zonePolicy(config, "FW_WAN_POLICY"),
		WanRules:    compileZoneRules(store.CommZoneWAN, isolationClients, profileOf, rules),
		LocalPolicy: zonePolicy(config, "FW_LOCAL_POLICY"),
		LocalRules:  compileZoneRules(store.CommZoneLocal, isolationClients, profileOf, rules),
	}, nil)
}

// teardownFirewallWorker manda o worker desmontar as chains wan/local -
// usado no stop do hotspot.
func teardownFirewallWorker(ctx context.Context, worker *workerapi.Client, iface string) error {
	return worker.Call(ctx, http.MethodPost, "/hotspot/firewall/apply", firewallApplyPayload{
		Interface: iface,
		Enabled:   false,
	}, nil)
}

// reapplyHotspotFirewall reconstroi as zonas wan/local depois que o
// hotspot sobe - mesmo retry com backoff de reapplyHotspotShaping.
func reapplyHotspotFirewall(ctx context.Context, db *sql.DB, worker *workerapi.Client) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		lastErr = applyFirewall(ctx, db, worker)
		if lastErr == nil {
			return
		}
		time.Sleep(time.Second)
	}
	log.Printf("[backend] reaplicacao do firewall (wan/local) falhou: %v", lastErr)
}
