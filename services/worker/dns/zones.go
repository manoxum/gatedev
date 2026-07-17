package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type zoneKind int

const (
	zoneNone zoneKind = iota
	zoneLocal
	zoneRemote      // dono e outro no da malha, rota conhecida -> proxy pro proximo salto
	zoneMeshUnknown // dentro de DOMAINS, mas sem dono local nem rota -> NXDOMAIN
)

// zoneFor decide como resolver um nome. Alem da zona/tipo, devolve o
// proximo salto quando kind == zoneRemote - so nesse caso o valor e
// significativo.
func zoneFor(fqdn string, cfg *dnsConfig) (zone string, kind zoneKind, nextHop string) {
	labels := dns.SplitDomainName(fqdn)
	if len(labels) == 0 {
		return fqdn, zoneNone, ""
	}

	name := strings.Join(labels, ".")
	if cfg.nginxHosts[name] {
		return name + ".", zoneLocal, ""
	}
	if zone, ok := suffixZoneFor(labels, cfg.nginxZones); ok {
		return zone + ".", zoneLocal, ""
	}
	if zone, ok := suffixZoneFor(labels, cfg.domainZones); ok {
		if zone == name || isConcreteOwnedDomainZone(zone) {
			return zone + ".", zoneLocal, ""
		}
	}
	if zone, route, ok := cfg.routes.lookupSuffix(labels); ok {
		if route.Source == "self" {
			return zone + ".", zoneLocal, ""
		}
		if route.NextHop != "" {
			return zone + ".", zoneRemote, route.NextHop
		}
		return zone + ".", zoneMeshUnknown, ""
	}
	// TLDs locais casam por sufixo em qualquer profundidade ("local",
	// "local.com", "a.b.local"), e um sufixo declarado explicitamente em
	// DNS_LOCAL_TLDS vence o fallback mesh-unknown de DOMAINS abaixo.
	if zone, ok := suffixZoneFor(labels, cfg.tlds); ok {
		return zone + ".", zoneLocal, ""
	}
	if zone, ok := suffixZoneFor(labels, cfg.domainZones); ok {
		return zone + ".", zoneMeshUnknown, ""
	}
	return labels[len(labels)-1] + ".", zoneNone, ""
}

func suffixZoneFor(labels []string, domains map[string]bool) (string, bool) {
	for i := 0; i < len(labels); i++ {
		zone := strings.Join(labels[i:], ".")
		if domains[zone] {
			return zone, true
		}
	}
	return "", false
}

func isConcreteOwnedDomainZone(zone string) bool {
	return strings.Contains(zone, ".")
}

// answerIPFor resolve o IP de resposta conforme a view: container/hotspot
// respondem com o IP do socket que recebeu a consulta; host usa loopback
// persistente por hostname.
func answerIPFor(cfg *dnsConfig, v view, kind zoneKind, name string, responseIP string) (net.IP, error) {
	switch v {
	case viewContainer:
		ip := net.ParseIP(responseIP)
		if ip == nil {
			return nil, fmt.Errorf("gateway Docker invalido para resposta: %q", responseIP)
		}
		return ip, nil
	case viewHotspot:
		ip := net.ParseIP(responseIP)
		if ip == nil {
			return nil, fmt.Errorf("gateway do hotspot invalido para resposta: %q", responseIP)
		}
		return ip, nil
	default:
		return loopbackIPFor(cfg, name)
	}
}

func loopbackIPFor(cfg *dnsConfig, name string) (net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if offset, ok := cachedOffset(ctx, cfg.cache, name); ok {
		return offsetToLoopback(offset), nil
	}

	offset, err := getOrAllocateOffset(ctx, cfg.db, name)
	if err != nil {
		return nil, err
	}
	storeOffset(ctx, cfg.cache, name, offset)
	return offsetToLoopback(offset), nil
}

// offsetToLoopback converte um offset (sequence do Postgres, comeca em 2)
// num IP dentro de 127.0.0.0/8 - os 24 bits menos significativos do
// offset viram os tres ultimos octetos do IP.
func offsetToLoopback(offset int64) net.IP {
	b := uint32(offset) & 0xFFFFFF
	return net.IPv4(127, byte(b>>16), byte(b>>8), byte(b))
}
