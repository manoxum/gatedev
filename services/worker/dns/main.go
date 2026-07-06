// Comando dns-provider e um servidor DNS split-horizon: para os TLDs
// locais (DNS_LOCAL_TLDS) responde de forma diferente conforme o IP em
// que a consulta chegou (host/container/hotspot, ver server.go:view) e
// encaminha qualquer outro dominio para DNS publico. Substitui o antigo
// wrapper baseado em CoreDNS+Corefile por um binario Go proprio, unica
// forma pratica de implementar as tres views + alocacao persistente de
// IP de loopback por hostname (Postgres, com cache Redis).
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// hostname devolve o hostname do container, usado como valor padrao de
// DISCOVER_NODE_NAME quando a variavel nao e definida.
func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "no-desconhecido"
	}
	return name
}

func main() {
	log.SetFlags(log.LstdFlags)
	log.Println("[dns-provider] iniciando servidor DNS split-horizon")

	tlds, err := parseTLDs(getenv("DNS_LOCAL_TLDS", "local,test,example"))
	if err != nil {
		log.Fatalf("[dns-provider] %v", err)
	}
	log.Printf("[dns-provider] TLDs locais resolvidos: %v", zoneNames(tlds))

	domainZones, err := parseOptionalDomains(os.Getenv("DOMAINS"), "DOMAINS")
	if err != nil {
		log.Fatalf("[dns-provider] %v", err)
	}
	if len(domainZones) > 0 {
		log.Printf("[dns-provider] zonas declaradas em DOMAINS: %v", zoneNames(domainZones))
	}
	nginxNames := loadNginxNames(getenv("NGINX_CONFIG_PATH", "/nginx-config"))
	if len(nginxNames.hosts) > 0 || len(nginxNames.zones) > 0 {
		log.Printf("[dns-provider] server_name do nginx-ui descobertos: hosts=%v zonas=%v", zoneNames(nginxNames.hosts), zoneNames(nginxNames.zones))
	}

	db, err := openPostgres()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao conectar no Postgres: %v", err)
	}
	defer db.Close()

	gatewayCtx, gatewayCancel := context.WithTimeout(context.Background(), 10*time.Second)
	hotspotGateway, err := loadHotspotGateway(gatewayCtx, db, "192.168.12.1")
	gatewayCancel()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao ler HOTSPOT_GATEWAY de hotspot_config: %v", err)
	}
	log.Printf("[dns-provider] gateway do hotspot carregado do banco: %s", hotspotGateway)

	hostSourceIPs, err := discoverHostSourceIPs(os.Getenv("HOST_SOURCE_CIDR"), "127.0.0.1", hotspotGateway)
	if err != nil {
		log.Fatalf("[dns-provider] erro ao interpretar HOST_SOURCE_CIDR: %v", err)
	}
	if len(hostSourceIPs) > 0 {
		if strings.TrimSpace(os.Getenv("HOST_SOURCE_CIDR")) == "" {
			log.Printf("[dns-provider] IPs LAN/peer detectados automaticamente: %v", hostSourceIPs)
		} else {
			log.Printf("[dns-provider] IPs LAN/peer detectados via HOST_SOURCE_CIDR: %v", hostSourceIPs)
		}
	}

	dockerExcludes := append([]string{hotspotGateway}, hostSourceIPs...)
	dockerGateways, err := discoverDockerGateways(dockerExcludes...)
	if err != nil {
		log.Fatalf("[dns-provider] erro ao descobrir gateways Docker locais: %v", err)
	}
	if len(dockerGateways) == 0 {
		log.Printf("[dns-provider] aviso: nenhum gateway Docker local detectado; a view container nao sera aberta")
	} else {
		log.Printf("[dns-provider] gateways Docker detectados para a view container: %v", dockerGateways)
	}

	timeout := 90 * time.Second
	if raw := os.Getenv("COREDNS_WAIT_TIMEOUT"); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil {
			timeout = seconds
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	cache, err := openRedis(ctx)
	cancel()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao conectar no Redis: %v", err)
	}

	hydrateCtx, hydrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := hydrateCache(hydrateCtx, db, cache); err != nil {
		log.Printf("[dns-provider] aviso: falha ao hidratar cache Redis a partir do Postgres: %v", err)
	}
	hydrateCancel()

	nodeName := getenv("DISCOVER_NODE_NAME", hostname())
	identityCtx, identityCancel := context.WithTimeout(context.Background(), 10*time.Second)
	fingerprint, err := ensureNodeFingerprint(identityCtx, db)
	identityCancel()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao garantir fingerprint local: %v", err)
	}
	remoteMode := normalizeRemoteRouteMode(getenv("DISCOVER_REMOTE_ROUTES", "auto"))
	discoverPort := getenv("DISCOVER_PORT", "8531")

	routes := newRouteTable()
	routesCtx, routesCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if loaded, err := loadAllRoutes(routesCtx, db); err != nil {
		log.Printf("[dns-provider] aviso: falha ao carregar tabela de descoberta do Postgres: %v", err)
	} else {
		routes.replace(loaded)
	}
	routesCancel()

	cfg := &dnsConfig{
		tlds:        tlds,
		domainZones: domainZones,
		nginxHosts:  nginxNames.hosts,
		nginxZones:  nginxNames.zones,
		db:          db,
		cache:       cache,
		routes:      routes,
		nodeName:    nodeName,
		fingerprint: fingerprint,
		remoteMode:  remoteMode,
	}
	log.Printf("[dns-provider] no de descoberta '%s' fingerprint=%s, rotas remotas: %s", nodeName, fingerprint, remoteMode)

	// O servidor HTTP de descoberta e o poll de peers nao dependem de
	// nenhuma interface/IP de rede do host - sobem incondicionalmente,
	// assim que Postgres/Redis/fingerprint estiverem prontos, para que um
	// gateway Docker ou HOST_SOURCE_CIDR ausente/desatualizado nunca torne
	// este no invisivel para quem esta tentando descobri-lo.
	go startDiscoverServer(cfg, discoverPort)
	go pollPeers(cfg)

	for _, gateway := range dockerGateways {
		gateway := gateway
		go serveWhenIPAvailable(gateway, "gateway Docker", timeout, newHandler(cfg, viewContainer, gateway))
	}
	for _, hostIP := range hostSourceIPs {
		hostIP := hostIP
		go serveWhenIPAvailable(hostIP, "peer/LAN (HOST_SOURCE_CIDR)", timeout, newHandler(cfg, viewContainer, hostIP))
	}
	go serveWhenIPAvailable(hotspotGateway, "hotspot", timeout, newHandler(cfg, viewHotspot, hotspotGateway))

	log.Fatalf("[dns-provider] erro no servidor: %v", serve("127.0.0.1:53", newHandler(cfg, viewHost, "")))
}

// serveWhenIPAvailable espera o IP informado existir como endereco real do
// host antes de abrir o socket DNS - nunca fatal: se o IP nao aparecer a
// tempo (gateway Docker/HOST_SOURCE_CIDR desatualizado, hotspot ainda nao
// subiu), so loga e tenta de novo, sem derrubar o processo nem o servidor
// de descoberta (ver comentario acima em main()).
func serveWhenIPAvailable(ip, description string, timeout time.Duration, handler dns.HandlerFunc) {
	for {
		log.Printf("[dns-provider] aguardando IP de %s (%s) para abrir DNS em %s:53", description, ip, ip)
		if err := waitForIPs([]string{ip}, timeout); err != nil {
			log.Printf("[dns-provider] aviso: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if err := serve(ip+":53", handler); err != nil {
			log.Printf("[dns-provider] aviso: DNS de %s (%s) encerrou: %v", description, ip, err)
			time.Sleep(5 * time.Second)
		}
	}
}
