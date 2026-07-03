package main

import "testing"

func TestNormalizeEnvValueUnquotesNodeName(t *testing.T) {
	if got := normalizeEnvValue(`"Daniel Costa"`); got != "Daniel Costa" {
		t.Fatalf("normalizeEnvValue returned %q, want Daniel Costa", got)
	}
}
