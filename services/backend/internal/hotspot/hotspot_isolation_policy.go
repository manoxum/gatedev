// hotspot_isolation_policy.go e o motor PURO da zona 'clients' do
// firewall: dado quem esta conectado, o perfil efetivo de cada MAC e as
// regras, compila as entradas de firewall (por par MAC origem -> IP
// destino, com protocolo/portas/acao) que o worker instala no chain
// BINDNET-ISOLATION. Sem banco/HTTP aqui de proposito - e o que permite
// testar as modalidades em hotspot_isolation_policy_test.go sem rede.
//
// Semantica de firewall: para cada par ordenado, as regras aplicaveis
// sao ordenadas por especificidade de ponta (dispositivo > perfil >
// qualquer), depois por especificidade L4 (protocolo/portas concretos
// acima de 'any'), e por fim bloquear antes de permitir em empate. O
// worker instala nessa ordem (iptables casa a primeira) e um DROP final
// no chain garante o default deny.
package hotspot

import (
	"sort"

	"bindnet/backend/internal/hotspot/store"
)

// isolationClient e o minimo que o motor precisa saber de um cliente
// conectado agora (subconjunto de workerHotspotClient).
type isolationClient struct {
	MAC string
	IP  string
}

// firewallPairRule e uma entrada concreta do chain da zona clients: o
// MAC origem falando com o IP destino, opcionalmente restrita a um
// protocolo/portas, permitida ou bloqueada.
type firewallPairRule struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Protocol string `json:"protocol"`
	DstPorts string `json:"dstPorts"`
	Action   string `json:"action"`
}

type applicableRule struct {
	endpointSpec int
	l4Spec       int
	action       string
	protocol     string
	dstPorts     string
}

func endpointSpecificity(kind string) int {
	switch kind {
	case store.CommEndpointDevice:
		return 2
	case store.CommEndpointProfile:
		return 1
	default:
		return 0
	}
}

func endpointMatches(kind string, ref *string, mac, profileID string) bool {
	switch kind {
	case store.CommEndpointDevice:
		return ref != nil && *ref == mac
	case store.CommEndpointProfile:
		return ref != nil && *ref == profileID
	case store.CommEndpointAny:
		return true
	default:
		return false
	}
}

// l4Specificity pontua o quao especifico e o casamento L4 de uma regra:
// protocolo concreto conta, portas contam - para uma regra "tcp 443"
// sobrepor uma "any" mesmo com a mesma especificidade de ponta.
func l4Specificity(protocol, dstPorts string) int {
	score := 0
	if protocol != "" && protocol != store.CommProtocolAny {
		score++
	}
	if dstPorts != "" {
		score++
	}
	return score
}

func ruleAppliesToPair(rule store.CommRule, srcMAC, srcProfile, dstMAC, dstProfile string) bool {
	forward := endpointMatches(rule.SourceKind, &rule.SourceRef, srcMAC, srcProfile) &&
		endpointMatches(rule.TargetKind, rule.TargetRef, dstMAC, dstProfile)
	if forward {
		return true
	}
	if rule.Direction == store.CommDirectionBoth {
		return endpointMatches(rule.SourceKind, &rule.SourceRef, dstMAC, dstProfile) &&
			endpointMatches(rule.TargetKind, rule.TargetRef, srcMAC, srcProfile)
	}
	return false
}

// applicableRulesForPair reune as regras da zona clients que casam com o
// par (mais o allow implicito do perfil interno) e as ordena como o
// firewall avalia: ponta mais especifica primeiro, L4 mais especifico
// primeiro, bloquear antes de permitir em empate.
func applicableRulesForPair(srcMAC, dstMAC string, profileOf map[string]string, internalAllow map[string]bool, rules []store.CommRule) []applicableRule {
	srcProfile := profileOf[srcMAC]
	dstProfile := profileOf[dstMAC]

	var entries []applicableRule
	if srcProfile != "" && srcProfile == dstProfile && internalAllow[srcProfile] {
		entries = append(entries, applicableRule{endpointSpec: 2, action: store.CommActionAllow, protocol: store.CommProtocolAny})
	}
	for _, rule := range rules {
		if !rule.Enabled || rule.Zone != store.CommZoneClients {
			continue
		}
		if !ruleAppliesToPair(rule, srcMAC, srcProfile, dstMAC, dstProfile) {
			continue
		}
		ports := ""
		if rule.DstPorts != nil {
			ports = *rule.DstPorts
		}
		entries = append(entries, applicableRule{
			endpointSpec: endpointSpecificity(rule.SourceKind) + endpointSpecificity(rule.TargetKind),
			l4Spec:       l4Specificity(rule.Protocol, ports),
			action:       rule.Action,
			protocol:     rule.Protocol,
			dstPorts:     ports,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].endpointSpec != entries[j].endpointSpec {
			return entries[i].endpointSpec > entries[j].endpointSpec
		}
		if entries[i].l4Spec != entries[j].l4Spec {
			return entries[i].l4Spec > entries[j].l4Spec
		}
		// Empate total: bloquear antes de permitir (deny vence).
		return entries[i].action == store.CommActionDeny && entries[j].action == store.CommActionAllow
	})
	return entries
}

// compileClientsZonePairs compila o estado desejado completo do chain de
// isolamento para os clientes conectados agora: para cada par ordenado
// (X -> Y) cujo IP de Y ja e conhecido, as entradas de firewall na
// ordem em que o worker deve instala-las. Pares sem nenhuma regra de
// permitir aplicavel sao omitidos (o DROP final do chain cobre). MACs
// sem perfil conhecido caem no perfil Padrao (mesma semantica do
// COALESCE de HotspotDeviceProfileRefs).
func compileClientsZonePairs(clients []isolationClient, profileOf map[string]string, internalAllow map[string]bool, rules []store.CommRule) []firewallPairRule {
	effectiveProfiles := effectiveProfileMap(clients, profileOf)

	pairs := []firewallPairRule{}
	for _, src := range clients {
		for _, dst := range clients {
			if src.MAC == dst.MAC || dst.IP == "" {
				continue
			}
			entries := applicableRulesForPair(src.MAC, dst.MAC, effectiveProfiles, internalAllow, rules)
			if !hasAllow(entries) {
				continue
			}
			for _, entry := range entries {
				pairs = append(pairs, firewallPairRule{
					MAC:      src.MAC,
					IP:       dst.IP,
					Protocol: entry.protocol,
					DstPorts: entry.dstPorts,
					Action:   entry.action,
				})
			}
		}
	}
	return pairs
}

func hasAllow(entries []applicableRule) bool {
	for _, entry := range entries {
		if entry.action == store.CommActionAllow {
			return true
		}
	}
	return false
}

// effectiveProfileMap resolve o perfil efetivo de cada MAC conectado -
// MAC sem perfil conhecido cai no perfil Padrao (mesma semantica do
// COALESCE de HotspotDeviceProfileRefs). Compartilhado pelas zonas
// clients/wan/local.
func effectiveProfileMap(clients []isolationClient, profileOf map[string]string) map[string]string {
	effective := make(map[string]string, len(clients))
	for _, client := range clients {
		if profileID, ok := profileOf[client.MAC]; ok && profileID != "" {
			effective[client.MAC] = profileID
		} else {
			effective[client.MAC] = store.DefaultProfileID
		}
	}
	return effective
}
