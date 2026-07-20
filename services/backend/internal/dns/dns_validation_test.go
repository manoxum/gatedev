package dns

import "testing"

func TestValidateLocalTLDs(t *testing.T) {
	valid := []string{
		"local",
		"local,test,example",
		" .local , TEST ",
		"a1,b-2",
		"local.com",
		"local,test,example,local.com",
		"**.a.b.local.org",
		"*.corp",
	}
	for _, raw := range valid {
		if err := validateLocalTLDs(raw); err != nil {
			t.Errorf("validateLocalTLDs(%q) devia aceitar, rejeitou: %v", raw, err)
		}
	}

	invalid := []string{"", " , ", "-local", "local-", "lo_cal", "açaí", "a..b", "local,-bad.com"}
	for _, raw := range invalid {
		if err := validateLocalTLDs(raw); err == nil {
			t.Errorf("validateLocalTLDs(%q) devia rejeitar, aceitou", raw)
		}
	}
}

func TestValidateDomains(t *testing.T) {
	valid := []string{"", "bnet", "costa.bnet", "*.costa.bnet", "costa.bnet,dev", "Costa.BNET."}
	for _, raw := range valid {
		if err := validateDomains(raw); err != nil {
			t.Errorf("validateDomains(%q) devia aceitar, rejeitou: %v", raw, err)
		}
	}

	invalid := []string{"-costa.bnet", "costa-.bnet", "costa..bnet", "co_sta.bnet"}
	for _, raw := range invalid {
		if err := validateDomains(raw); err == nil {
			t.Errorf("validateDomains(%q) devia rejeitar, aceitou", raw)
		}
	}
}
