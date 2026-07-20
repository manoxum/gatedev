package network

import (
	"net"
	"strings"
)

// NormalizeMAC valida e normaliza um endereco MAC para minusculo no
// formato canonico "aa:bb:cc:dd:ee:ff".
func NormalizeMAC(raw string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return strings.ToLower(hw.String()), nil
}
