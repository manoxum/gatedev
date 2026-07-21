package network

import (
	"reflect"
	"testing"
)

func TestHostDNSRouteDomainsIncludesMultiLabelSuffixes(t *testing.T) {
	got := hostDNSRouteDomains(map[string]string{
		"DNS_LOCAL_TLDS": "local,test,ah,local.com, **.a.b.local.org",
		"DOMAINS":        "ahmed.bnet; LOCAL.COM",
	})
	want := []string{"~a.b.local.org", "~ah", "~ahmed.bnet", "~local", "~local.com", "~test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hostDNSRouteDomains() = %v, want %v", got, want)
	}
}

func TestHostDNSRouteDomainsDropsInvalidAndDuplicateValues(t *testing.T) {
	got := hostDNSRouteDomains(map[string]string{
		"DNS_LOCAL_TLDS": "local,local,inválido,-bad,bad-,a..b",
	})
	want := []string{"~local"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hostDNSRouteDomains() = %v, want %v", got, want)
	}
}
