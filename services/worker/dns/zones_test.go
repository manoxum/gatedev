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

func TestZoneForMultiLabelLocalTLDMatchesAnyDepth(t *testing.T) {
	cfg := &dnsConfig{
		tlds:        map[string]bool{"local": true, "local.com": true, "a.b.local.org": true},
		domainZones: map[string]bool{},
		nginxHosts:  map[string]bool{},
		nginxZones:  map[string]bool{},
		routes:      newRouteTable(),
	}

	cases := []struct {
		name string
		zone string
		kind zoneKind
	}{
		{"foo.local.", "local.", zoneLocal},
		{"local.com.", "local.com.", zoneLocal},
		{"app.local.com.", "local.com.", zoneLocal},
		{"x.y.z.local.com.", "local.com.", zoneLocal},
		{"svc.a.b.local.org.", "a.b.local.org.", zoneLocal},
		{"other.com.", "com.", zoneNone},
		{"b.local.org.", "org.", zoneNone},
	}
	for _, tc := range cases {
		zone, kind, nextHop := zoneFor(tc.name, cfg)
		if zone != tc.zone || kind != tc.kind || nextHop != "" {
			t.Errorf("zoneFor(%s) = (%q, %v, %q), want (%q, %v, \"\")", tc.name, zone, kind, nextHop, tc.zone, tc.kind)
		}
	}
}

func TestZoneForLocalTLDWinsOverBroadMeshRoot(t *testing.T) {
	cfg := &dnsConfig{
		tlds:        map[string]bool{"apps.bnet": true},
		domainZones: map[string]bool{"bnet": true},
		nginxHosts:  map[string]bool{},
		nginxZones:  map[string]bool{},
		routes:      newRouteTable(),
	}

	zone, kind, _ := zoneFor("web.apps.bnet.", cfg)
	if zone != "apps.bnet." || kind != zoneLocal {
		t.Fatalf("zoneFor(web.apps.bnet.) = (%q, %v), want local apps.bnet.", zone, kind)
	}
	zone, kind, _ = zoneFor("web.other.bnet.", cfg)
	if zone != "bnet." || kind != zoneMeshUnknown {
		t.Fatalf("zoneFor(web.other.bnet.) = (%q, %v), want unknown bnet.", zone, kind)
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
