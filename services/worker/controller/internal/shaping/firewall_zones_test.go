package shaping

import (
	"strings"
	"testing"
)

func TestBuildWanChainRulesStructure(t *testing.T) {
	rules := []firewallZonePayload{
		{MAC: "aa:bb:cc:dd:ee:01", Protocol: "tcp", DstPorts: "25", Action: "deny"},
	}
	got := buildWanChainRules("allow", rules)
	if got[0].comment != "bn-fw-wan-est" {
		t.Fatalf("primeira regra deveria ser established, veio %q", got[0].comment)
	}
	if got[len(got)-1].comment != "bn-fw-wan-default" {
		t.Fatalf("ultima regra deveria ser default, veio %q", got[len(got)-1].comment)
	}
	// Politica allow -> RETURN no default (deixa seguir para o hotspot).
	if !strings.Contains(strings.Join(got[len(got)-1].args, " "), "-j RETURN") {
		t.Fatalf("default allow deveria ser RETURN: %v", got[len(got)-1].args)
	}
	// Regra de deny wan usa DROP.
	mid := strings.Join(got[1].args, " ")
	if !strings.Contains(mid, "-j DROP") || !strings.Contains(mid, "--mac-source aa:bb:cc:dd:ee:01") || !strings.Contains(mid, "-p tcp") {
		t.Fatalf("regra wan deny mal formada: %q", mid)
	}
}

func TestBuildWanChainRulesDenyPolicyDrops(t *testing.T) {
	got := buildWanChainRules("deny", nil)
	last := strings.Join(got[len(got)-1].args, " ")
	if !strings.Contains(last, "-j DROP") {
		t.Fatalf("default deny deveria ser DROP: %q", last)
	}
}

func TestBuildLocalChainRulesHasEssentialAllows(t *testing.T) {
	got := buildLocalChainRules("deny", nil)
	comments := map[string]bool{}
	for _, rule := range got {
		comments[rule.comment] = true
	}
	// Mesmo com politica deny, DNS/DHCP/painel continuam liberados.
	for _, essential := range []string{"bn-fw-local-est", "bn-fw-local-dhcp", "bn-fw-local-dns-udp", "bn-fw-local-http", "bn-fw-local-https"} {
		if !comments[essential] {
			t.Fatalf("faltou permissao essencial %q em politica deny", essential)
		}
	}
	if got[len(got)-1].comment != "bn-fw-local-default" ||
		!strings.Contains(strings.Join(got[len(got)-1].args, " "), "-j DROP") {
		t.Fatalf("default deny local deveria terminar em DROP")
	}
}

func TestZoneChainRuleAnySourceHasNoMacMatch(t *testing.T) {
	rule := firewallZonePayload{MAC: "", Protocol: "any", Action: "allow"}
	args := strings.Join(zoneChainRule(fwLocalChain, 0, rule, "ACCEPT", false).args, " ")
	if strings.Contains(args, "--mac-source") {
		t.Fatalf("origem 'todos' nao deveria ter --mac-source: %q", args)
	}
}

func TestZoneChainRuleWanHostOnlyWithHost(t *testing.T) {
	rule := firewallZonePayload{MAC: "", Protocol: "tcp", DstPorts: "443", DstHost: "1.2.3.4", Action: "deny"}
	args := strings.Join(zoneChainRule(fwWanChain, 0, rule, "DROP", true).args, " ")
	if !strings.Contains(args, "-d 1.2.3.4") || !strings.Contains(args, "--dports 443") {
		t.Fatalf("regra wan com host/porta mal formada: %q", args)
	}
	// Sem withHost (zona local), o -d nao entra.
	localArgs := strings.Join(zoneChainRule(fwLocalChain, 0, rule, "DROP", false).args, " ")
	if strings.Contains(localArgs, "-d 1.2.3.4") {
		t.Fatalf("zona local nao deveria usar dstHost: %q", localArgs)
	}
}

func TestIsValidHostOrCIDR(t *testing.T) {
	for _, ok := range []string{"1.2.3.4", "10.0.0.0/24", "192.168.1.255"} {
		if !isValidHostOrCIDR(ok) {
			t.Fatalf("%q deveria ser valido", ok)
		}
	}
	for _, bad := range []string{"", "999.1.1.1", "abc", "10.0.0.0/40"} {
		if isValidHostOrCIDR(bad) {
			t.Fatalf("%q deveria ser invalido", bad)
		}
	}
}
