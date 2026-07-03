package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// shapingChain e um chain proprio na tabela mangle, independente do
// BINDNET-HOTSPOT existente (filter/nat) criado por
// services/worker/hotspot/interfaces.sh - so marca/conta trafego por
// dispositivo, nunca decide accept/drop.
const shapingChain = "BINDNET-SHAPING"

// ensureShapingChain cria o chain (idempotente, ignora erro "already
// exists") e garante o jump a partir de mangle/FORWARD, mesmo padrao
// de ensure_iptables_chain/ensure_iptables_jump em interfaces.sh.
func ensureShapingChain() error {
	_ = runIptables("-t", "mangle", "-N", shapingChain)
	if err := iptablesCheck("-t", "mangle", "-C", "FORWARD", "-j", shapingChain); err != nil {
		if err := runIptables("-t", "mangle", "-I", "FORWARD", "1", "-j", shapingChain); err != nil {
			return fmt.Errorf("jump mangle/FORWARD -> %s: %w", shapingChain, err)
		}
	}
	return nil
}

// ensureDeviceMarkRules garante a regra de upload (casada por MAC de
// origem, estavel - nao muda com renovacao de DHCP) e sempre
// reaplica a regra de download (casada por IP de destino) com o IP
// atual, ja que o worker nao guarda o IP anterior - reenviar a cada
// ciclo de reconciliacao do backend resolve a renovacao de DHCP sem
// precisar detectar a mudanca.
func ensureDeviceMarkRules(apIface, uplinkIface, mac, ip string, fwmark int) error {
	upComment := "bn-up-" + mac
	markHex := "0x" + strconv.FormatInt(int64(fwmark), 16)
	if err := iptablesCheck("-t", "mangle", "-C", shapingChain,
		"-i", apIface, "-o", uplinkIface, "-m", "mac", "--mac-source", mac,
		"-m", "comment", "--comment", upComment, "-j", "MARK", "--set-mark", markHex); err != nil {
		if err := runIptables("-t", "mangle", "-A", shapingChain,
			"-i", apIface, "-o", uplinkIface, "-m", "mac", "--mac-source", mac,
			"-m", "comment", "--comment", upComment, "-j", "MARK", "--set-mark", markHex); err != nil {
			return fmt.Errorf("regra de upload para %s: %w", mac, err)
		}
	}
	return refreshDeviceIP(apIface, uplinkIface, mac, ip, fwmark)
}

// refreshDeviceIP e um no-op quando a regra de download do dispositivo
// ja casa com o IP atual (preserva o contador de bytes entre chamadas -
// ensureDeviceShaping e chamada a cada poll de 2-3s da pagina de
// detalhe e a cada ciclo de reconciliacao, entao um "sempre recria"
// aqui zerava a contagem de download constantemente, impedindo a
// velocidade ao vivo de refletir tráfego real). So apaga (por
// comentario) e recria quando o IP realmente mudou - renovacao de DHCP
// - sem precisar saber qual era o IP antigo.
func refreshDeviceIP(apIface, uplinkIface, mac, ip string, fwmark int) error {
	downComment := "bn-down-" + mac
	markHex := "0x" + strconv.FormatInt(int64(fwmark), 16)
	if err := iptablesCheck("-t", "mangle", "-C", shapingChain,
		"-i", uplinkIface, "-o", apIface, "-d", ip,
		"-m", "comment", "--comment", downComment, "-j", "MARK", "--set-mark", markHex); err == nil {
		return nil
	}
	deleteRulesByComment(downComment)
	if err := runIptables("-t", "mangle", "-A", shapingChain,
		"-i", uplinkIface, "-o", apIface, "-d", ip,
		"-m", "comment", "--comment", downComment, "-j", "MARK", "--set-mark", markHex); err != nil {
		return fmt.Errorf("regra de download para %s: %w", mac, err)
	}
	return nil
}

// applyGlobalMarkRules conta todo trafego ap0<->bn-uplink, sem casar
// por MAC/IP - usa "-j RETURN" (nunca ACCEPT/DROP) porque o objetivo e
// so contar; RETURN devolve o pacote pra cadeia chamadora (FORWARD)
// sem alterar o veredito, e o contador do iptables incrementa em
// qualquer regra que case, seja qual for o alvo.
func applyGlobalMarkRules(apIface, uplinkIface string) error {
	if err := iptablesCheck("-t", "mangle", "-C", shapingChain,
		"-i", apIface, "-o", uplinkIface, "-m", "comment", "--comment", "bn-global-up", "-j", "RETURN"); err != nil {
		if err := runIptables("-t", "mangle", "-A", shapingChain,
			"-i", apIface, "-o", uplinkIface, "-m", "comment", "--comment", "bn-global-up", "-j", "RETURN"); err != nil {
			return fmt.Errorf("regra global de upload: %w", err)
		}
	}
	if err := iptablesCheck("-t", "mangle", "-C", shapingChain,
		"-i", uplinkIface, "-o", apIface, "-m", "comment", "--comment", "bn-global-down", "-j", "RETURN"); err != nil {
		if err := runIptables("-t", "mangle", "-A", shapingChain,
			"-i", uplinkIface, "-o", apIface, "-m", "comment", "--comment", "bn-global-down", "-j", "RETURN"); err != nil {
			return fmt.Errorf("regra global de download: %w", err)
		}
	}
	return nil
}

// readCounters le os bytes acumulados nas regras de marca do
// dispositivo, buscando pelos comentarios dedicados - nao depende de
// nenhuma classe tc existir, funciona so com as regras de iptables.
func readCounters(mac string) (downloadBytes, uploadBytes uint64, err error) {
	return readCommentCounters("bn-down-"+mac, "bn-up-"+mac)
}

func readGlobalCounters() (downloadBytes, uploadBytes uint64, err error) {
	return readCommentCounters("bn-global-down", "bn-global-up")
}

func readCommentCounters(downComment, upComment string) (downloadBytes, uploadBytes uint64, err error) {
	output, err := exec.Command("iptables", "-w", "-t", "mangle", "-L", shapingChain, "-v", "-x", "-n").CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("iptables -L %s: %s: %w", shapingChain, strings.TrimSpace(string(output)), err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		bytes, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}
		switch {
		case strings.Contains(line, "/* "+downComment+" */"):
			downloadBytes = bytes
		case strings.Contains(line, "/* "+upComment+" */"):
			uploadBytes = bytes
		}
	}
	return downloadBytes, uploadBytes, nil
}

// removeDeviceMarkRules apaga as duas regras de marca/contagem do
// dispositivo, best-effort (chamado quando o limite/rastreamento de um
// dispositivo e removido).
func removeDeviceMarkRules(mac string) {
	deleteRulesByComment("bn-up-" + mac)
	deleteRulesByComment("bn-down-" + mac)
}

// deleteRulesByComment apaga, em loop, toda regra do shapingChain que
// contenha o comentario informado - mesmo padrao de
// remove_iptables_jump em interfaces.sh (repete ate o -D falhar).
func deleteRulesByComment(comment string) {
	output, err := exec.Command("iptables", "-w", "-t", "mangle", "-L", shapingChain, "--line-numbers", "-n").CombinedOutput()
	if err != nil {
		return
	}
	for {
		lineNumber := findRuleLineByComment(string(output), comment)
		if lineNumber == "" {
			return
		}
		if err := runIptables("-t", "mangle", "-D", shapingChain, lineNumber); err != nil {
			return
		}
		output, err = exec.Command("iptables", "-w", "-t", "mangle", "-L", shapingChain, "--line-numbers", "-n").CombinedOutput()
		if err != nil {
			return
		}
	}
}

func findRuleLineByComment(listing, comment string) string {
	for _, line := range strings.Split(listing, "\n") {
		if strings.Contains(line, "/* "+comment+" */") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}

// flushShapingChain limpa todas as regras (best-effort, chamado no
// teardown quando o hotspot para).
func flushShapingChain() {
	_ = runIptables("-t", "mangle", "-F", shapingChain)
}

func runIptables(args ...string) error {
	output, err := exec.Command("iptables", append([]string{"-w"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func iptablesCheck(args ...string) error {
	_, err := exec.Command("iptables", append([]string{"-w"}, args...)...).CombinedOutput()
	return err
}
