package main

import (
	"fmt"
	"strings"
)

// Validacao de DNS_LOCAL_TLDS e DOMAINS com as mesmas regras que o
// dns-provider aplica no boot (services/worker/dns/config.go). Sem esta
// checagem, o painel gravava no .env um valor que o dns-provider rejeita
// como erro fatal, deixando o container em loop de restart e derrubando
// toda a resolucao local.
//
// Um "TLD local" pode ser um label simples ("local") ou um sufixo com
// varios labels ("local.com", "a.b.local"): qualquer nome que termine
// nesse sufixo resolve como zona local. Prefixos "*."/"**."/"." sao
// descartados por normalizeDNSZone (dns_discovery.go).

func validateLocalTLDs(raw string) error {
	valid := 0
	for _, part := range splitDNSList(raw) {
		tld := normalizeDNSZone(part)
		if tld == "" {
			continue
		}
		if !isValidDNSDomain(tld) {
			return fmt.Errorf("TLD local invalido: '%s' - cada label usa apenas letras minusculas, numeros e hifen (sem hifen nas pontas)", strings.TrimSpace(part))
		}
		valid++
	}
	if valid == 0 {
		return fmt.Errorf("DNS_LOCAL_TLDS deve conter pelo menos um TLD valido")
	}
	return nil
}

// validateDomains aceita lista vazia (desliga o discover mode).
func validateDomains(raw string) error {
	for _, part := range splitDNSList(raw) {
		domain := normalizeDNSZone(part)
		if domain == "" {
			continue
		}
		if !isValidDNSDomain(domain) {
			return fmt.Errorf("dominio invalido em DOMAINS: '%s' - cada label usa apenas letras minusculas, numeros e hifen (sem hifen nas pontas)", strings.TrimSpace(part))
		}
	}
	return nil
}

func splitDNSList(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' })
}
