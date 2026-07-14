package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func registerNetworkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /network/interfaces", handleInterfaces)
	mux.HandleFunc("POST /network/wifi-unmanage", handleWifiUnmanage)
	mux.HandleFunc("POST /network/wifi-manage", handleWifiManage)
	mux.HandleFunc("POST /network/dns-test", handleDNSTest)
	mux.HandleFunc("POST /network/peer-scan", handlePeerScan)
}

type interfaceInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // "wifi" | "other"
	State      string `json:"state"`
	SpeedMbps  int    `json:"speedMbps,omitempty"`
	Associated bool   `json:"associated,omitempty"` // so relevante para type=="wifi"
}

// virtualInterfacePrefixes cobre interfaces que o Docker/kernel criam
// sozinhos e que nunca sao uma saida de internet de verdade - "br-"
// (com hifen) e deliberado: so bate no padrao que o Docker gera pra
// redes customizadas (br-<12 hex>), nao numa bridge manual como "br0"
// que o usuario poderia escolher de proposito.
var virtualInterfacePrefixes = []string{"docker", "br-", "veth", "virbr", "tun", "tap", "wg", "bn-"}

// isVirtualInterface filtra interfaces que nao fazem sentido nos
// seletores de WIFI_INTERFACE/INTERNET_INTERFACE do painel: "lo"
// (loopback) e "ap0" (interface virtual que o create_ap cria) alem das
// interfaces geradas por Docker/VPN/bridge e do uplink dummy Bindnet.
func isVirtualInterface(name string) bool {
	if name == "lo" || name == "ap0" {
		return true
	}
	for _, prefix := range virtualInterfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// handleInterfaces lista as interfaces de rede reais do host - so
// funciona porque o worker roda com network_mode: host.
func handleInterfaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	output, err := exec.Command("ip", "-o", "link", "show").CombinedOutput()
	if err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}

	wifiOutput, _ := exec.Command("iw", "dev").CombinedOutput()
	wifi := map[string]bool{}
	for _, line := range strings.Split(string(wifiOutput), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "Interface" {
			wifi[fields[1]] = true
		}
	}

	var interfaces []interfaceInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 3)
		if len(parts) < 2 {
			continue
		}
		name := strings.SplitN(strings.TrimSpace(parts[1]), "@", 2)[0]
		if isVirtualInterface(name) {
			continue
		}
		ifaceType := "other"
		associated := false
		if wifi[name] {
			ifaceType = "wifi"
			associated = interfaceAssociated(name)
		}
		state := "down"
		if strings.Contains(line, "UP") {
			state = "up"
		}
		interfaces = append(interfaces, interfaceInfo{
			Name:       name,
			Type:       ifaceType,
			State:      state,
			SpeedMbps:  interfaceSpeedMbps(name),
			Associated: associated,
		})
	}

	_ = json.NewEncoder(w).Encode(interfaces)
}

// interfaceAssociated diz se a interface Wi-Fi esta associada como
// estacao (cliente) a uma rede agora - o mesmo teste que
// services/worker/hotspot/entrypoint.sh (try_create_ap) usa para
// decidir entre criar uma interface AP virtual (preservando essa
// associacao) ou usar --no-virt diretamente na interface fisica. O
// backend usa este campo para so desgerenciar a placa fisica no
// NetworkManager (POST /network/wifi-unmanage) quando ela NAO estiver
// associada - ver unmanagePhysicalInterfaceIfIdle em
// services/backend/hotspot_network.go.
func interfaceAssociated(name string) bool {
	output, err := exec.Command("iw", "dev", name, "link").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(output)), "Connected to")
}

func interfaceSpeedMbps(name string) int {
	data, err := os.ReadFile("/sys/class/net/" + name + "/speed")
	if err != nil {
		return 0
	}
	var speed int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &speed); err != nil || speed <= 0 {
		return 0
	}
	return speed
}

type dnsTestRequest struct {
	Hostname string `json:"hostname"`
	Server   string `json:"server"` // normalmente HOTSPOT_GATEWAY
}

type dnsTestResponse struct {
	Addresses []string `json:"addresses"`
	Error     string   `json:"error,omitempty"`
}

// handleDNSTest consulta o dns-provider diretamente pelo endereco onde
// ele escuta - so o worker (network_mode: host) alcanca esse endereco.
func handleDNSTest(w http.ResponseWriter, r *http.Request) {
	var req dnsTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hostname == "" || req.Server == "" {
		http.Error(w, "campos 'hostname' e 'server' obrigatorios", http.StatusBadRequest)
		return
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", net.JoinHostPort(req.Server, "53"))
		},
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	addresses, err := resolver.LookupHost(ctx, req.Hostname)
	response := dnsTestResponse{Addresses: addresses}
	if err != nil {
		response.Error = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
