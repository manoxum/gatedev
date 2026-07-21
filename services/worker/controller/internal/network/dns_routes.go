package network

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

const (
	hostDNSConnection = "bindnet-dns"
	hostDNSInterface  = "bn-dns"
)

// SyncHostDNSRoutes encaminha os namespaces internos ao listener host do
// dns-provider. Os '~' tornam os dominios route-only no systemd-resolved:
// nao alteram a pesquisa de nomes curtos e ganham do DNS default pela maior
// correspondencia de sufixo (inclusive para zonas como local.com).
func SyncHostDNSRoutes(config map[string]string) error {
	routes := hostDNSRouteDomains(config)
	if len(routes) == 0 {
		return fmt.Errorf("DNS_LOCAL_TLDS/DOMAINS nao contem nenhuma rota DNS valida")
	}
	routeList := strings.Join(routes, ",")

	if err := exec.Command("nmcli", "connection", "show", hostDNSConnection).Run(); err != nil {
		output, addErr := exec.Command(
			"nmcli", "connection", "add",
			"type", "dummy", "ifname", hostDNSInterface, "con-name", hostDNSConnection,
			"ipv4.method", "manual", "ipv4.addresses", "192.0.2.1/32",
			"ipv4.never-default", "yes", "ipv4.dns", "127.0.0.1",
			"ipv4.dns-search", routeList, "ipv6.method", "disabled",
		).CombinedOutput()
		if addErr != nil {
			return fmt.Errorf("nmcli nao conseguiu criar %s: %s: %w", hostDNSConnection, strings.TrimSpace(string(output)), addErr)
		}
	} else {
		output, modifyErr := exec.Command(
			"nmcli", "connection", "modify", hostDNSConnection,
			"ipv4.dns", "127.0.0.1", "ipv4.dns-search", routeList,
			"ipv4.never-default", "yes", "ipv6.method", "disabled",
		).CombinedOutput()
		if modifyErr != nil {
			return fmt.Errorf("nmcli nao conseguiu atualizar %s: %s: %w", hostDNSConnection, strings.TrimSpace(string(output)), modifyErr)
		}
	}

	output, err := exec.Command("nmcli", "connection", "up", hostDNSConnection).CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli nao conseguiu ativar %s: %s: %w", hostDNSConnection, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func hostDNSRouteDomains(config map[string]string) []string {
	seen := map[string]bool{}
	for _, key := range []string{"DNS_LOCAL_TLDS", "DOMAINS"} {
		for _, raw := range strings.FieldsFunc(config[key], func(r rune) bool {
			return r == ',' || r == ';' || r == ' '
		}) {
			domain := normalizeRouteDomain(raw)
			if domain != "" {
				seen["~"+domain] = true
			}
		}
	}
	routes := make([]string, 0, len(seen))
	for route := range seen {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	return routes
}

func normalizeRouteDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimLeft(value, ".*")
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return ""
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return ""
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return ""
			}
		}
	}
	return value
}
