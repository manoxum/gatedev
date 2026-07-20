// Package config trata a leitura de variaveis de ambiente e a validacao
// de TLDs/dominios locais do dns-provider - funcoes puras, sem I/O de rede
// ou banco, para poder ser importado por qualquer outro pacote sem ciclo.
package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func Getenv(name, fallback string) string {
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

// ParseTLDs aceita tanto TLDs simples ("local") quanto sufixos com
// varios labels tratados como TLD local ("local.com", "a.b.local"):
// minusculo, prefixos "*."/"**."/"." descartados, cada label validado
// [a-z0-9-] sem hifen nas pontas, duplicatas ignoradas, pelo menos uma
// entrada valida exigida. Qualquer nome que termine num desses sufixos
// resolve como zona local (ver pacote zones).
func ParseTLDs(raw string) (map[string]bool, error) {
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
		tld := NormalizeZone(part)
		if tld == "" {
			continue
		}
		if !IsValidDomain(tld) {
			return nil, fmt.Errorf("%s contem TLD invalido: %s", envName, tld)
		}
		tlds[tld] = true
	}
	return tlds, nil
}

func ParseOptionalDomains(raw string, envName string) (map[string]bool, error) {
	domains := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		domain := NormalizeZone(part)
		if domain == "" {
			continue
		}
		if !IsValidDomain(domain) {
			return nil, fmt.Errorf("%s contem dominio invalido: %s", envName, domain)
		}
		domains[domain] = true
	}
	return domains, nil
}

// NormalizeZone descarta prefixos wildcard ("*.", "**.", ".") e o ponto
// final: "**.a.b.local" e "a.b.local" declaram o mesmo sufixo local.
func NormalizeZone(value string) string {
	domain := strings.ToLower(strings.TrimSpace(value))
	domain = strings.TrimLeft(domain, "*.")
	return strings.TrimSuffix(domain, ".")
}

func IsValidDomain(domain string) bool {
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

func ZoneNames(zones map[string]bool) []string {
	names := make([]string, 0, len(zones))
	for zone := range zones {
		names = append(names, zone)
	}
	sort.Strings(names)
	return names
}

func NormalizeRemoteRouteMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "manual" {
		return "manual"
	}
	return "auto"
}
