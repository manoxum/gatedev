// certificate_domains.go normaliza os dominios/IPs informados para
// emissao de certificado (certificates.go). Suporta multiplos SANs
// (Subject Alternative Names) em um unico certificado, incluindo um
// dominio curinga (ex.: "*.mydomain") entre eles.
package cert

import (
	"net"
	"strings"
)

const defaultDomain = "localhost.local"

// normalizeDomain e uma copia byte a byte da funcao homonima do antigo
// cert-proxy, com um adicional: o primeiro rotulo pode ser "*" (dominio
// curinga, ex.: "*.mydomain") sem cair no fallback. Minusculo, sem
// porta, sem "." final; cai para defaultDomain se algum rotulo (fora o
// "*" curinga) nao passar na validacao [a-z0-9-].
func normalizeDomain(value string) string {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if domain == "" {
		return defaultDomain
	}
	if h, _, err := net.SplitHostPort(domain); err == nil {
		domain = h
	}
	if ip := net.ParseIP(domain); ip != nil {
		return domain
	}
	labels := strings.Split(domain, ".")
	if labels[0] == "*" {
		if len(labels) < 2 {
			return defaultDomain
		}
		labels = labels[1:]
	}
	for _, label := range labels {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return defaultDomain
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return defaultDomain
			}
		}
	}
	return domain
}

// hasNonEmptyDomain reporta se ao menos uma entrada nao esta em
// branco - usado para validar o corpo de POST /api/certificates antes
// de chamar issueCertificate (que ja teria como fallback defaultDomain).
func hasNonEmptyDomain(rawDomains []string) bool {
	for _, raw := range rawDomains {
		if strings.TrimSpace(raw) != "" {
			return true
		}
	}
	return false
}

// normalizeDomainList normaliza cada entrada com normalizeDomain e
// remove duplicatas, preservando a ordem informada (a primeira entrada
// valida vira o dominio/CN primario do certificado em issueCertificate).
// Cai para []string{defaultDomain} se nenhuma entrada nao-vazia for
// informada - mesmo comportamento de fallback de um unico dominio.
func normalizeDomainList(rawDomains []string) []string {
	seen := make(map[string]bool, len(rawDomains))
	result := make([]string, 0, len(rawDomains))
	for _, raw := range rawDomains {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		domain := normalizeDomain(raw)
		if seen[domain] {
			continue
		}
		seen[domain] = true
		result = append(result, domain)
	}
	if len(result) == 0 {
		result = []string{defaultDomain}
	}
	return result
}

// splitNonEmpty separa valores de array_to_string(coluna, ',') - retorna
// nil (em vez de []string{""}) quando a coluna original era um array
// Postgres vazio ou nulo.
func splitNonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}
