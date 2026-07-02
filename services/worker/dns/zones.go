package main

import (
	"context"
	"net"
	"time"

	"github.com/miekg/dns"
)

type zoneKind int

const (
	zoneNone zoneKind = iota
	zoneLocal
	zoneDiscover
)

func zoneFor(fqdn string, cfg *dnsConfig) (string, zoneKind) {
	labels := dns.SplitDomainName(fqdn)
	if len(labels) == 0 {
		return fqdn, zoneNone
	}

	tld := labels[len(labels)-1]
	zone := tld + "."
	if cfg.discoverDomains[tld] {
		return zone, zoneDiscover
	}
	if cfg.tlds[tld] {
		return zone, zoneLocal
	}
	return zone, zoneNone
}

// answerIPFor resolve o IP de resposta conforme o tipo de zona. Zonas
// discover sempre apontam para o IP LAN desta instancia; zonas locais
// continuam usando split-horizon por view.
func answerIPFor(cfg *dnsConfig, v view, kind zoneKind, name string) (net.IP, error) {
	if kind == zoneDiscover {
		return cfg.discoverIP, nil
	}

	switch v {
	case viewContainer:
		return net.ParseIP(cfg.dockerGateway), nil
	case viewHotspot:
		return net.ParseIP(cfg.hotspotGateway), nil
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
