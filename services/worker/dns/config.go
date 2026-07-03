package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return normalizeEnvValue(value)
	}
	return fallback
}

func normalizeEnvValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 {
		return trimmed
	}
	if (strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`)) ||
		(strings.HasPrefix(trimmed, `'`) && strings.HasSuffix(trimmed, `'`)) {
		return strings.Trim(trimmed, `"'`)
	}
	return trimmed
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
		tld := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(part), "."))
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

func parseOptionalDomains(raw string, envName string) (map[string]bool, error) {
	domains := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		domain := normalizeZone(part)
		if domain == "" {
			continue
		}
		if !isValidDomain(domain) {
			return nil, fmt.Errorf("%s contem dominio invalido: %s", envName, domain)
		}
		domains[domain] = true
	}
	return domains, nil
}

func normalizeZone(value string) string {
	domain := strings.ToLower(strings.TrimSpace(value))
	domain = strings.TrimPrefix(domain, "*.")
	domain = strings.TrimPrefix(domain, ".")
	return strings.TrimSuffix(domain, ".")
}

func isValidTLD(tld string) bool {
	if strings.Contains(tld, ".") {
		return false
	}
	return isValidDNSLabel(tld)
}

func isValidDomain(domain string) bool {
	for _, label := range strings.Split(domain, ".") {
		if !isValidDNSLabel(label) {
			return false
		}
	}
	return true
}

func isValidDNSLabel(label string) bool {
	if label == "" {
		return false
	}
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}
	for _, r := range label {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func zoneNames(zones map[string]bool) []string {
	names := make([]string, 0, len(zones))
	for zone := range zones {
		names = append(names, zone)
	}
	sort.Strings(names)
	return names
}

func normalizeRemoteRouteMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "manual" {
		return "manual"
	}
	return "auto"
}
