package shaping

import (
	"bindnet/worker/internal/network"
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// hotspotFilterChain e o chain de filter/FORWARD criado e populado
// pelo proprio hotspot (services/worker/hotspot/interfaces.sh,
// UPLINK_FILTER_CHAIN) com duas regras ACCEPT genericas (subnet do
// hotspot -> uplink, e RELATED,ESTABLISHED de volta). As regras de
// bloqueio por falta de credito sao inseridas NA FRENTE dessas duas
// (sempre -I ... 1) - senao a ACCEPT generica libera o pacote antes do
// nosso DROP ser avaliado, encerrando a travessia do chain.
const hotspotFilterChain = "BINDNET-HOTSPOT"

type hotspotTrafficBlockRequest struct {
	Interface string `json:"interface"`
	MAC       string `json:"mac"`
	IP        string `json:"ip,omitempty"`
}

// RegisterTrafficBlockRoutes expoe o bloqueio de trafego por falta de
// credito - diferente de /hotspot/block (hostapd deny_acl+deauth, usado
// so pelo bloqueio manual do admin), aqui o dispositivo continua
// associado ao Wi-Fi, so o trafego para de passar (DROP via iptables).
func RegisterTrafficBlockRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/trafficblock", handleTrafficBlock)
	mux.HandleFunc("POST /hotspot/trafficunblock", handleTrafficUnblock)
}

func handleTrafficBlock(w http.ResponseWriter, r *http.Request) {
	handleTrafficBlockAction(w, r, true)
}

func handleTrafficUnblock(w http.ResponseWriter, r *http.Request) {
	handleTrafficBlockAction(w, r, false)
}

func handleTrafficBlockAction(w http.ResponseWriter, r *http.Request, block bool) {
	var req hotspotTrafficBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return
	}
	mac, err := network.NormalizeMAC(req.MAC)
	if err != nil {
		http.Error(w, "mac invalido", http.StatusBadRequest)
		return
	}
	req.Interface = strings.TrimSpace(req.Interface)
	if req.Interface == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}

	if !block {
		removeDeviceTrafficBlock(mac)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	apIface, uplinkIface, err := resolveShapingInterfaces(req.Interface)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := ensureDeviceTrafficBlock(apIface, uplinkIface, mac, strings.TrimSpace(req.IP)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ensureDeviceTrafficBlock insere DROP de upload (por MAC, estavel) no
// topo do hotspotFilterChain, e de download (por IP, best-effort - so
// se ja soubermos o IP atual) - dispositivo continua associado ao
// Wi-Fi (sem deauth/deny_acl), so o trafego para de passar.
func ensureDeviceTrafficBlock(apIface, uplinkIface, mac, ip string) error {
	upComment := "bn-creditblock-up-" + mac
	if iptablesCheck(
		"-C", hotspotFilterChain,
		"-i", apIface, "-o", uplinkIface, "-m", "mac", "--mac-source", mac,
		"-m", "comment", "--comment", upComment, "-j", "DROP",
	) != nil {
		if err := runIptables(
			"-I", hotspotFilterChain, "1",
			"-i", apIface, "-o", uplinkIface, "-m", "mac", "--mac-source", mac,
			"-m", "comment", "--comment", upComment, "-j", "DROP",
		); err != nil {
			return err
		}
	}
	return refreshDeviceTrafficBlockIP(apIface, uplinkIface, mac, ip)
}

// refreshDeviceTrafficBlockIP e um no-op quando a regra de download ja
// casa com o IP atual (mesma ideia de refreshDeviceIP para o shaping) -
// so apaga e recria quando o IP mudou (renovacao de DHCP) ou ainda nao
// existia (dispositivo bloqueado antes de reconectar).
func refreshDeviceTrafficBlockIP(apIface, uplinkIface, mac, ip string) error {
	downComment := "bn-creditblock-down-" + mac
	if net.ParseIP(ip) == nil {
		return nil
	}
	if iptablesCheck(
		"-C", hotspotFilterChain,
		"-i", uplinkIface, "-o", apIface, "-d", ip,
		"-m", "comment", "--comment", downComment, "-j", "DROP",
	) == nil {
		return nil
	}
	deleteRulesByComment("", hotspotFilterChain, downComment)
	return runIptables(
		"-I", hotspotFilterChain, "1",
		"-i", uplinkIface, "-o", apIface, "-d", ip,
		"-m", "comment", "--comment", downComment, "-j", "DROP",
	)
}

func removeDeviceTrafficBlock(mac string) {
	deleteRulesByComment("", hotspotFilterChain, "bn-creditblock-up-"+mac)
	deleteRulesByComment("", hotspotFilterChain, "bn-creditblock-down-"+mac)
}
