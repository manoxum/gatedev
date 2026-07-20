// certificate_validity.go calcula o periodo de validade (NotBefore/
// NotAfter) de um certificado leaf a partir da quantidade+unidade
// informadas pelo usuario ao emitir (certificates.go).
package cert

import "time"

const (
	defaultValidityQuantity = 2
	defaultValidityUnit     = "years"
)

var validValidityUnits = map[string]bool{
	"days": true, "weeks": true, "months": true, "years": true,
}

// normalizeValidityPeriod valida quantidade/unidade informadas pelo
// usuario; cai para o padrao (2 anos, mesmo comportamento fixo de
// antes desta opcao existir) se invalidas.
func normalizeValidityPeriod(quantity int, unit string) (int, string) {
	if quantity <= 0 || !validValidityUnits[unit] {
		return defaultValidityQuantity, defaultValidityUnit
	}
	return quantity, unit
}

// certificateExpiry calcula NotAfter a partir de now, quantity e unit -
// unit ja deve ter passado por normalizeValidityPeriod.
func certificateExpiry(now time.Time, quantity int, unit string) time.Time {
	switch unit {
	case "days":
		return now.AddDate(0, 0, quantity)
	case "weeks":
		return now.AddDate(0, 0, quantity*7)
	case "months":
		return now.AddDate(0, quantity, 0)
	default: // "years"
		return now.AddDate(quantity, 0, 0)
	}
}
