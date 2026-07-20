package shaping

import "testing"

func TestParseHotspotNATInterface(t *testing.T) {
	output := `
-N BINDNET-HOTSPOT
-A BINDNET-HOTSPOT -s 192.168.12.0/24 -o enp1s0 -j MASQUERADE
`

	if got := parseHotspotNATInterface(output); got != "enp1s0" {
		t.Fatalf("parseHotspotNATInterface = %q, want enp1s0", got)
	}
}

func TestParseRouteInterface(t *testing.T) {
	output := `1.1.1.1 via 10.234.2.252 dev enp1s0 src 10.234.2.102 uid 0`

	if got := parseRouteInterface(output); got != "enp1s0" {
		t.Fatalf("parseRouteInterface = %q, want enp1s0", got)
	}
}
