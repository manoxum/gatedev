package zones

import (
	"testing"

	"bindnet/dns-provider/internal/core"
)

func TestZoneForExactDomainZoneIsLocal(t *testing.T) {
	cfg := &core.Config{
		TLDs:        map[string]bool{"local": true},
		DomainZones: map[string]bool{"costa.bnet": true},
		NginxHosts:  map[string]bool{},
		NginxZones:  map[string]bool{},
		Routes:      core.NewTable(),
	}

	zone, kind, nextHop := For("costa.bnet.", cfg)
	if zone != "costa.bnet." || kind != Local || nextHop != "" {
		t.Fatalf("For(costa.bnet.) = (%q, %v, %q), want local costa.bnet.", zone, kind, nextHop)
	}
}

func TestZoneForSubdomainInsideConcreteDomainZoneIsLocal(t *testing.T) {
	cfg := &core.Config{
		TLDs:        map[string]bool{"local": true},
		DomainZones: map[string]bool{"costa.bnet": true},
		NginxHosts:  map[string]bool{},
		NginxZones:  map[string]bool{},
		Routes:      core.NewTable(),
	}

	zone, kind, nextHop := For("app.costa.bnet.", cfg)
	if zone != "costa.bnet." || kind != Local || nextHop != "" {
		t.Fatalf("For(app.costa.bnet.) = (%q, %v, %q), want local costa.bnet.", zone, kind, nextHop)
	}
}

func TestZoneForUnknownNameInsideBroadMeshZoneStaysUnknown(t *testing.T) {
	cfg := &core.Config{
		TLDs:        map[string]bool{"local": true},
		DomainZones: map[string]bool{"bnet": true},
		NginxHosts:  map[string]bool{},
		NginxZones:  map[string]bool{},
		Routes:      core.NewTable(),
	}

	zone, kind, nextHop := For("app.unknown.bnet.", cfg)
	if zone != "bnet." || kind != MeshUnknown || nextHop != "" {
		t.Fatalf("For(app.unknown.bnet.) = (%q, %v, %q), want unknown bnet.", zone, kind, nextHop)
	}
}

func TestZoneForRemoteRouteMatchesSubdomains(t *testing.T) {
	routes := core.NewTable()
	routes.Replace(map[string]core.Route{
		"ahmed.bnet": {
			Domain:  "ahmed.bnet",
			Owner:   "Ahmed",
			NextHop: "10.234.2.140",
			Source:  "10.234.2.140:8531",
			State:   core.StateOK,
		},
	})
	cfg := &core.Config{
		TLDs:        map[string]bool{"local": true},
		DomainZones: map[string]bool{"costa.bnet": true},
		NginxHosts:  map[string]bool{},
		NginxZones:  map[string]bool{},
		Routes:      routes,
	}

	zone, kind, nextHop := For("test.ahmed.bnet.", cfg)
	if zone != "ahmed.bnet." || kind != Remote || nextHop != "10.234.2.140" {
		t.Fatalf("For(test.ahmed.bnet.) = (%q, %v, %q), want remote ahmed.bnet via 10.234.2.140", zone, kind, nextHop)
	}
}

func TestZoneForMultiLabelLocalTLDMatchesAnyDepth(t *testing.T) {
	cfg := &core.Config{
		TLDs:        map[string]bool{"local": true, "local.com": true, "a.b.local.org": true},
		DomainZones: map[string]bool{},
		NginxHosts:  map[string]bool{},
		NginxZones:  map[string]bool{},
		Routes:      core.NewTable(),
	}

	cases := []struct {
		name string
		zone string
		kind Kind
	}{
		{"foo.local.", "local.", Local},
		{"local.com.", "local.com.", Local},
		{"app.local.com.", "local.com.", Local},
		{"x.y.z.local.com.", "local.com.", Local},
		{"svc.a.b.local.org.", "a.b.local.org.", Local},
		{"other.com.", "com.", None},
		{"b.local.org.", "org.", None},
	}
	for _, tc := range cases {
		zone, kind, nextHop := For(tc.name, cfg)
		if zone != tc.zone || kind != tc.kind || nextHop != "" {
			t.Errorf("For(%s) = (%q, %v, %q), want (%q, %v, \"\")", tc.name, zone, kind, nextHop, tc.zone, tc.kind)
		}
	}
}

func TestZoneForLocalTLDWinsOverBroadMeshRoot(t *testing.T) {
	cfg := &core.Config{
		TLDs:        map[string]bool{"apps.bnet": true},
		DomainZones: map[string]bool{"bnet": true},
		NginxHosts:  map[string]bool{},
		NginxZones:  map[string]bool{},
		Routes:      core.NewTable(),
	}

	zone, kind, _ := For("web.apps.bnet.", cfg)
	if zone != "apps.bnet." || kind != Local {
		t.Fatalf("For(web.apps.bnet.) = (%q, %v), want local apps.bnet.", zone, kind)
	}
	zone, kind, _ = For("web.other.bnet.", cfg)
	if zone != "bnet." || kind != MeshUnknown {
		t.Fatalf("For(web.other.bnet.) = (%q, %v), want unknown bnet.", zone, kind)
	}
}
