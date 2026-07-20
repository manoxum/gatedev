// Package hotspot expoe as operacoes do ponto de acesso Wi-Fi no worker:
// ciclo de vida do servico (service.go), listagem de clientes, ACL por MAC
// (hostapd deny_acl) e fingerprint DHCP. Delega orquestracao Docker ao
// pacote compose e operacoes de NetworkManager ao pacote network.
package hotspot

import (
	"bufio"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"bindnet/worker/internal/compose"
)

func RegisterClientRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /hotspot/clients", handleHotspotClients)
	mux.HandleFunc("POST /hotspot/block", handleHotspotBlock)
	mux.HandleFunc("POST /hotspot/unblock", handleHotspotUnblock)
}

type hotspotClient struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	SignalDBM *int   `json:"signalDbm,omitempty"`
}

// handleHotspotClients usa o comando nativo "create_ap --list-clients"
// (ver print_client/list_clients em /usr/local/bin/create_ap) em vez de
// ler o dnsmasq.leases diretamente, pois o caminho desse arquivo tem um
// sufixo temporario aleatorio (mktemp) que muda a cada execucao.
func handleHotspotClients(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("interface")
	if iface == "" {
		http.Error(w, "parametro 'interface' obrigatorio", http.StatusBadRequest)
		return
	}

	containerID, err := compose.ServiceContainerID("hotspot")
	if err != nil || containerID == "" {
		log.Printf("[worker] erro ao localizar container do hotspot: %v", err)
		_ = json.NewEncoder(w).Encode([]hotspotClient{})
		return
	}

	realIface := compose.ResolveRunningIface(containerID, iface)
	output, err := exec.Command("docker", "exec", containerID, "create_ap", "--list-clients", realIface).CombinedOutput()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Printf("[worker] erro ao listar clientes do hotspot: %v (%s)", err, output)
		_ = json.NewEncoder(w).Encode([]hotspotClient{})
		return
	}

	// Best-effort: sinal e so um extra informativo, driver sem suporte ou
	// erro no "iw" nunca deve derrubar a listagem de clientes em si (ver
	// hotspotClientSignal em signal.go).
	signals := hotspotClientSignal(containerID, realIface)

	var clients []hotspotClient
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || !strings.Contains(fields[0], ":") {
			// pula cabecalho ("MAC IP Hostname") e a linha
			// "No clients connected".
			continue
		}
		client := hotspotClient{MAC: fields[0], IP: fields[1], Hostname: fields[2]}
		if dbm, found := signals[strings.ToLower(fields[0])]; found {
			client.SignalDBM = &dbm
		}
		clients = append(clients, client)
	}
	_ = json.NewEncoder(w).Encode(clients)
}
