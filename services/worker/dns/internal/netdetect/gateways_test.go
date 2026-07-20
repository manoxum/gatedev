package netdetect

import "testing"

func TestDiscoverHostSourceIPsParsesCIDRAndPlainIP(t *testing.T) {
	ips, err := DiscoverHostSourceIPs("10.234.2.102/32, 10.234.2.103", "10.234.2.103")
	if err != nil {
		t.Fatalf("DiscoverHostSourceIPs returned error: %v", err)
	}
	if len(ips) != 1 || ips[0] != "10.234.2.102" {
		t.Fatalf("DiscoverHostSourceIPs returned %v, want [10.234.2.102]", ips)
	}
}

func TestDiscoverHostSourceIPsRejectsLoopback(t *testing.T) {
	if _, err := DiscoverHostSourceIPs("127.0.0.1/32"); err == nil {
		t.Fatalf("DiscoverHostSourceIPs accepted loopback HOST_SOURCE_CIDR")
	}
}

func TestIsIgnoredLANInterface(t *testing.T) {
	for _, name := range []string{"lo", "ap0", "docker0", "br-abcd", "veth123", "wg0"} {
		if !isIgnoredLANInterface(name) {
			t.Fatalf("isIgnoredLANInterface(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"eth0", "wlp2s0", "enp3s0"} {
		if isIgnoredLANInterface(name) {
			t.Fatalf("isIgnoredLANInterface(%q) = true, want false", name)
		}
	}
}
