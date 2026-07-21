package shaping

import (
	"os/exec"
	"strconv"
	"strings"
)

// isolationChain e o chain de filter/FORWARD da zona 'clients' do
// firewall: so ve o trafego hairpin cliente->cliente (-i ap -o ap, ver
// o jump em ensureIsolationJump) - o caminho cliente->internet
// (-o uplink) e o trafego para o proprio gateway (INPUT) nunca passam
// por aqui. Estrutura: RELATED,ESTABLISHED no topo, as entradas de
// firewall na ordem que o backend calculou (mais especifico primeiro,
// ver hotspot_isolation_policy.go) e um DROP no fim - default deny.
const isolationChain = "BINDNET-ISOLATION"

const (
	isolationJumpComment        = "bn-iso-jump"
	isolationEstablishedComment = "bniso:established"
	isolationDropComment        = "bniso:drop"
)

// isolationEntryComment codifica o conteudo completo de uma entrada (e
// sua posicao) num comentario deterministico - e a assinatura que
// syncIsolationChain compara para decidir se o chain ja esta no estado
// desejado sem precisar reconstruir.
func isolationEntryComment(index int, pair isolationPairPayload) string {
	ports := pair.DstPorts
	if ports == "" {
		ports = "any"
	}
	return strings.Join([]string{
		"bniso", strconv.Itoa(index), pair.Action, pair.Protocol, ports,
		strings.ReplaceAll(pair.MAC, ":", ""), pair.IP,
	}, "-")
}

// isolationEntryArgs monta os argumentos iptables de uma entrada:
// casa por MAC de origem + IP de destino, opcionalmente protocolo e
// portas de destino, com alvo ACCEPT (allow) ou DROP (deny).
func isolationEntryArgs(index int, pair isolationPairPayload) []string {
	args := []string{"-A", isolationChain, "-m", "mac", "--mac-source", pair.MAC, "-d", pair.IP}
	if pair.Protocol != "" && pair.Protocol != "any" {
		args = append(args, "-p", pair.Protocol)
		if pair.DstPorts != "" {
			args = append(args, "-m", "multiport", "--dports", pair.DstPorts)
		}
	}
	target := "ACCEPT"
	if pair.Action == "deny" {
		target = "DROP"
	}
	return append(args, "-m", "comment", "--comment", isolationEntryComment(index, pair), "-j", target)
}

// desiredIsolationComments e a assinatura ordenada do chain no estado
// desejado (established + entradas + drop) - comparada com a atual.
func desiredIsolationComments(pairs []isolationPairPayload) []string {
	comments := make([]string, 0, len(pairs)+2)
	comments = append(comments, isolationEstablishedComment)
	for index, pair := range pairs {
		comments = append(comments, isolationEntryComment(index, pair))
	}
	return append(comments, isolationDropComment)
}

// syncIsolationChain materializa o estado desejado do chain. Como a
// ordem das entradas importa (o firewall casa a primeira), o chain e
// reconstruido por inteiro quando a assinatura muda; quando ja bate,
// e um no-op (so uma leitura) - o caso comum a cada ciclo de
// reconciliacao.
func syncIsolationChain(apIface string, pairs []isolationPairPayload) error {
	_ = runIptables("-N", isolationChain)
	if err := ensureIsolationJump(apIface); err != nil {
		return err
	}

	desired := desiredIsolationComments(pairs)
	if equalStringSlices(currentIsolationComments(), desired) {
		return nil
	}

	if err := runIptables("-F", isolationChain); err != nil {
		return err
	}
	if err := runIptables(
		"-A", isolationChain,
		"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
		"-m", "comment", "--comment", isolationEstablishedComment, "-j", "ACCEPT",
	); err != nil {
		return err
	}
	for index, pair := range pairs {
		if err := runIptables(isolationEntryArgs(index, pair)...); err != nil {
			return err
		}
	}
	return runIptables(
		"-A", isolationChain,
		"-m", "comment", "--comment", isolationDropComment, "-j", "DROP",
	)
}

// currentIsolationComments le os comentarios das regras do chain, em
// ordem - a assinatura atual para comparar com a desejada.
func currentIsolationComments() []string {
	output, err := exec.Command("iptables", "-w", "-S", isolationChain).CombinedOutput()
	if err != nil {
		return nil
	}
	var comments []string
	for _, line := range strings.Split(string(output), "\n") {
		if idx := strings.Index(line, "--comment "); idx >= 0 {
			comments = append(comments, parseIptablesComment(line[idx+len("--comment "):]))
		}
	}
	return comments
}

// parseIptablesComment extrai o valor do comentario de um trecho de
// "iptables -S", que vem entre aspas (ex.: `"bniso:drop" -j DROP`).
func parseIptablesComment(rest string) string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "\"") {
		if end := strings.Index(rest[1:], "\""); end >= 0 {
			return rest[1 : 1+end]
		}
	}
	return strings.Fields(rest)[0]
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ensureIsolationJump garante o unico ponto de entrada do chain: um
// jump em FORWARD restrito ao hairpin da interface AP atual. Se a
// interface mudou (ap0 virtual vs placa fisica, conforme o modo do
// create_ap), remove o jump antigo pelo comentario antes de inserir o
// novo - nunca ficam dois jumps validos ao mesmo tempo.
func ensureIsolationJump(apIface string) error {
	if iptablesCheck(
		"-C", "FORWARD",
		"-i", apIface, "-o", apIface,
		"-m", "comment", "--comment", isolationJumpComment, "-j", isolationChain,
	) == nil {
		return nil
	}
	deleteRulesByComment("", "FORWARD", isolationJumpComment)
	return runIptables(
		"-I", "FORWARD", "1",
		"-i", apIface, "-o", apIface,
		"-m", "comment", "--comment", isolationJumpComment, "-j", isolationChain,
	)
}

// teardownIsolation desmonta o isolamento por completo (interruptor
// desligado ou hotspot parado): remove o jump de FORWARD, apaga o
// chain e reverte os sysctls de hairpin. Best-effort de proposito -
// com o hotspot ja parado a interface AP pode nem existir mais (os
// sysctls dela morrem junto), e nada aqui deve falhar o desligamento.
func teardownIsolation(ifaceParam string) {
	deleteRulesByComment("", "FORWARD", isolationJumpComment)
	_ = runIptables("-F", isolationChain)
	_ = runIptables("-X", isolationChain)
	if ifaceParam == "" {
		return
	}
	if apIface, _, err := resolveShapingInterfaces(ifaceParam); err == nil {
		revertIsolationSysctls(apIface)
	}
}
