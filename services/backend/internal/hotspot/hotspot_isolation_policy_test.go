package hotspot

import (
	"fmt"
	"sort"
	"testing"

	"bindnet/backend/internal/hotspot/store"
)

// Cenario base dos testes: A1/A2 no perfil "a", B1 no perfil "b", cada
// um com IP proprio. Os casos exercitam as modalidades de isolamento e
// o casamento L4 (protocolo/portas) do firewall.
var isolationTestClients = []isolationClient{
	{MAC: "aa:aa:aa:aa:aa:01", IP: "192.168.12.11"},
	{MAC: "aa:aa:aa:aa:aa:02", IP: "192.168.12.12"},
	{MAC: "bb:bb:bb:bb:bb:01", IP: "192.168.12.21"},
}

var isolationTestProfiles = map[string]string{
	"aa:aa:aa:aa:aa:01": "profile-a",
	"aa:aa:aa:aa:aa:02": "profile-a",
	"bb:bb:bb:bb:bb:01": "profile-b",
}

func ref(value string) *string { return &value }

func commRule(sourceKind, sourceRef, targetKind string, targetRef *string, direction, action string) store.CommRule {
	return store.CommRule{
		Zone:       store.CommZoneClients,
		SourceKind: sourceKind, SourceRef: sourceRef,
		TargetKind: targetKind, TargetRef: targetRef,
		Direction: direction, Protocol: store.CommProtocolAny, Action: action, Enabled: true,
	}
}

// effectiveAllowed reduz as entradas compiladas ao que de fato passa: o
// worker instala em ordem e o iptables casa a PRIMEIRA regra de cada
// par (mac -> ip), entao a primeira entrada de cada par decide.
func effectiveAllowed(pairs []firewallPairRule) []string {
	first := map[string]string{}
	order := []string{}
	for _, pair := range pairs {
		key := pair.MAC + ">" + pair.IP
		if _, seen := first[key]; !seen {
			first[key] = pair.Action
			order = append(order, key)
		}
	}
	allowed := []string{}
	for _, key := range order {
		if first[key] == store.CommActionAllow {
			allowed = append(allowed, key)
		}
	}
	sort.Strings(allowed)
	return allowed
}

func assertAllowed(t *testing.T, got []firewallPairRule, want ...string) {
	t.Helper()
	sort.Strings(want)
	if want == nil {
		want = []string{}
	}
	allowed := effectiveAllowed(got)
	if fmt.Sprint(allowed) != fmt.Sprint(want) {
		t.Fatalf("pares permitidos = %v, esperado %v", allowed, want)
	}
}

func TestIsolationDefaultDenyEverything(t *testing.T) {
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, nil)
	assertAllowed(t, got)
}

func TestIsolationInternalProfileOnly(t *testing.T) {
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles,
		map[string]bool{"profile-a": true}, nil)
	assertAllowed(t, got,
		"aa:aa:aa:aa:aa:01>192.168.12.12",
		"aa:aa:aa:aa:aa:02>192.168.12.11")
}

func TestIsolationSameProfileRuleActsAsInternal(t *testing.T) {
	rules := []store.CommRule{
		commRule(store.CommEndpointProfile, "profile-a", store.CommEndpointProfile, ref("profile-a"), store.CommDirectionBoth, store.CommActionAllow),
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, rules)
	assertAllowed(t, got,
		"aa:aa:aa:aa:aa:01>192.168.12.12",
		"aa:aa:aa:aa:aa:02>192.168.12.11")
}

func TestIsolationFullyIsolatedDeviceBeatsInternalAllow(t *testing.T) {
	rules := []store.CommRule{
		commRule(store.CommEndpointDevice, "aa:aa:aa:aa:aa:01", store.CommEndpointAny, nil, store.CommDirectionBoth, store.CommActionDeny),
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles,
		map[string]bool{"profile-a": true}, rules)
	// A1 nunca aparece como origem permitida; os demais pares de A ficam
	// so entre A2 e ninguem (A2<->A1 bloqueado pela regra de A1).
	assertAllowed(t, got)
}

func TestIsolationDeviceToDeviceAllowBeatsAnyDeny(t *testing.T) {
	rules := []store.CommRule{
		commRule(store.CommEndpointDevice, "aa:aa:aa:aa:aa:01", store.CommEndpointAny, nil, store.CommDirectionBoth, store.CommActionDeny),
		commRule(store.CommEndpointDevice, "aa:aa:aa:aa:aa:01", store.CommEndpointDevice, ref("bb:bb:bb:bb:bb:01"), store.CommDirectionBoth, store.CommActionAllow),
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, rules)
	assertAllowed(t, got,
		"aa:aa:aa:aa:aa:01>192.168.12.21",
		"bb:bb:bb:bb:bb:01>192.168.12.11")
}

func TestIsolationProfileToProfileOneWay(t *testing.T) {
	rules := []store.CommRule{
		commRule(store.CommEndpointProfile, "profile-a", store.CommEndpointProfile, ref("profile-b"), store.CommDirectionTo, store.CommActionAllow),
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, rules)
	assertAllowed(t, got,
		"aa:aa:aa:aa:aa:01>192.168.12.21",
		"aa:aa:aa:aa:aa:02>192.168.12.21")
}

func TestIsolationCombinedModalities(t *testing.T) {
	rules := []store.CommRule{
		commRule(store.CommEndpointProfile, "profile-a", store.CommEndpointProfile, ref("profile-b"), store.CommDirectionBoth, store.CommActionAllow),
		commRule(store.CommEndpointDevice, "aa:aa:aa:aa:aa:02", store.CommEndpointProfile, ref("profile-b"), store.CommDirectionBoth, store.CommActionDeny),
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles,
		map[string]bool{"profile-a": true}, rules)
	assertAllowed(t, got,
		"aa:aa:aa:aa:aa:01>192.168.12.12",
		"aa:aa:aa:aa:aa:02>192.168.12.11",
		"aa:aa:aa:aa:aa:01>192.168.12.21",
		"bb:bb:bb:bb:bb:01>192.168.12.11")
}

func TestIsolationTieSameSpecificityDenyWins(t *testing.T) {
	rules := []store.CommRule{
		commRule(store.CommEndpointProfile, "profile-a", store.CommEndpointProfile, ref("profile-b"), store.CommDirectionBoth, store.CommActionAllow),
		commRule(store.CommEndpointProfile, "profile-a", store.CommEndpointProfile, ref("profile-b"), store.CommDirectionBoth, store.CommActionDeny),
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, rules)
	assertAllowed(t, got)
}

func TestIsolationDisabledRuleIgnored(t *testing.T) {
	rule := commRule(store.CommEndpointProfile, "profile-a", store.CommEndpointProfile, ref("profile-b"), store.CommDirectionBoth, store.CommActionAllow)
	rule.Enabled = false
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, []store.CommRule{rule})
	assertAllowed(t, got)
}

// Regras de outras zonas (wan/local) nao entram na compilacao da zona
// clients - so o worker de cada zona as consome.
func TestIsolationIgnoresNonClientZoneRules(t *testing.T) {
	rule := commRule(store.CommEndpointProfile, "profile-a", store.CommEndpointProfile, ref("profile-a"), store.CommDirectionBoth, store.CommActionAllow)
	rule.Zone = store.CommZoneWAN
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, []store.CommRule{rule})
	assertAllowed(t, got)
}

func TestIsolationUnknownMACFallsBackToDefaultProfile(t *testing.T) {
	clients := []isolationClient{
		{MAC: "cc:cc:cc:cc:cc:01", IP: "192.168.12.31"},
		{MAC: "cc:cc:cc:cc:cc:02", IP: "192.168.12.32"},
	}
	got := compileClientsZonePairs(clients, map[string]string{},
		map[string]bool{store.DefaultProfileID: true}, nil)
	assertAllowed(t, got,
		"cc:cc:cc:cc:cc:01>192.168.12.32",
		"cc:cc:cc:cc:cc:02>192.168.12.31")
}

func TestIsolationTargetWithoutIPSkipped(t *testing.T) {
	clients := []isolationClient{
		{MAC: "aa:aa:aa:aa:aa:01", IP: "192.168.12.11"},
		{MAC: "aa:aa:aa:aa:aa:02", IP: ""},
	}
	got := compileClientsZonePairs(clients, isolationTestProfiles,
		map[string]bool{"profile-a": true}, nil)
	assertAllowed(t, got, "aa:aa:aa:aa:aa:02>192.168.12.11")
}

// L4: uma regra tcp/443 entre A1 e A2 produz uma entrada ACCEPT restrita
// a esse protocolo/porta (o resto cai no DROP default do chain).
func TestIsolationL4AllowScopedToPort(t *testing.T) {
	rules := []store.CommRule{
		{
			Zone:       store.CommZoneClients,
			SourceKind: store.CommEndpointDevice, SourceRef: "aa:aa:aa:aa:aa:01",
			TargetKind: store.CommEndpointDevice, TargetRef: ref("aa:aa:aa:aa:aa:02"),
			Direction: store.CommDirectionTo, Protocol: store.CommProtocolTCP, DstPorts: ref("443"),
			Action: store.CommActionAllow, Enabled: true,
		},
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, rules)
	var found *firewallPairRule
	for i := range got {
		if got[i].MAC == "aa:aa:aa:aa:aa:01" && got[i].IP == "192.168.12.12" {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatal("esperava entrada A1 -> A2")
	}
	if found.Protocol != store.CommProtocolTCP || found.DstPorts != "443" || found.Action != store.CommActionAllow {
		t.Fatalf("entrada L4 = %+v, esperado tcp/443 allow", *found)
	}
}

// L4 precedencia: "deny udp" (L4 mais especifico) fica ANTES de
// "allow any" para o mesmo par - o worker instala nessa ordem e o
// iptables casa a primeira, bloqueando so udp e liberando o resto.
func TestIsolationL4SpecificDenyOrderedBeforeBroadAllow(t *testing.T) {
	rules := []store.CommRule{
		commRule(store.CommEndpointDevice, "aa:aa:aa:aa:aa:01", store.CommEndpointDevice, ref("aa:aa:aa:aa:aa:02"), store.CommDirectionTo, store.CommActionAllow),
		{
			Zone:       store.CommZoneClients,
			SourceKind: store.CommEndpointDevice, SourceRef: "aa:aa:aa:aa:aa:01",
			TargetKind: store.CommEndpointDevice, TargetRef: ref("aa:aa:aa:aa:aa:02"),
			Direction: store.CommDirectionTo, Protocol: store.CommProtocolUDP,
			Action: store.CommActionDeny, Enabled: true,
		},
	}
	got := compileClientsZonePairs(isolationTestClients, isolationTestProfiles, map[string]bool{}, rules)
	var pairEntries []firewallPairRule
	for _, entry := range got {
		if entry.MAC == "aa:aa:aa:aa:aa:01" && entry.IP == "192.168.12.12" {
			pairEntries = append(pairEntries, entry)
		}
	}
	if len(pairEntries) != 2 {
		t.Fatalf("esperava 2 entradas para A1->A2, veio %d: %+v", len(pairEntries), pairEntries)
	}
	if pairEntries[0].Action != store.CommActionDeny || pairEntries[0].Protocol != store.CommProtocolUDP {
		t.Fatalf("primeira entrada deveria ser deny udp, veio %+v", pairEntries[0])
	}
	if pairEntries[1].Action != store.CommActionAllow || pairEntries[1].Protocol != store.CommProtocolAny {
		t.Fatalf("segunda entrada deveria ser allow any, veio %+v", pairEntries[1])
	}
}
