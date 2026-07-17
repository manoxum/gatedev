package main

import "testing"

func TestNormalizeEnvValueUnquotesNodeName(t *testing.T) {
	if got := normalizeEnvValue(`"Daniel Costa"`); got != "Daniel Costa" {
		t.Fatalf("normalizeEnvValue returned %q, want Daniel Costa", got)
	}
}

func TestParseTLDsAcceptsMultiLabelSuffixes(t *testing.T) {
	tlds, err := parseTLDs("local,test,example,local.com,**.a.b.local.org, .dev ,*.corp")
	if err != nil {
		t.Fatalf("parseTLDs rejeitou lista valida: %v", err)
	}
	for _, want := range []string{"local", "test", "example", "local.com", "a.b.local.org", "dev", "corp"} {
		if !tlds[want] {
			t.Errorf("parseTLDs nao registrou %q (mapa: %v)", want, tlds)
		}
	}
}

func TestParseTLDsRejectsInvalidEntries(t *testing.T) {
	for _, raw := range []string{"", " , ", "local,-bad", "local,bad-", "local,ba_d", "local,a..b"} {
		if _, err := parseTLDs(raw); err == nil {
			t.Errorf("parseTLDs(%q) devia falhar, aceitou", raw)
		}
	}
}
