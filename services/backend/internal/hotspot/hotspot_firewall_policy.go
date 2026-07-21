// hotspot_firewall_policy.go e o motor PURO das zonas 'wan'
// (cliente->internet) e 'local' (cliente->gateway) do firewall. Ao
// contrario da zona clients (que enumera pares por IP destino), aqui o
// destino e implicito (internet ou gateway): cada regra casa pela
// ORIGEM (MAC do cliente, ou todos) + L4 (protocolo/portas) + host
// externo (so wan). As entradas saem na ordem em que o firewall avalia:
// origem mais especifica primeiro (dispositivo > perfil > todos),
// depois L4 mais especifico, e bloquear antes de permitir em empate. A
// politica padrao da zona (allow/deny) e aplicada pelo worker no fim.
package hotspot

import (
	"sort"

	"bindnet/backend/internal/hotspot/store"
)

// firewallZoneRule e uma entrada concreta de uma zona wan/local: o MAC
// de origem (vazio = todos os clientes), opcionalmente restrita a
// protocolo/portas e, na zona wan, a um host/CIDR externo.
type firewallZoneRule struct {
	MAC      string `json:"mac"`
	Protocol string `json:"protocol"`
	DstPorts string `json:"dstPorts"`
	DstHost  string `json:"dstHost"`
	Action   string `json:"action"`
}

type zoneEntry struct {
	sourceSpec int
	l4Spec     int
	rule       firewallZoneRule
}

// compileZoneRules compila as entradas ordenadas de uma zona (wan ou
// local) para os clientes conectados agora. Regras de origem 'profile'
// sao expandidas para os MACs conectados daquele perfil; origem 'any'
// vira uma entrada sem filtro de MAC (casa todos os clientes); origem
// 'device' usa o MAC direto (mesmo desconectado - a regra fica pronta
// para quando ele voltar).
func compileZoneRules(zone string, clients []isolationClient, profileOf map[string]string, rules []store.CommRule) []firewallZoneRule {
	effective := effectiveProfileMap(clients, profileOf)

	var entries []zoneEntry
	for _, rule := range rules {
		if !rule.Enabled || rule.Zone != zone {
			continue
		}
		ports := ""
		if rule.DstPorts != nil {
			ports = *rule.DstPorts
		}
		host := ""
		if rule.DstHost != nil {
			host = *rule.DstHost
		}
		for _, mac := range zoneSourceMACs(rule, clients, effective) {
			entries = append(entries, zoneEntry{
				sourceSpec: endpointSpecificity(rule.SourceKind),
				l4Spec:     l4Specificity(rule.Protocol, ports),
				rule: firewallZoneRule{
					MAC: mac, Protocol: rule.Protocol, DstPorts: ports, DstHost: host, Action: rule.Action,
				},
			})
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].sourceSpec != entries[j].sourceSpec {
			return entries[i].sourceSpec > entries[j].sourceSpec
		}
		if entries[i].l4Spec != entries[j].l4Spec {
			return entries[i].l4Spec > entries[j].l4Spec
		}
		return entries[i].rule.Action == store.CommActionDeny && entries[j].rule.Action == store.CommActionAllow
	})

	out := make([]firewallZoneRule, len(entries))
	for i, entry := range entries {
		out[i] = entry.rule
	}
	return out
}

// zoneSourceMACs traduz a origem da regra em MACs a casar: device -> o
// proprio MAC; profile -> os MACs conectados desse perfil; any -> uma
// entrada com MAC vazio (sem filtro de origem, casa todos os clientes).
func zoneSourceMACs(rule store.CommRule, clients []isolationClient, effective map[string]string) []string {
	switch rule.SourceKind {
	case store.CommEndpointDevice:
		return []string{rule.SourceRef}
	case store.CommEndpointProfile:
		var macs []string
		for _, client := range clients {
			if effective[client.MAC] == rule.SourceRef {
				macs = append(macs, client.MAC)
			}
		}
		return macs
	default:
		return []string{""}
	}
}
