package main

import (
	"log"
	"os"
	"time"

	"github.com/miekg/dns"

	"bindnet/dns-provider/internal/dnsserver"
	"bindnet/dns-provider/internal/netdetect"
)

// serveWhenIPAvailable espera o IP informado existir como endereco real do
// host antes de abrir o socket DNS - nunca fatal: se o IP nao aparecer a
// tempo (gateway Docker/HOST_SOURCE_CIDR desatualizado, hotspot ainda nao
// subiu), so loga e tenta de novo, sem derrubar o processo nem o servidor
// de descoberta (ver comentario em main()).
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

// hostname devolve o hostname do container, usado como valor padrao do nome
// do no de descoberta quando nada foi configurado no painel.
func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "no-desconhecido"
	}
	return name
}
