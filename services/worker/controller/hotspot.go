package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

func registerHotspotClientRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /hotspot/clients", handleHotspotClients)
}

type hotspotClient struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
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

	containerID, err := composeServiceContainerID("hotspot")
	if err != nil || containerID == "" {
		log.Printf("[worker] erro ao localizar container do hotspot: %v", err)
		_ = json.NewEncoder(w).Encode([]hotspotClient{})
		return
	}

	output, err := exec.Command("docker", "exec", containerID, "create_ap", "--list-clients", iface).CombinedOutput()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Printf("[worker] erro ao listar clientes do hotspot: %v (%s)", err, output)
		_ = json.NewEncoder(w).Encode([]hotspotClient{})
		return
	}

	var clients []hotspotClient
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || !strings.Contains(fields[0], ":") {
			// pula cabecalho ("MAC IP Hostname") e a linha
			// "No clients connected".
			continue
		}
		clients = append(clients, hotspotClient{MAC: fields[0], IP: fields[1], Hostname: fields[2]})
	}
	_ = json.NewEncoder(w).Encode(clients)
}
