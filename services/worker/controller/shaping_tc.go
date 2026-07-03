package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// noLimitCeilMbps e o teto nominal usado quando nao ha limite Mbps
// configurado (global ou por dispositivo) - alto o bastante para nunca
// ser o fator limitante num hotspot domestico real, mas mantendo a
// hierarquia HTB sempre presente (evita ter que criar/destruir qdiscs
// condicionalmente a cada mudanca de configuracao).
const noLimitCeilMbps = 10000

// minGuaranteedMbps e a taxa minima garantida (rate) de qualquer
// classe HTB dedicada - o "ceil" e que impõe o teto real configurado
// pelo admin; o "rate" so evita inanicao total sob contencao.
const minGuaranteedMbps = 1

// ensureRootQdisc garante a hierarquia HTB minima em uma interface:
// qdisc raiz (default 1:999, catch-all para quem nao tem classe
// dedicada) + classe pai 1:1 (teto global) + classe default 1:999.
// Idempotente via "tc ... replace" - nao falha se ja existir.
func ensureRootQdisc(iface string) error {
	if err := runTC("qdisc", "replace", "dev", iface, "root", "handle", "1:", "htb", "default", "999"); err != nil {
		return fmt.Errorf("qdisc raiz em %s: %w", iface, err)
	}
	if err := runTC("class", "replace", "dev", iface, "parent", "1:", "classid", "1:1",
		"htb", "rate", mbit(noLimitCeilMbps), "ceil", mbit(noLimitCeilMbps)); err != nil {
		return fmt.Errorf("classe pai 1:1 em %s: %w", iface, err)
	}
	if err := runTC("class", "replace", "dev", iface, "parent", "1:1", "classid", "1:999",
		"htb", "rate", mbit(minGuaranteedMbps), "ceil", mbit(noLimitCeilMbps)); err != nil {
		return fmt.Errorf("classe default 1:999 em %s: %w", iface, err)
	}
	return nil
}

// updateRootCeil atualiza so o teto da classe pai 1:1 (limite global)
// sem recriar qdisc/classe default - chamado sempre que o admin muda o
// limite global, ou com noLimitCeilMbps quando ele remove o limite.
func updateRootCeil(iface string, ceilMbps int) error {
	if ceilMbps <= 0 {
		ceilMbps = noLimitCeilMbps
	}
	if err := runTC("class", "replace", "dev", iface, "parent", "1:", "classid", "1:1",
		"htb", "rate", mbit(ceilMbps), "ceil", mbit(ceilMbps)); err != nil {
		return fmt.Errorf("atualizar teto global em %s: %w", iface, err)
	}
	return nil
}

// ensureDeviceClass cria/atualiza a classe HTB dedicada de um
// dispositivo (classid = fwmark, sempre dentro de 1:1) e o filtro que
// classifica pacotes marcados com esse fwmark nela.
func ensureDeviceClass(iface string, fwmark, rateMbps int) error {
	classID := fmt.Sprintf("1:%d", fwmark)
	if err := runTC("class", "replace", "dev", iface, "parent", "1:1", "classid", classID,
		"htb", "rate", mbit(minGuaranteedMbps), "ceil", mbit(rateMbps)); err != nil {
		return fmt.Errorf("classe %s em %s: %w", classID, iface, err)
	}
	if err := runTC("filter", "replace", "dev", iface, "parent", "1:", "protocol", "ip", "pref", "1",
		"handle", strconv.Itoa(fwmark), "fw", "flowid", classID); err != nil {
		return fmt.Errorf("filtro fwmark %d em %s: %w", fwmark, iface, err)
	}
	return nil
}

// removeDeviceClass remove o filtro e a classe dedicada de um
// dispositivo, best-effort - o trafego dele volta a cair na classe
// default 1:999 (some limite proprio, so o teto global se houver).
func removeDeviceClass(iface string, fwmark int) {
	_ = runTC("filter", "del", "dev", iface, "parent", "1:", "protocol", "ip", "pref", "1",
		"handle", strconv.Itoa(fwmark), "fw")
	_ = runTC("class", "del", "dev", iface, "classid", fmt.Sprintf("1:%d", fwmark))
}

// teardownRootQdisc remove a hierarquia HTB inteira de uma interface,
// best-effort (chamado no teardown quando o hotspot para - os qdiscs
// morrem de qualquer forma junto com a interface, isso e so higiene).
func teardownRootQdisc(iface string) {
	_ = runTC("qdisc", "del", "dev", iface, "root")
}

func mbit(mbps int) string {
	return strconv.Itoa(mbps) + "mbit"
}

func runTC(args ...string) error {
	output, err := exec.Command("tc", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
