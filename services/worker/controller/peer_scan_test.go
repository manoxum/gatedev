package main

import (
	"net"
	"testing"
)

func TestBoundedIPv4HostsScansWholeSmallSubnet(t *testing.T) {
	_, network, err := net.ParseCIDR("10.234.2.0/23")
	if err != nil {
		t.Fatal(err)
	}
	hosts := boundedIPv4Hosts(net.IPv4(10, 234, 2, 102), network)
	if len(hosts) != 510 {
		t.Fatalf("boundedIPv4Hosts returned %d hosts, want 510", len(hosts))
	}
	if hosts[0] != "10.234.2.1" || hosts[len(hosts)-1] != "10.234.3.254" {
		t.Fatalf("boundedIPv4Hosts returned range %s..%s, want 10.234.2.1..10.234.3.254", hosts[0], hosts[len(hosts)-1])
	}
}

func TestBoundedIPv4HostsLargeSubnetIncludesLocal24AndNearbyHosts(t *testing.T) {
	_, network, err := net.ParseCIDR("10.234.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	hosts := boundedIPv4Hosts(net.IPv4(10, 234, 2, 102), network)
	if len(hosts) != peerScanMaxCandidates {
		t.Fatalf("boundedIPv4Hosts returned %d hosts, want cap %d", len(hosts), peerScanMaxCandidates)
	}
	if hosts[0] != "10.234.2.1" {
		t.Fatalf("boundedIPv4Hosts did not start with local /24, got %s", hosts[0])
	}
	if !containsString(hosts, "10.234.3.1") {
		t.Fatalf("boundedIPv4Hosts did not include nearby host outside local /24")
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
