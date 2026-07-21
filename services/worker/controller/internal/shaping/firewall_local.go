package shaping

// Zona local (cliente->gateway/serviços do host). O chain
// BINDNET-FW-LOCAL e avaliado no INPUT para o trafego -i ap e usa
// ACCEPT para "permitir" (entrega o pacote ao serviço) e DROP para
// "bloquear". No TOPO ficam permissoes fixas e inegociaveis
// (established, DHCP, DNS, painel HTTP/HTTPS, ICMP) para o operador
// NUNCA conseguir se trancar fora do painel/rede, mesmo com politica
// padrao 'deny'. A politica padrao 'allow' usa RETURN (nao interfere no
// resto do INPUT do host); 'deny' usa DROP.
const fwLocalJumpComment = "bn-fw-local-jump"

// essentialLocalAllows sao as portas/protocolos que o cliente sempre
// pode falar com o gateway, independente de regras/politica - a rede de
// seguranca contra auto-bloqueio. Ordem: established primeiro.
var essentialLocalAllows = []struct {
	comment  string
	protocol string
	ports    string
}{
	{"bn-fw-local-dhcp", "udp", "67"},
	{"bn-fw-local-dns-udp", "udp", "53"},
	{"bn-fw-local-dns-tcp", "tcp", "53"},
	{"bn-fw-local-http", "tcp", "80"},
	{"bn-fw-local-https", "tcp", "443"},
	{"bn-fw-local-icmp", "icmp", ""},
}

func syncLocalChain(apIface, policy string, rules []firewallZonePayload) error {
	if err := ensureLocalJump(apIface); err != nil {
		return err
	}
	return syncChainRules(fwLocalChain, buildLocalChainRules(policy, rules))
}

func buildLocalChainRules(policy string, rules []firewallZonePayload) []chainRule {
	out := []chainRule{{
		comment: "bn-fw-local-est",
		args: []string{
			"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
			"-m", "comment", "--comment", "bn-fw-local-est", "-j", "ACCEPT",
		},
	}}
	for _, essential := range essentialLocalAllows {
		args := []string{}
		if essential.protocol != "" {
			args = append(args, "-p", essential.protocol)
			if essential.ports != "" {
				args = append(args, "--dport", essential.ports)
			}
		}
		args = append(args, "-m", "comment", "--comment", essential.comment, "-j", "ACCEPT")
		out = append(out, chainRule{comment: essential.comment, args: args})
	}
	for index, rule := range rules {
		out = append(out, zoneChainRule(fwLocalChain, index, rule, localTarget(rule.Action), false))
	}
	defaultTarget := "RETURN"
	if policy == "deny" {
		defaultTarget = "DROP"
	}
	return append(out, chainRule{
		comment: "bn-fw-local-default",
		args:    []string{"-m", "comment", "--comment", "bn-fw-local-default", "-j", defaultTarget},
	})
}

func localTarget(action string) string {
	if action == "deny" {
		return "DROP"
	}
	return "ACCEPT"
}

func ensureLocalJump(apIface string) error {
	if iptablesCheck(
		"-C", "INPUT",
		"-i", apIface,
		"-m", "comment", "--comment", fwLocalJumpComment, "-j", fwLocalChain,
	) == nil {
		return nil
	}
	deleteRulesByComment("", "INPUT", fwLocalJumpComment)
	return runIptables(
		"-I", "INPUT", "1",
		"-i", apIface,
		"-m", "comment", "--comment", fwLocalJumpComment, "-j", fwLocalChain,
	)
}
