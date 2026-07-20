package hotspot

import (
	"bindnet/worker/internal/compose"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
)

// dnsmasqDHCPLog e o caminho fixo (nao o CONFDIR aleatorio do
// create_ap) onde services/worker/hotspot/patch-create-ap.sh configura
// o dnsmasq pra logar "log-dhcp" - unica forma de capturar as opcoes
// DHCP que cada dispositivo pede (usadas pelo backend para heuristicas
// locais de tipo/SO).
const dnsmasqDHCPLog = "/tmp/bindnet-dnsmasq-dhcp.log"

func RegisterFingerprintRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /hotspot/fingerprint", handleHotspotFingerprint)
}

type hotspotFingerprint struct {
	DHCPFingerprint string `json:"dhcpFingerprint,omitempty"`
	DHCPVendor      string `json:"dhcpVendor,omitempty"`
}

// handleHotspotFingerprint le o log de DHCP do dnsmasq e devolve, para
// o MAC pedido, os dados brutos que o dispositivo mandou no
// DHCPDISCOVER/DHCPREQUEST (lista de opcoes pedidas + vendor class, se
// houver) - services/backend cruza isso com fabricante por OUI, este
// endpoint nao chama nenhum servico externo.
func handleHotspotFingerprint(w http.ResponseWriter, r *http.Request) {
	mac := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mac")))
	w.Header().Set("Content-Type", "application/json")
	if mac == "" {
		http.Error(w, "parametro 'mac' obrigatorio", http.StatusBadRequest)
		return
	}

	containerID, err := compose.ServiceContainerID("hotspot")
	if err != nil || containerID == "" {
		_ = json.NewEncoder(w).Encode(hotspotFingerprint{})
		return
	}

	output, _ := exec.Command("docker", "exec", containerID, "tail", "-n", "1000", dnsmasqDHCPLog).CombinedOutput()
	_ = json.NewEncoder(w).Encode(parseDHCPFingerprint(string(output), mac))
}

// parseDHCPFingerprint varre o log em ordem e guarda o ultimo bloco de
// transacao DHCP visto para o MAC pedido - dnsmasq escreve algo como:
//
//	dnsmasq-dhcp: DHCPDISCOVER(ap0) 62:f6:93:1f:6a:2f
//	dnsmasq-dhcp: requested options: 1:netmask, 3:router, 6:dns-server
//	dnsmasq-dhcp: vendor class: android-dhcp-14
//
// so as linhas entre uma transacao (DHCPDISCOVER/DHCPREQUEST/...) e a
// proxima pertencem a ela; por isso zera o estado a cada nova
// transacao para nao misturar dados de dispositivos diferentes.
func parseDHCPFingerprint(log, mac string) hotspotFingerprint {
	var result hotspotFingerprint
	matching := false

	for _, line := range strings.Split(log, "\n") {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "dhcpdiscover") || strings.Contains(lower, "dhcprequest") || strings.Contains(lower, "dhcpack"):
			matching = strings.Contains(lower, mac)
		case matching && strings.Contains(lower, "requested options:"):
			if _, rest, ok := strings.Cut(line, "requested options:"); ok {
				result.DHCPFingerprint = dhcpOptionNumbers(rest)
			}
		case matching && strings.Contains(lower, "vendor class:"):
			if _, rest, ok := strings.Cut(line, "vendor class:"); ok {
				result.DHCPVendor = strings.TrimSpace(rest)
			}
		}
	}
	return result
}

// dhcpOptionNumbers converte "1:netmask, 3:router, 6:dns-server" em
// "1,3,6" para preservar a ordem do fingerprint DHCP.
func dhcpOptionNumbers(raw string) string {
	var numbers []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if number, _, ok := strings.Cut(part, ":"); ok && number != "" {
			numbers = append(numbers, number)
		}
	}
	return strings.Join(numbers, ",")
}
