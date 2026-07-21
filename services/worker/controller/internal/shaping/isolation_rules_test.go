package shaping

import (
	"strings"
	"testing"
)

func TestIsolationEntryCommentDeterministic(t *testing.T) {
	pair := isolationPairPayload{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.12.21", Protocol: "tcp", DstPorts: "80,443", Action: "allow"}
	got := isolationEntryComment(2, pair)
	want := "bniso-2-allow-tcp-80,443-aabbccddeeff-192.168.12.21"
	if got != want {
		t.Fatalf("comment = %q, esperado %q", got, want)
	}
	// Portas vazias viram "any" para a assinatura nunca ficar ambigua.
	noPorts := isolationPairPayload{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.12.21", Protocol: "any", Action: "deny"}
	if c := isolationEntryComment(0, noPorts); !strings.Contains(c, "-any-") {
		t.Fatalf("comment sem portas = %q, esperava conter '-any-'", c)
	}
}

func TestIsolationEntryArgsL4(t *testing.T) {
	pair := isolationPairPayload{MAC: "aa:bb:cc:dd:ee:ff", IP: "10.0.0.5", Protocol: "tcp", DstPorts: "22", Action: "allow"}
	args := strings.Join(isolationEntryArgs(1, pair), " ")
	for _, want := range []string{"--mac-source aa:bb:cc:dd:ee:ff", "-d 10.0.0.5", "-p tcp", "--dports 22", "-j ACCEPT"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q nao contem %q", args, want)
		}
	}

	deny := isolationPairPayload{MAC: "aa:bb:cc:dd:ee:ff", IP: "10.0.0.5", Protocol: "any", Action: "deny"}
	dargs := strings.Join(isolationEntryArgs(0, deny), " ")
	if !strings.Contains(dargs, "-j DROP") || strings.Contains(dargs, "-p ") {
		t.Fatalf("deny any deveria ser DROP sem -p: %q", dargs)
	}
}

func TestDesiredIsolationCommentsOrder(t *testing.T) {
	pairs := []isolationPairPayload{
		{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.12.11", Protocol: "any", Action: "allow"},
		{MAC: "aa:bb:cc:dd:ee:02", IP: "192.168.12.12", Protocol: "tcp", DstPorts: "443", Action: "allow"},
	}
	got := desiredIsolationComments(pairs)
	if got[0] != isolationEstablishedComment || got[len(got)-1] != isolationDropComment {
		t.Fatalf("assinatura deve comecar com established e terminar com drop: %v", got)
	}
	if len(got) != len(pairs)+2 {
		t.Fatalf("assinatura = %d entradas, esperado %d", len(got), len(pairs)+2)
	}
}

func TestParseIptablesComment(t *testing.T) {
	if c := parseIptablesComment(`"bniso:drop" -j DROP`); c != "bniso:drop" {
		t.Fatalf("comment = %q, esperado bniso:drop", c)
	}
}

func TestIsValidPortList(t *testing.T) {
	valid := []string{"80", "80,443", "8000-8100", "53,80,443,1000-2000"}
	for _, v := range valid {
		if !isValidPortList(v) {
			t.Fatalf("%q deveria ser valido", v)
		}
	}
	invalid := []string{"", "0", "70000", "80-", "abc", "80,,443"}
	for _, v := range invalid {
		if isValidPortList(v) {
			t.Fatalf("%q deveria ser invalido", v)
		}
	}
}
