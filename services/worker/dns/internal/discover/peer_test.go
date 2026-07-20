package discover

import (
	"testing"

	"bindnet/dns-provider/internal/core"
)

func TestOwnRoutesAdvertisesConcreteDomainZones(t *testing.T) {
	cfg := &core.Config{
		DomainZones: map[string]bool{"costa.bnet": true, "bnet": true},
		NginxHosts:  map[string]bool{},
		NginxZones:  map[string]bool{},
		NodeName:    "Daniel Costa",
	}

	owned := ownRoutes(cfg)
	if _, ok := owned["costa.bnet"]; !ok {
		t.Fatalf("ownRoutes did not advertise concrete domain zone costa.bnet")
	}
	if _, ok := owned["bnet"]; ok {
		t.Fatalf("ownRoutes advertised broad mesh root bnet")
	}
}

func TestRouteDomainsUsesOwnedRoutes(t *testing.T) {
	cfg := &core.Config{
		DomainZones: map[string]bool{"costa.bnet": true, "bnet": true},
		NginxHosts:  map[string]bool{"test.costa.bnet": true},
		NginxZones:  map[string]bool{},
		NodeName:    "Daniel Costa",
	}

	domains := routeDomains(ownRoutes(cfg))
	if len(domains) != 2 || domains[0] != "costa.bnet" || domains[1] != "test.costa.bnet" {
		t.Fatalf("routeDomains(ownRoutes) = %v, want [costa.bnet test.costa.bnet]", domains)
	}
}
