package shaping

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"bindnet/worker/internal/network"
)

var portListPattern = regexp.MustCompile(`^\d{1,5}(-\d{1,5})?(,\d{1,5}(-\d{1,5})?)*$`)

func isValidIsolationProtocol(protocol string) bool {
	switch protocol {
	case "any", "tcp", "udp", "icmp":
		return true
	default:
		return false
	}
}

func isValidPortList(list string) bool {
	if !portListPattern.MatchString(list) {
		return false
	}
	for _, part := range strings.Split(list, ",") {
		for _, p := range strings.SplitN(part, "-", 2) {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 65535 {
				return false
			}
		}
	}
	return true
}

// RegisterIsolationRoutes expoe o isolamento de clientes do hotspot: o
// backend manda o estado desejado COMPLETO (pares MAC origem -> IP
// destino que podem iniciar trafego) e o worker materializa isso no
// chain BINDNET-ISOLATION (ver isolation_rules.go) - idempotente e sem
// estado local, mesma filosofia do shaping/bloqueio por credito.
func RegisterIsolationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/isolation/apply", handleIsolationApply)
}

type isolationPairPayload struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Protocol string `json:"protocol"`
	DstPorts string `json:"dstPorts"`
	Action   string `json:"action"`
}

type isolationApplyRequest struct {
	Interface string                 `json:"interface"`
	Enabled   bool                   `json:"enabled"`
	Pairs     []isolationPairPayload `json:"pairs"`
}

func handleIsolationApply(w http.ResponseWriter, r *http.Request) {
	var req isolationApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return
	}

	if !req.Enabled {
		teardownIsolation(strings.TrimSpace(req.Interface))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if strings.TrimSpace(req.Interface) == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}
	apIface, _, err := resolveShapingInterfaces(req.Interface)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	pairs := make([]isolationPairPayload, 0, len(req.Pairs))
	for _, pair := range req.Pairs {
		mac, err := network.NormalizeMAC(pair.MAC)
		if err != nil {
			http.Error(w, "mac invalido: "+pair.MAC, http.StatusBadRequest)
			return
		}
		if net.ParseIP(pair.IP) == nil {
			http.Error(w, "ip invalido: "+pair.IP, http.StatusBadRequest)
			return
		}
		if !isValidIsolationProtocol(pair.Protocol) {
			http.Error(w, "protocolo invalido: "+pair.Protocol, http.StatusBadRequest)
			return
		}
		if pair.DstPorts != "" && !isValidPortList(pair.DstPorts) {
			http.Error(w, "portas invalidas: "+pair.DstPorts, http.StatusBadRequest)
			return
		}
		if pair.Action != "allow" && pair.Action != "deny" {
			http.Error(w, "acao invalida: "+pair.Action, http.StatusBadRequest)
			return
		}
		pairs = append(pairs, isolationPairPayload{
			MAC: mac, IP: pair.IP, Protocol: pair.Protocol, DstPorts: pair.DstPorts, Action: pair.Action,
		})
	}

	if err := ensureIsolationSysctls(apIface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := syncIsolationChain(apIface, pairs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ensureIsolationSysctls prepara o hairpin L3 na interface AP: com o
// ap_isolate do hostapd cortando o L2 direto entre estacoes,
// proxy_arp_pvlan=1 (RFC 3069) faz o host responder ARP em nome dos
// outros clientes e rotear cliente->host->cliente pela MESMA interface
// - e so esse trafego roteado que atravessa FORWARD e o chain
// BINDNET-ISOLATION consegue filtrar. send_redirects=0 evita o kernel
// anunciar via ICMP redirect um atalho L2 que o ap_isolate bloqueia.
func ensureIsolationSysctls(apIface string) error {
	if err := writeIsolationSysctl(apIface, "proxy_arp_pvlan", "1"); err != nil {
		return err
	}
	return writeIsolationSysctl(apIface, "send_redirects", "0")
}

func revertIsolationSysctls(apIface string) {
	_ = writeIsolationSysctl(apIface, "proxy_arp_pvlan", "0")
	_ = writeIsolationSysctl(apIface, "send_redirects", "1")
}

func writeIsolationSysctl(iface, key, value string) error {
	if iface == "" || strings.ContainsAny(iface, "/.") {
		return fmt.Errorf("interface invalida para sysctl: %q", iface)
	}
	path := "/proc/sys/net/ipv4/conf/" + iface + "/" + key
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		return fmt.Errorf("falha ao escrever %s=%s: %w", path, value, err)
	}
	return nil
}
