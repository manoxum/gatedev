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

// nmDropin e o arquivo de configuracao que marca a interface Wi-Fi
// como nao-gerenciada pelo NetworkManager, para o hostapd assumir o
// controle dela durante o hotspot.
const nmDropin = "/etc/NetworkManager/conf.d/90-bindnet-hotspot-unmanaged.conf"

func registerNetworkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /network/interfaces", handleInterfaces)
	mux.HandleFunc("POST /network/wifi-unmanage", handleWifiUnmanage)
	mux.HandleFunc("POST /network/wifi-manage", handleWifiManage)
	mux.HandleFunc("POST /network/dns-test", handleDNSTest)
}

type interfaceInfo struct {
	Name  string `json:"name"`
	Type  string `json:"type"` // "wifi" | "other"
	State string `json:"state"`
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
		if name == "lo" {
			continue
		}
		ifaceType := "other"
		if wifi[name] {
			ifaceType = "wifi"
		}
		state := "down"
		if strings.Contains(line, "UP") {
			state = "up"
		}
		interfaces = append(interfaces, interfaceInfo{Name: name, Type: ifaceType, State: state})
	}

	_ = json.NewEncoder(w).Encode(interfaces)
}

type interfaceRequest struct {
	Interface string `json:"interface"`
}

// handleWifiUnmanage marca a interface fisica (e a virtual "ap0" que o
// create_ap cria) como nao-gerenciada pelo NetworkManager, para o
// hostapd poder assumir o controle dela.
func handleWifiUnmanage(w http.ResponseWriter, r *http.Request) {
	var req interfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Interface == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}

	content := fmt.Sprintf("[keyfile]\nunmanaged-devices=interface-name:%s;interface-name:ap0\n", req.Interface)
	if err := os.WriteFile(nmDropin, []byte(content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if output, err := exec.Command("nmcli", "general", "reload", "conf").CombinedOutput(); err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWifiManage remove o drop-in e devolve a interface ao controle
// normal do NetworkManager.
func handleWifiManage(w http.ResponseWriter, r *http.Request) {
	var req interfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Interface == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}

	_ = os.Remove(nmDropin)
	if output, err := exec.Command("nmcli", "general", "reload", "conf").CombinedOutput(); err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}
	output, err := exec.Command("nmcli", "device", "set", req.Interface, "managed", "yes").CombinedOutput()
	if err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
