package discover

import (
	"context"
	"log"
	"time"

	"bindnet/dns-provider/internal/core"
	"bindnet/dns-provider/internal/store"
)

const (
	pollInterval = 15 * time.Second
	maxHops      = 16
	staleAfter   = 3 * pollInterval
)

// PollPeers roda para sempre, consultando cada peer direto configurado no
// Postgres a cada pollInterval e substituindo o snapshot em memoria da
// tabela de descoberta - nunca bloqueia o caminho de resolucao DNS, que so
// le o snapshot mais recente via core.Table.LookupSuffix.
func PollPeers(cfg *core.Config) {
	for {
		pollOnce(cfg)
		time.Sleep(pollInterval)
	}
}

// pollOnce executa um ciclo de troca de vetor de distancia: consulta cada
// peer, mescla o que veio de volta com o que ja era conhecido (regra: so
// substitui uma rota existente se ela vier do mesmo peer - refresh - ou se
// a nova distancia for menor), e marca como "stale" (sem apagar) qualquer
// rota que nao foi reconfirmada por tempo demais.
func pollOnce(cfg *core.Config) {
	updated := ownRoutes(cfg)
	previous := map[string]core.Route{}
	for _, route := range cfg.Routes.Snapshot() {
		previous[route.Domain] = route
	}

	peers := effectivePeers(cfg)
	for _, peer := range peers {
		advertised, err := fetchRoutes(peer)
		if err != nil {
			log.Printf("[dns-provider] aviso: falha ao consultar peer de descoberta %s: %v", peer, err)
			continue
		}
		mergeAdvertised(cfg, updated, peer, advertised)
	}

	markStaleOrCarryOver(previous, updated, peers)
	cfg.Routes.Replace(updated)
	persistSnapshot(cfg, updated)
}

// effectivePeers devolve apenas os peers configurados pelo operador no
// banco. A busca de novos peers e uma acao explicita do painel; nenhum
// servidor entra na malha so por ter sido visto na rede.
func effectivePeers(cfg *core.Config) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	configured, err := store.LoadConfiguredPeers(ctx, cfg.DB)
	if err != nil {
		log.Printf("[dns-provider] aviso: falha ao carregar peers configurados: %v", err)
		return nil
	}
	seen := map[string]bool{}
	var peers []string
	for _, peer := range configured {
		if seen[peer] {
			continue
		}
		seen[peer] = true
		peers = append(peers, peer)
	}
	return peers
}

// mergeAdvertised aplica as rotas anunciadas por um peer no mapa "updated"
// (regra de vetor de distancia: distancia+1, nunca aprender uma rota de
// volta pro proprio dono, limite de saltos, so substitui uma rota
// existente vinda de outro peer se a nova distancia for menor).
func mergeAdvertised(cfg *core.Config, updated map[string]core.Route, peer string, advertised []advertisedRoute) {
	for _, adv := range advertised {
		if adv.OwnerFingerprint != "" && adv.OwnerFingerprint == cfg.Fingerprint {
			continue // nunca aprender uma rota de volta para a propria identidade persistente
		}
		if adv.Owner == cfg.NodeName {
			continue // nunca aprender uma rota de volta para si mesmo
		}
		if cfg.RemoteMode == "manual" && adv.Distance > 0 {
			continue // vizinhos remotos so entram quando adicionados manualmente pelo painel
		}
		newDistance := adv.Distance + 1
		if newDistance > maxHops {
			continue
		}
		if _, ok := ownRoutes(cfg)[adv.Domain]; ok {
			continue // dominio anunciado localmente sempre vence
		}
		existing, exists := updated[adv.Domain]
		if exists && existing.Source != peer && newDistance >= existing.Distance {
			continue
		}
		updated[adv.Domain] = core.Route{
			Domain:           adv.Domain,
			Owner:            adv.Owner,
			OwnerFingerprint: adv.OwnerFingerprint,
			NextHop:          hostOf(peer),
			Distance:         newDistance,
			Source:           peer,
			State:            core.StateOK,
			LastSeen:         time.Now(),
		}
	}
}

// markStaleOrCarryOver preserva, no mapa "updated", qualquer rota
// aprendida antes que nao foi reconfirmada neste ciclo - marcando como
// "stale" (nunca apagando) se ja passou tempo demais sem confirmacao.
func markStaleOrCarryOver(previous, updated map[string]core.Route, activePeers []string) {
	activeSources := map[string]bool{}
	for _, peer := range activePeers {
		activeSources[peer] = true
	}
	for domain, old := range previous {
		if _, present := updated[domain]; present {
			continue
		}
		if old.Source == "self" {
			continue
		}
		if !activeSources[old.Source] {
			continue
		}
		if time.Since(old.LastSeen) > staleAfter {
			old.State = core.StateStale
		}
		updated[domain] = old
	}
}

func persistSnapshot(cfg *core.Config, updated map[string]core.Route) {
	if cfg.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	routes := make([]core.Route, 0, len(updated))
	for _, route := range updated {
		routes = append(routes, route)
	}
	if err := store.PersistRoutes(ctx, cfg.DB, routes); err != nil {
		log.Printf("[dns-provider] aviso: falha ao persistir tabela de descoberta: %v", err)
	}
}
