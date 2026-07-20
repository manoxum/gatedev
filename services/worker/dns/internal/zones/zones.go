// Package zones decide como o dns-provider resolve cada nome: zona local
// (loopback persistente ou IP da view), proximo salto da malha, mesh
// desconhecido (NXDOMAIN) ou fora de qualquer zona (encaminhar).
package zones

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"

	"bindnet/dns-provider/internal/cache"
	"bindnet/dns-provider/internal/core"
	"bindnet/dns-provider/internal/store"
)

type Kind int

const (
	None Kind = iota
	Local
	Remote      // dono e outro no da malha, rota conhecida -> proxy pro proximo salto
	MeshUnknown // dentro de DOMAINS, mas sem dono local nem rota -> NXDOMAIN
)

// For decide como resolver um nome. Alem da zona/tipo, devolve o proximo
// salto quando kind == Remote - so nesse caso o valor e significativo.
func For(fqdn string, cfg *core.Config) (zone string, kind Kind, nextHop string) {
	labels := dns.SplitDomainName(fqdn)
	if len(labels) == 0 {
		return fqdn, None, ""
	}

	name := strings.Join(labels, ".")
	if cfg.NginxHosts[name] {
		return name + ".", Local, ""
	}
	if zone, ok := SuffixZoneFor(labels, cfg.NginxZones); ok {
		return zone + ".", Local, ""
	}
	if zone, ok := SuffixZoneFor(labels, cfg.DomainZones); ok {
		if zone == name || IsConcreteOwnedDomainZone(zone) {
			return zone + ".", Local, ""
		}
	}
	if zone, route, ok := cfg.Routes.LookupSuffix(labels); ok {
		if route.Source == "self" {
			return zone + ".", Local, ""
		}
		if route.NextHop != "" {
			return zone + ".", Remote, route.NextHop
		}
		return zone + ".", MeshUnknown, ""
	}
	// TLDs locais casam por sufixo em qualquer profundidade ("local",
	// "local.com", "a.b.local"), e um sufixo declarado explicitamente em
	// DNS_LOCAL_TLDS vence o fallback mesh-unknown de DOMAINS abaixo.
	if zone, ok := SuffixZoneFor(labels, cfg.TLDs); ok {
		return zone + ".", Local, ""
	}
	if zone, ok := SuffixZoneFor(labels, cfg.DomainZones); ok {
		return zone + ".", MeshUnknown, ""
	}
	return labels[len(labels)-1] + ".", None, ""
}

func SuffixZoneFor(labels []string, domains map[string]bool) (string, bool) {
	for i := 0; i < len(labels); i++ {
		zone := strings.Join(labels[i:], ".")
		if domains[zone] {
			return zone, true
		}
	}
	return "", false
}

func IsConcreteOwnedDomainZone(zone string) bool {
	return strings.Contains(zone, ".")
}

// AnswerIPFor resolve o IP de resposta conforme a view: container/hotspot
// respondem com o IP do socket que recebeu a consulta; host usa loopback
// persistente por hostname.
func AnswerIPFor(cfg *core.Config, v core.View, kind Kind, name string, responseIP string) (net.IP, error) {
	switch v {
	case core.ViewContainer:
		ip := net.ParseIP(responseIP)
		if ip == nil {
			return nil, fmt.Errorf("gateway Docker invalido para resposta: %q", responseIP)
		}
		return ip, nil
	case core.ViewHotspot:
		ip := net.ParseIP(responseIP)
		if ip == nil {
			return nil, fmt.Errorf("gateway do hotspot invalido para resposta: %q", responseIP)
		}
		return ip, nil
	default:
		return loopbackIPFor(cfg, name)
	}
}

func loopbackIPFor(cfg *core.Config, name string) (net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if offset, ok := cache.Offset(ctx, cfg.Cache, name); ok {
		return offsetToLoopback(offset), nil
	}

	offset, err := store.GetOrAllocateOffset(ctx, cfg.DB, name)
	if err != nil {
		return nil, err
	}
	cache.StoreOffset(ctx, cfg.Cache, name, offset)
	return offsetToLoopback(offset), nil
}

// offsetToLoopback converte um offset (sequence do Postgres, comeca em 2)
// num IP dentro de 127.0.0.0/8 - os 24 bits menos significativos do offset
// viram os tres ultimos octetos do IP.
func offsetToLoopback(offset int64) net.IP {
	b := uint32(offset) & 0xFFFFFF
	return net.IPv4(127, byte(b>>16), byte(b>>8), byte(b))
}
