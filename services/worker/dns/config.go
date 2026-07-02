package main

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// parseTLDs segue as mesmas regras do antigo docker-entrypoint.sh:
// minusculo, sem "." inicial, validado [a-z0-9-] sem hifen nas pontas,
// duplicatas ignoradas, pelo menos um TLD valido exigido.
func parseTLDs(raw string) (map[string]bool, error) {
	tlds, err := parseOptionalTLDs(raw, "DNS_LOCAL_TLDS")
	if err != nil {
		return nil, err
	}
	if len(tlds) == 0 {
		return nil, fmt.Errorf("DNS_LOCAL_TLDS deve conter pelo menos um TLD local")
	}
	return tlds, nil
}

func parseOptionalTLDs(raw string, envName string) (map[string]bool, error) {
	tlds := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		tld := strings.ToLower(strings.TrimPrefix(part, "."))
		if tld == "" {
			continue
		}
		if !isValidTLD(tld) {
			return nil, fmt.Errorf("%s contem TLD invalido: %s", envName, tld)
		}
		tlds[tld] = true
	}
	return tlds, nil
}

func isValidTLD(tld string) bool {
	if strings.HasPrefix(tld, "-") || strings.HasSuffix(tld, "-") {
		return false
	}
	for _, r := range tld {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func tldNames(tlds map[string]bool) []string {
	names := make([]string, 0, len(tlds))
	for tld := range tlds {
		names = append(names, tld)
	}
	sort.Strings(names)
	return names
}

func instanceIP() (net.IP, error) {
	if raw := os.Getenv("HOST_SOURCE_CIDR"); raw != "" {
		if ip := ipFromCIDR(raw); ip != nil {
			return ip, nil
		}
		return nil, fmt.Errorf("HOST_SOURCE_CIDR invalido: %s", raw)
	}

	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("nao foi possivel detectar o IP da instancia: %w", err)
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return nil, fmt.Errorf("nao foi possivel ler o IP da interface de saida")
	}
	return addr.IP, nil
}

func ipFromCIDR(raw string) net.IP {
	if ip := net.ParseIP(raw); ip != nil {
		return ip
	}
	ip, _, err := net.ParseCIDR(raw)
	if err != nil {
		return nil
	}
	return ip
}
