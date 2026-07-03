package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// advertisedRoute e o formato trocado entre peers via HTTP - so o
// essencial pra reconstruir o vetor de distancia do lado de quem
// recebe; "nextHop"/"source"/"state" sao sempre recalculados
// localmente por quem recebe, nunca repassados como vieram.
type advertisedRoute struct {
	Domain           string `json:"domain"`
	Owner            string `json:"owner"`
	OwnerFingerprint string `json:"ownerFingerprint,omitempty"`
	Distance         int    `json:"distance"`
}

type discoverInfo struct {
	NodeName    string   `json:"nodeName"`
	Fingerprint string   `json:"fingerprint"`
	Domains     []string `json:"domains"`
}

// ownRoutes calcula quais dominios este no anuncia como dono direto
// (distancia 0): todo nome anunciado localmente via nginx-ui
// (nginxHosts/nginxZones) que tambem participa da malha (cai dentro de
// algum sufixo de DOMAINS). Recalculada a cada ciclo de poll - barato o
// suficiente pra nao precisar cachear.
func ownRoutes(cfg *dnsConfig) map[string]discoveredRoute {
	owned := map[string]discoveredRoute{}
	now := time.Now()

	addOwned := func(name string) {
		owned[name] = discoveredRoute{
			Domain:           name,
			Owner:            cfg.nodeName,
			OwnerFingerprint: cfg.fingerprint,
			Distance:         0,
			Source:           "self",
			State:            routeStateOK,
			LastSeen:         now,
		}
	}

	addIfInMesh := func(name string) {
		if _, ok := suffixZoneFor(strings.Split(name, "."), cfg.domainZones); !ok {
			return
		}
		addOwned(name)
	}
	for zone := range cfg.domainZones {
		if isConcreteOwnedDomainZone(zone) {
			addOwned(zone)
		}
	}
	for host := range cfg.nginxHosts {
		addIfInMesh(host)
	}
	for zone := range cfg.nginxZones {
		addIfInMesh(zone)
	}
	return owned
}

// advertisedRoutesFor monta a resposta HTTP para um peer que esta
// consultando este no: rotas proprias + tudo que este no aprendeu da
// malha, exceto o que foi aprendido pelo proprio requisitante (split
// horizon - nunca devolve pra um vizinho uma rota que so existe porque
// foi ele quem a anunciou).
func advertisedRoutesFor(cfg *dnsConfig, requesterIP string) []advertisedRoute {
	var result []advertisedRoute
	seen := map[string]bool{}
	add := func(route discoveredRoute) {
		if seen[route.Domain] {
			return
		}
		seen[route.Domain] = true
		result = append(result, advertisedRoute{
			Domain:           route.Domain,
			Owner:            route.Owner,
			OwnerFingerprint: route.OwnerFingerprint,
			Distance:         route.Distance,
		})
	}
	for _, route := range ownRoutes(cfg) {
		add(route)
	}
	for _, route := range cfg.routes.snapshot() {
		if requesterIP != "" && hostOf(route.Source) == requesterIP {
			continue
		}
		add(route)
	}
	return result
}

// hostOf extrai o IP de um endereco "host:porta"; devolve o valor
// original se nao tiver esse formato (ex.: a origem sintetica "self",
// que nunca deve bater com um IP de requisitante de verdade).
func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// startDiscoverServer expoe a tabela de rotas deste no para os peers -
// unico servidor HTTP do dns-provider, que ate aqui so tinha sockets
// UDP:53. Sobe mesmo sem peer direto configurado, porque um no "folha"
// ainda precisa responder quando outro no o lista como vizinho.
func startDiscoverServer(cfg *dnsConfig, port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /discover/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discoverInfo{
			NodeName:    cfg.nodeName,
			Fingerprint: cfg.fingerprint,
			Domains:     routeDomains(ownRoutes(cfg)),
		})
	})
	mux.HandleFunc("GET /discover/routes", func(w http.ResponseWriter, r *http.Request) {
		requesterIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(advertisedRoutesFor(cfg, requesterIP))
	})
	addr := ":" + port
	log.Printf("[dns-provider] endpoint de descoberta escutando em %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[dns-provider] erro no servidor de descoberta: %v", err)
	}
}

func routeDomains(routes map[string]discoveredRoute) []string {
	domains := make([]string, 0, len(routes))
	for domain := range routes {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

// fetchRoutes consulta o endpoint de descoberta de um peer.
func fetchRoutes(peer string) ([]advertisedRoute, error) {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/discover/routes", peer))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var routes []advertisedRoute
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return nil, err
	}
	return routes, nil
}
