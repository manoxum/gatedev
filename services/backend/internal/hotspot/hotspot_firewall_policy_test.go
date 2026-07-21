package hotspot

import (
	"testing"

	"bindnet/backend/internal/hotspot/store"
)

func wanRule(sourceKind, sourceRef, protocol string, dstPorts, dstHost *string, action string) store.CommRule {
	return store.CommRule{
		Zone:       store.CommZoneWAN,
		SourceKind: sourceKind, SourceRef: sourceRef,
		TargetKind: store.CommEndpointAny, Direction: store.CommDirectionTo,
		Protocol: protocol, DstPorts: dstPorts, DstHost: dstHost, Action: action, Enabled: true,
	}
}

func TestZoneRulesAnySourceNoMACFilter(t *testing.T) {
	rules := []store.CommRule{
		wanRule(store.CommEndpointAny, "", store.CommProtocolTCP, ref("25"), nil, store.CommActionDeny),
	}
	got := compileZoneRules(store.CommZoneWAN, isolationTestClients, isolationTestProfiles, rules)
	if len(got) != 1 || got[0].MAC != "" || got[0].Protocol != "tcp" || got[0].DstPorts != "25" || got[0].Action != "deny" {
		t.Fatalf("entrada wan any = %+v, esperado 1 entrada sem MAC, tcp/25 deny", got)
	}
}

func TestZoneRulesProfileExpandsToConnectedMACs(t *testing.T) {
	rules := []store.CommRule{
		wanRule(store.CommEndpointProfile, "profile-a", store.CommProtocolAny, nil, nil, store.CommActionDeny),
	}
	got := compileZoneRules(store.CommZoneWAN, isolationTestClients, isolationTestProfiles, rules)
	// profile-a tem dois MACs conectados (A1, A2).
	macs := map[string]bool{}
	for _, entry := range got {
		macs[entry.MAC] = true
	}
	if len(got) != 2 || !macs["aa:aa:aa:aa:aa:01"] || !macs["aa:aa:aa:aa:aa:02"] {
		t.Fatalf("profile-a deveria expandir para A1+A2, veio %+v", got)
	}
}

func TestZoneRulesDeviceEmittedEvenWhenOffline(t *testing.T) {
	rules := []store.CommRule{
		wanRule(store.CommEndpointDevice, "ff:ff:ff:ff:ff:ff", store.CommProtocolAny, nil, nil, store.CommActionDeny),
	}
	got := compileZoneRules(store.CommZoneWAN, isolationTestClients, isolationTestProfiles, rules)
	if len(got) != 1 || got[0].MAC != "ff:ff:ff:ff:ff:ff" {
		t.Fatalf("regra de device offline deveria ser emitida, veio %+v", got)
	}
}

func TestZoneRulesOrderedDeviceBeforeAnyAndDenyFirst(t *testing.T) {
	rules := []store.CommRule{
		wanRule(store.CommEndpointAny, "", store.CommProtocolAny, nil, nil, store.CommActionDeny),
		wanRule(store.CommEndpointDevice, "aa:aa:aa:aa:aa:01", store.CommProtocolAny, nil, nil, store.CommActionAllow),
	}
	got := compileZoneRules(store.CommZoneWAN, isolationTestClients, isolationTestProfiles, rules)
	// device (spec 2) precede any (spec 0), independentemente da ordem de entrada.
	if got[0].MAC != "aa:aa:aa:aa:aa:01" || got[len(got)-1].MAC != "" {
		t.Fatalf("ordenacao errada: %+v", got)
	}
}

func TestZoneRulesL4SpecificDenyBeforeBroadAllowSameSource(t *testing.T) {
	rules := []store.CommRule{
		wanRule(store.CommEndpointAny, "", store.CommProtocolAny, nil, nil, store.CommActionAllow),
		wanRule(store.CommEndpointAny, "", store.CommProtocolUDP, nil, nil, store.CommActionDeny),
	}
	got := compileZoneRules(store.CommZoneWAN, isolationTestClients, isolationTestProfiles, rules)
	if got[0].Protocol != "udp" || got[0].Action != "deny" {
		t.Fatalf("deny udp (L4 especifico) deveria vir primeiro: %+v", got)
	}
}

func TestZoneRulesLocalIgnoresWanRules(t *testing.T) {
	rules := []store.CommRule{
		wanRule(store.CommEndpointAny, "", store.CommProtocolAny, nil, nil, store.CommActionDeny),
	}
	got := compileZoneRules(store.CommZoneLocal, isolationTestClients, isolationTestProfiles, rules)
	if len(got) != 0 {
		t.Fatalf("zona local nao deveria ver regras da zona wan: %+v", got)
	}
}

func TestZoneRulesDisabledIgnored(t *testing.T) {
	rule := wanRule(store.CommEndpointAny, "", store.CommProtocolAny, nil, nil, store.CommActionDeny)
	rule.Enabled = false
	got := compileZoneRules(store.CommZoneWAN, isolationTestClients, isolationTestProfiles, []store.CommRule{rule})
	if len(got) != 0 {
		t.Fatalf("regra desabilitada nao deveria ser emitida: %+v", got)
	}
}
