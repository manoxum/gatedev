package shaping

import (
	"strconv"
	"strings"
)

// Zona wan (cliente->internet). O chain BINDNET-FW-WAN e avaliado no
// FORWARD para o trafego -i ap -o uplink e usa RETURN para "permitir"
// (deixa o pacote seguir para BINDNET-HOTSPOT, onde ainda passam o
// bloqueio por credito e o MASQUERADE) e DROP para "bloquear". Por isso
// o jump precisa ficar ACIMA do jump de BINDNET-HOTSPOT no FORWARD:
// senao o ACCEPT generico do hotspot liberaria o pacote antes das
// nossas regras de bloqueio serem avaliadas.
const fwWanJumpComment = "bn-fw-wan-jump"

func syncWanChain(apIface, uplinkIface, policy string, rules []firewallZonePayload) error {
	if err := ensureWanJumpAboveHotspot(apIface, uplinkIface); err != nil {
		return err
	}
	return syncChainRules(fwWanChain, buildWanChainRules(policy, rules))
}

// buildWanChainRules monta, em ordem: RETURN para conexoes ja
// estabelecidas, uma entrada por regra (RETURN=permitir, DROP=bloquear)
// e a politica padrao no fim (RETURN se allow, DROP se deny).
func buildWanChainRules(policy string, rules []firewallZonePayload) []chainRule {
	out := []chainRule{{
		comment: "bn-fw-wan-est",
		args: []string{
			"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
			"-m", "comment", "--comment", "bn-fw-wan-est", "-j", "RETURN",
		},
	}}
	for index, rule := range rules {
		out = append(out, zoneChainRule(fwWanChain, index, rule, wanTarget(rule.Action), true))
	}
	defaultTarget := "RETURN"
	if policy == "deny" {
		defaultTarget = "DROP"
	}
	return append(out, chainRule{
		comment: "bn-fw-wan-default",
		args:    []string{"-m", "comment", "--comment", "bn-fw-wan-default", "-j", defaultTarget},
	})
}

func wanTarget(action string) string {
	if action == "deny" {
		return "DROP"
	}
	return "RETURN"
}

// ensureWanJumpAboveHotspot garante o jump do chain wan no FORWARD e,
// crucialmente, acima do jump de BINDNET-HOTSPOT. So mexe quando esta
// ausente ou abaixo do hotspot (esteve-vel no regime normal).
func ensureWanJumpAboveHotspot(apIface, uplinkIface string) error {
	wanIdx, hotspotIdx := forwardJumpPositions()
	if wanIdx >= 0 && (hotspotIdx < 0 || wanIdx < hotspotIdx) {
		return nil
	}
	deleteRulesByComment("", "FORWARD", fwWanJumpComment)
	return runIptables(
		"-I", "FORWARD", "1",
		"-i", apIface, "-o", uplinkIface,
		"-m", "comment", "--comment", fwWanJumpComment, "-j", fwWanChain,
	)
}

// forwardJumpPositions devolve os indices (ordem em "iptables -S
// FORWARD") do nosso jump wan e do jump de BINDNET-HOTSPOT, ou -1 se
// ausentes.
func forwardJumpPositions() (wanIdx, hotspotIdx int) {
	wanIdx, hotspotIdx = -1, -1
	for index, line := range strings.Split(iptablesListing("FORWARD"), "\n") {
		if strings.Contains(line, fwWanJumpComment) {
			wanIdx = index
		}
		if strings.Contains(line, "-j "+hotspotFilterChain) {
			hotspotIdx = index
		}
	}
	return wanIdx, hotspotIdx
}

// zoneChainRule monta a entrada iptables de uma regra de zona (wan ou
// local): casa MAC de origem (quando houver), protocolo/portas e, na
// wan, host de destino; com o alvo ja resolvido pela zona.
func zoneChainRule(chain string, index int, rule firewallZonePayload, target string, withHost bool) chainRule {
	comment := zoneRuleComment(chain, index, rule)
	args := []string{}
	if rule.MAC != "" {
		args = append(args, "-m", "mac", "--mac-source", rule.MAC)
	}
	if withHost && rule.DstHost != "" {
		args = append(args, "-d", rule.DstHost)
	}
	if rule.Protocol != "" && rule.Protocol != "any" {
		args = append(args, "-p", rule.Protocol)
		if rule.DstPorts != "" {
			args = append(args, "-m", "multiport", "--dports", rule.DstPorts)
		}
	}
	return chainRule{comment: comment, args: append(args, "-m", "comment", "--comment", comment, "-j", target)}
}

func zoneRuleComment(chain string, index int, rule firewallZonePayload) string {
	ports := rule.DstPorts
	if ports == "" {
		ports = "any"
	}
	host := rule.DstHost
	if host == "" {
		host = "any"
	}
	mac := rule.MAC
	if mac == "" {
		mac = "all"
	}
	return strings.Join([]string{
		strings.ToLower(chain), strconv.Itoa(index), rule.Action, rule.Protocol, ports, host,
		strings.ReplaceAll(mac, ":", ""),
	}, "-")
}
