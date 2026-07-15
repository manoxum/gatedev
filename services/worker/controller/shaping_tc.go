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
// dedicada) + classe pai 1:1 (teto global) + classe default 1:999. So
// cria a qdisc raiz quando ela ainda nao existe (ver hasHTBRootQdisc) -
// "tc qdisc replace" NAO e idempotente para htb: a primeira chamada
// cria normalmente, mas qualquer chamada seguinte falha com "Error:
// Change operation not supported by specified qdisc." (confirmado ao
// vivo neste host - o kernel recusa reconfigurar uma qdisc htb raiz ja
// existente via netlink "change", so classes filhas suportam "replace"
// de verdade). Como ensureRootQdisc roda a cada shaping de dispositivo
// (todo poll de 2-3s da pagina de detalhe, todo ciclo de reconciliacao),
// sem essa checagem a segunda chamada em diante sempre falhava,
// quebrando shaping E contagem ao vivo pra qualquer dispositivo com
// taxa configurada.
func ensureRootQdisc(iface string) error {
	if !hasHTBRootQdisc(iface) {
		if err := runTC("qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "999"); err != nil {
			return fmt.Errorf("qdisc raiz em %s: %w", iface, err)
		}
	}
	if err := runTC("class", "replace", "dev", iface, "parent", "1:", "classid", "1:1",
		"htb", "rate", rate(noLimitCeilMbps, rateUnitMbit), "ceil", rate(noLimitCeilMbps, rateUnitMbit)); err != nil {
		return fmt.Errorf("classe pai 1:1 em %s: %w", iface, err)
	}
	if err := runTC("class", "replace", "dev", iface, "parent", "1:1", "classid", "1:999",
		"htb", "rate", rate(minGuaranteedMbps, rateUnitMbit), "ceil", rate(noLimitCeilMbps, rateUnitMbit)); err != nil {
		return fmt.Errorf("classe default 1:999 em %s: %w", iface, err)
	}
	return nil
}

// hasHTBRootQdisc verifica se a interface ja tem a qdisc raiz htb
// (handle 1:) que ensureRootQdisc cria - ver o comentario ali sobre por
// que isso e necessario (tc qdisc replace nao e idempotente para htb).
func hasHTBRootQdisc(iface string) bool {
	output, err := exec.Command("tc", "qdisc", "show", "dev", iface).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "qdisc htb 1: root")
}

// updateRootCeil atualiza so o teto da classe pai 1:1 (limite global)
// sem recriar qdisc/classe default - chamado sempre que o admin muda o
// limite global, ou com noLimitCeilMbps quando ele remove o limite.
func updateRootCeil(iface string, ceilValue int, ceilUnit string) error {
	if ceilValue <= 0 || ceilUnit == "" {
		ceilValue, ceilUnit = noLimitCeilMbps, rateUnitMbit
	}
	if err := runTC("class", "replace", "dev", iface, "parent", "1:", "classid", "1:1",
		"htb", "rate", rate(ceilValue, ceilUnit), "ceil", rate(ceilValue, ceilUnit)); err != nil {
		return fmt.Errorf("atualizar teto global em %s: %w", iface, err)
	}
	return nil
}

// deviceClassID monta o classid tc (major:minor) de um fwmark - o
// "minor" de um classid tc e sempre interpretado em HEXADECIMAL pela
// ferramenta (convencao propria do tc, independente da base usada no
// "handle" do filtro fw abaixo, que e decimal - confirmado ao vivo
// neste host). Formatar em decimal fazia qualquer fwmark >= 10000 virar
// um classid > 0xffff ("Error: ... invalid class ID") - a sequence
// hotspot_device_fwmark_seq comeca em 100 e so cresce (ver migration
// 20260703000000_init_hotspot_shaping), entao isso quebrava a classe
// HTB dedicada (e o shaping) de praticamente todo dispositivo depois
// dos primeiros ~9900 vistos pelo Bindnet.
func deviceClassID(fwmark int) string {
	return fmt.Sprintf("1:%x", fwmark)
}

// ensureDeviceClass cria/atualiza a classe HTB dedicada de um
// dispositivo (classid = fwmark, sempre dentro de 1:1) e o filtro que
// classifica pacotes marcados com esse fwmark nela.
func ensureDeviceClass(iface string, fwmark, rateValue int, rateUnitValue string) error {
	classID := deviceClassID(fwmark)
	guaranteedValue, guaranteedUnit := guaranteedRate(rateValue, rateUnitValue)
	if err := runTC("class", "replace", "dev", iface, "parent", "1:1", "classid", classID,
		"htb", "rate", rate(guaranteedValue, guaranteedUnit), "ceil", rate(rateValue, rateUnitValue)); err != nil {
		return fmt.Errorf("classe %s em %s: %w", classID, iface, err)
	}
	if err := runTC("filter", "replace", "dev", iface, "parent", "1:", "protocol", "ip", "pref", "1",
		"handle", strconv.Itoa(fwmark), "fw", "flowid", classID); err != nil {
		return fmt.Errorf("filtro fwmark %d em %s: %w", fwmark, iface, err)
	}
	return nil
}

// guaranteedRate devolve o "rate" (piso garantido) da classe HTB
// dedicada de um dispositivo: minGuaranteedMbps, exceto quando o teto
// (ceil) configurado pelo admin e menor que isso - HTB exige rate <=
// ceil na mesma classe (confirmado pela doc do tc), entao nesses
// casos (qualquer limite abaixo de 1 Mbit/s, ex.: os 25 kbyte/s de um
// perfil "convidado") o piso garantido vira o proprio teto, sem
// inflar a garantia acima do que o admin permitiu.
func guaranteedRate(ceilValue int, ceilUnit string) (int, string) {
	if bitsPerSecond(ceilValue, ceilUnit) < bitsPerSecond(minGuaranteedMbps, rateUnitMbit) {
		return ceilValue, ceilUnit
	}
	return minGuaranteedMbps, rateUnitMbit
}

// bitsPerSecond converte um valor+unidade da API (mesmas unidades de
// tcRateSuffix) pra bits/s, usando prefixo decimal (SI) igual ao tc -
// so serve pra comparar taxas de unidades diferentes, nunca e passado
// direto pro tc (rate/tcRateSuffix continuam usando o valor+unidade
// originais).
func bitsPerSecond(value int, unit string) int64 {
	v := int64(value)
	switch unit {
	case "kbit":
		return v * 1_000
	case "gbit":
		return v * 1_000_000_000
	case "kbyte":
		return v * 1_000 * 8
	case "mbyte":
		return v * 1_000_000 * 8
	case "gbyte":
		return v * 1_000_000_000 * 8
	default: // "mbit" e qualquer valor vazio/desconhecido (mesmo default de tcRateSuffix)
		return v * 1_000_000
	}
}

// removeDeviceClass remove o filtro e a classe dedicada de um
// dispositivo, best-effort - o trafego dele volta a cair na classe
// default 1:999 (some limite proprio, so o teto global se houver).
func removeDeviceClass(iface string, fwmark int) {
	_ = runTC("filter", "del", "dev", iface, "parent", "1:", "protocol", "ip", "pref", "1",
		"handle", strconv.Itoa(fwmark), "fw")
	_ = runTC("class", "del", "dev", iface, "classid", deviceClassID(fwmark))
}

// teardownRootQdisc remove a hierarquia HTB inteira de uma interface,
// best-effort (chamado no teardown quando o hotspot para - os qdiscs
// morrem de qualquer forma junto com a interface, isso e so higiene).
func teardownRootQdisc(iface string) {
	_ = runTC("qdisc", "del", "dev", iface, "root")
}

// rateUnitMbit e o default usado internamente (teto nominal
// noLimitCeilMbps, rate minimo garantido) - nao vem do admin, entao
// nao precisa das outras 5 unidades que a API aceita.
const rateUnitMbit = "mbit"

// tcRateSuffix traduz a unidade da API (kbit/mbit/gbit em bits/s,
// kbyte/mbyte/gbyte em bytes/s) para o sufixo que o tc entende - tc
// usa kbit/mbit/gbit para bits/s e kbps/mbps/gbps (nome confuso, mas e
// assim que a ferramenta distingue) para bytes/s.
func tcRateSuffix(unit string) string {
	switch unit {
	case "kbyte":
		return "kbps"
	case "mbyte":
		return "mbps"
	case "gbyte":
		return "gbps"
	case "kbit", "gbit":
		return unit
	default:
		return "mbit"
	}
}

// rate monta o argumento de taxa do tc (ex.: "100mbit", "500kbps") a
// partir do valor digitado pelo admin e da unidade escolhida na UI.
func rate(value int, unit string) string {
	return strconv.Itoa(value) + tcRateSuffix(unit)
}

func runTC(args ...string) error {
	output, err := exec.Command("tc", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
