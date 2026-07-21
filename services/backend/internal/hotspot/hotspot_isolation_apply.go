// hotspot_isolation_apply.go liga o motor puro de
// hotspot_isolation_policy.go ao mundo real: le o interruptor
// CLIENT_ISOLATION, os clientes conectados, perfis e regras, compila os
// pares permitidos e manda o estado desejado completo pro worker
// (POST /hotspot/isolation/apply) - que materializa tudo no chain
// BINDNET-ISOLATION, idempotente e sem estado, igual ao shaping.
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

type isolationApplyPayload struct {
	Interface string             `json:"interface"`
	Enabled   bool               `json:"enabled"`
	Pairs     []firewallPairRule `json:"pairs"`
}

func isolationEnabled(ctx context.Context, db *sql.DB) (bool, error) {
	config, err := store.GetHotspotConfig(ctx, db)
	if err != nil {
		return false, err
	}
	return config["CLIENT_ISOLATION"] == "true", nil
}

// applyIsolationLive reaplica o isolamento agora, best-effort (so
// loga): chamada em toda mutacao que muda a politica (interruptor,
// regra, toggle interno de perfil, vinculo dispositivo->perfil) e a
// cada ciclo de reconciliacao - e o ciclo que cobre cliente novo
// conectando e renovacao de DHCP, em ate ~15s.
func applyIsolationLive(ctx context.Context, db *sql.DB, worker *workerapi.Client) {
	if err := applyIsolation(ctx, db, worker); err != nil {
		log.Printf("[backend] aplicacao ao vivo do isolamento falhou: %v", err)
	}
}

func applyIsolation(ctx context.Context, db *sql.DB, worker *workerapi.Client) error {
	iface, err := store.HotspotWifiInterface(ctx, db)
	if err != nil {
		return err
	}
	enabled, err := isolationEnabled(ctx, db)
	if err != nil {
		return err
	}
	if !enabled {
		return teardownIsolationWorker(ctx, worker, iface)
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

	profiles, err := store.ListProfiles(db)
	if err != nil {
		return err
	}
	internalAllow := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		internalAllow[profile.ID] = profile.AllowInternalCommunication
	}

	rules, err := store.ListCommRules(db)
	if err != nil {
		return err
	}

	return worker.Call(ctx, http.MethodPost, "/hotspot/isolation/apply", isolationApplyPayload{
		Interface: iface,
		Enabled:   true,
		Pairs:     compileClientsZonePairs(isolationClients, profileOf, internalAllow, rules),
	}, nil)
}

// teardownIsolationWorker manda o worker desmontar chain/sysctls do
// isolamento - usada com o interruptor desligado e no stop do hotspot.
func teardownIsolationWorker(ctx context.Context, worker *workerapi.Client, iface string) error {
	return worker.Call(ctx, http.MethodPost, "/hotspot/isolation/apply", isolationApplyPayload{
		Interface: iface,
		Enabled:   false,
	}, nil)
}

// reapplyHotspotIsolation reconstroi o isolamento depois que o hotspot
// sobe - mesmo retry com backoff de reapplyHotspotShaping, ja que
// ap0/bn-uplink podem demorar alguns segundos para existir.
func reapplyHotspotIsolation(ctx context.Context, db *sql.DB, worker *workerapi.Client) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		lastErr = applyIsolation(ctx, db, worker)
		if lastErr == nil {
			return
		}
		time.Sleep(time.Second)
	}
	log.Printf("[backend] reaplicacao do isolamento de clientes falhou: %v", lastErr)
}
