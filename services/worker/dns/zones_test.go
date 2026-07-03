package main

import "testing"

func TestZoneForExactDomainZoneIsLocal(t *testing.T) {
	cfg := &dnsConfig{
		tlds:        map[string]bool{"local": true},
		domainZones: map[string]bool{"costa.bnet": true},
		nginxHosts:  map[string]bool{},
		nginxZones:  map[string]bool{},
		routes:      newRouteTable(),
	}

	zone, kind, nextHop := zoneFor("costa.bnet.", cfg)
	if zone != "costa.bnet." || kind != zoneLocal || nextHop != "" {
		t.Fatalf("zoneFor(costa.bnet.) = (%q, %v, %q), want local costa.bnet.", zone, kind, nextHop)
	}
}

func TestZoneForSubdomainInsideConcreteDomainZoneIsLocal(t *testing.T) {
	cfg := &dnsConfig{
		tlds:        map[string]bool{"local": true},
		domainZones: map[string]bool{"costa.bnet": true},
		nginxHosts:  map[string]bool{},
		nginxZones:  map[string]bool{},
		routes:      newRouteTable(),
	}

	zone, kind, nextHop := zoneFor("app.costa.bnet.", cfg)
	if zone != "costa.bnet." || kind != zoneLocal || nextHop != "" {
		t.Fatalf("zoneFor(app.costa.bnet.) = (%q, %v, %q), want local costa.bnet.", zone, kind, nextHop)
	}
}

func TestZoneForUnknownNameInsideBroadMeshZoneStaysUnknown(t *testing.T) {
	cfg := &dnsConfig{
		tlds:        map[string]bool{"local": true},
		domainZones: map[string]bool{"bnet": true},
		nginxHosts:  map[string]bool{},
		nginxZones:  map[string]bool{},
		routes:      newRouteTable(),
	}

	zone, kind, nextHop := zoneFor("app.unknown.bnet.", cfg)
	if zone != "bnet." || kind != zoneMeshUnknown || nextHop != "" {
		t.Fatalf("zoneFor(app.unknown.bnet.) = (%q, %v, %q), want unknown bnet.", zone, kind, nextHop)
	}
}

func TestZoneForRemoteRouteMatchesSubdomains(t *testing.T) {
	routes := newRouteTable()
	routes.replace(map[string]discoveredRoute{
		"ahmed.bnet": {
			Domain:  "ahmed.bnet",
			Owner:   "Ahmed",
			NextHop: "10.234.2.140",
			Source:  "10.234.2.140:8531",
			State:   routeStateOK,
		},
	})
	cfg := &dnsConfig{
		tlds:        map[string]bool{"local": true},
		domainZones: map[string]bool{"costa.bnet": true},
		nginxHosts:  map[string]bool{},
		nginxZones:  map[string]bool{},
		routes:      routes,
	}

	zone, kind, nextHop := zoneFor("test.ahmed.bnet.", cfg)
	if zone != "ahmed.bnet." || kind != zoneRemote || nextHop != "10.234.2.140" {
		t.Fatalf("zoneFor(test.ahmed.bnet.) = (%q, %v, %q), want remote ahmed.bnet via 10.234.2.140", zone, kind, nextHop)
	}
}

func TestOwnRoutesAdvertisesConcreteDomainZones(t *testing.T) {
	cfg := &dnsConfig{
		domainZones: map[string]bool{"costa.bnet": true, "bnet": true},
		nginxHosts:  map[string]bool{},
		nginxZones:  map[string]bool{},
		nodeName:    "Daniel Costa",
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
	cfg := &dnsConfig{
		domainZones: map[string]bool{"costa.bnet": true, "bnet": true},
		nginxHosts:  map[string]bool{"test.costa.bnet": true},
		nginxZones:  map[string]bool{},
		nodeName:    "Daniel Costa",
	}

	domains := routeDomains(ownRoutes(cfg))
	if len(domains) != 2 || domains[0] != "costa.bnet" || domains[1] != "test.costa.bnet" {
		t.Fatalf("routeDomains(ownRoutes) = %v, want [costa.bnet test.costa.bnet]", domains)
	}
}
