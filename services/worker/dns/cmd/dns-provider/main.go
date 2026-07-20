// Comando dns-provider e um servidor DNS split-horizon: para os TLDs
// locais (DNS_LOCAL_TLDS) responde de forma diferente conforme o IP em que
// a consulta chegou (host/container/hotspot, ver internal/core.View) e
// encaminha qualquer outro dominio para DNS publico. Substitui o antigo
// wrapper baseado em CoreDNS+Corefile por um binario Go proprio, unica
// forma pratica de implementar as tres views + alocacao persistente de IP
// de loopback por hostname (Postgres, com cache Redis).
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"

	"bindnet/dns-provider/internal/cache"
	"bindnet/dns-provider/internal/config"
	"bindnet/dns-provider/internal/core"
	"bindnet/dns-provider/internal/discover"
	"bindnet/dns-provider/internal/dnsserver"
	"bindnet/dns-provider/internal/netdetect"
	"bindnet/dns-provider/internal/nginx"
	"bindnet/dns-provider/internal/store"
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

	tlds, err := config.ParseTLDs(config.Getenv("DNS_LOCAL_TLDS", "local,test,example"))
	if err != nil {
		log.Fatalf("[dns-provider] %v", err)
	}
	log.Printf("[dns-provider] TLDs locais resolvidos: %v", config.ZoneNames(tlds))

	domainZones, err := config.ParseOptionalDomains(os.Getenv("DOMAINS"), "DOMAINS")
	if err != nil {
		log.Fatalf("[dns-provider] %v", err)
	}
	if len(domainZones) > 0 {
		log.Printf("[dns-provider] zonas declaradas em DOMAINS: %v", config.ZoneNames(domainZones))
	}
	nginxNames := nginx.LoadNames(config.Getenv("NGINX_CONFIG_PATH", "/nginx-config"))
	if len(nginxNames.Hosts) > 0 || len(nginxNames.Zones) > 0 {
		log.Printf("[dns-provider] server_name do nginx-ui descobertos: hosts=%v zonas=%v", config.ZoneNames(nginxNames.Hosts), config.ZoneNames(nginxNames.Zones))
	}

	db, err := store.OpenPostgres()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao conectar no Postgres: %v", err)
	}
	defer db.Close()

	gatewayCtx, gatewayCancel := context.WithTimeout(context.Background(), 10*time.Second)
	hotspotGateway, err := store.LoadHotspotGateway(gatewayCtx, db, "192.168.12.1")
	gatewayCancel()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao ler HOTSPOT_GATEWAY de hotspot_config: %v", err)
	}
	log.Printf("[dns-provider] gateway do hotspot carregado do banco: %s", hotspotGateway)

	hostSourceIPs, err := netdetect.DiscoverHostSourceIPs(os.Getenv("HOST_SOURCE_CIDR"), "127.0.0.1", hotspotGateway)
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
	dockerGateways, err := netdetect.DiscoverDockerGateways(dockerExcludes...)
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
	redisCache, err := cache.Open(ctx)
	cancel()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao conectar no Redis: %v", err)
	}

	hydrateCtx, hydrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := cache.Hydrate(hydrateCtx, db, redisCache); err != nil {
		log.Printf("[dns-provider] aviso: falha ao hidratar cache Redis a partir do Postgres: %v", err)
	}
	hydrateCancel()

	nodeName := config.Getenv("DISCOVER_NODE_NAME", hostname())
	identityCtx, identityCancel := context.WithTimeout(context.Background(), 10*time.Second)
	fingerprint, err := store.EnsureNodeFingerprint(identityCtx, db)
	identityCancel()
	if err != nil {
		log.Fatalf("[dns-provider] erro ao garantir fingerprint local: %v", err)
	}
	remoteMode := config.NormalizeRemoteRouteMode(config.Getenv("DISCOVER_REMOTE_ROUTES", "auto"))
	discoverPort := config.Getenv("DISCOVER_PORT", "8531")

	routes := core.NewTable()
	routesCtx, routesCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if loaded, err := store.LoadAllRoutes(routesCtx, db); err != nil {
		log.Printf("[dns-provider] aviso: falha ao carregar tabela de descoberta do Postgres: %v", err)
	} else {
		routes.Replace(loaded)
	}
	routesCancel()

	cfg := &core.Config{
		TLDs:        tlds,
		DomainZones: domainZones,
		NginxHosts:  nginxNames.Hosts,
		NginxZones:  nginxNames.Zones,
		DB:          db,
		Cache:       redisCache,
		Routes:      routes,
		NodeName:    nodeName,
		Fingerprint: fingerprint,
		RemoteMode:  remoteMode,
	}
	log.Printf("[dns-provider] no de descoberta '%s' fingerprint=%s, rotas remotas: %s", nodeName, fingerprint, remoteMode)

	// O servidor HTTP de descoberta e o poll de peers nao dependem de
	// nenhuma interface/IP de rede do host - sobem incondicionalmente,
	// assim que Postgres/Redis/fingerprint estiverem prontos, para que um
	// gateway Docker ou HOST_SOURCE_CIDR ausente/desatualizado nunca torne
	// este no invisivel para quem esta tentando descobri-lo.
	go discover.StartServer(cfg, discoverPort)
	go discover.PollPeers(cfg)

	for _, gateway := range dockerGateways {
		gateway := gateway
		go serveWhenIPAvailable(gateway, "gateway Docker", timeout, dnsserver.NewHandler(cfg, core.ViewContainer, gateway))
	}
	for _, hostIP := range hostSourceIPs {
		hostIP := hostIP
		go serveWhenIPAvailable(hostIP, "peer/LAN (HOST_SOURCE_CIDR)", timeout, dnsserver.NewHandler(cfg, core.ViewContainer, hostIP))
	}
	go serveWhenIPAvailable(hotspotGateway, "hotspot", timeout, dnsserver.NewHandler(cfg, core.ViewHotspot, hotspotGateway))

	log.Fatalf("[dns-provider] erro no servidor: %v", dnsserver.Serve("127.0.0.1:53", dnsserver.NewHandler(cfg, core.ViewHost, "")))
}

// serveWhenIPAvailable espera o IP informado existir como endereco real do
// host antes de abrir o socket DNS - nunca fatal: se o IP nao aparecer a
// tempo (gateway Docker/HOST_SOURCE_CIDR desatualizado, hotspot ainda nao
// subiu), so loga e tenta de novo, sem derrubar o processo nem o servidor
// de descoberta (ver comentario acima em main()).
func serveWhenIPAvailable(ip, description string, timeout time.Duration, handler dns.HandlerFunc) {
	for {
		log.Printf("[dns-provider] aguardando IP de %s (%s) para abrir DNS em %s:53", description, ip, ip)
		if err := netdetect.WaitForIPs([]string{ip}, timeout); err != nil {
			log.Printf("[dns-provider] aviso: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if err := dnsserver.Serve(ip+":53", handler); err != nil {
			log.Printf("[dns-provider] aviso: DNS de %s (%s) encerrou: %v", description, ip, err)
			time.Sleep(5 * time.Second)
		}
	}
}
