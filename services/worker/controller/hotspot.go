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

	containerID, err := composeServiceContainerID("hotspot")
	if err != nil || containerID == "" {
		log.Printf("[worker] erro ao localizar container do hotspot: %v", err)
		_ = json.NewEncoder(w).Encode([]hotspotClient{})
		return
	}

	realIface := resolveRunningIface(containerID, iface)
	output, err := exec.Command("docker", "exec", containerID, "create_ap", "--list-clients", realIface).CombinedOutput()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Printf("[worker] erro ao listar clientes do hotspot: %v (%s)", err, output)
		_ = json.NewEncoder(w).Encode([]hotspotClient{})
		return
	}

	// Best-effort: sinal e so um extra informativo, driver sem suporte
	// ou erro no "iw" nunca deve derrubar a listagem de clientes em si
	// (ver hotspotClientSignal em hotspot_signal.go).
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

// resolveRunningIface traduz WIFI_INTERFACE (ex.: "wlp0s20f3") para a
// interface virtual que o create_ap realmente usa quando sobe em modo
// AP+estacao concorrente (ex.: "ap0") - list_clients() no create_ap so
// aceita a interface que ele mesmo esta rastreando; passar a fisica
// direto falha com "not used from create_ap instance" mesmo com
// clientes conectados de verdade. "create_ap --list-running" imprime
// "<pid> <iface-original> (<iface-real>)" quando os dois nomes
// divergem, ou so "<pid> <iface>" quando sao iguais (modo --no-virt) -
// nesse segundo caso ou se nada for encontrado, devolve a interface
// original sem alteracao.
func resolveRunningIface(containerID, iface string) string {
	output, err := exec.Command("docker", "exec", containerID, "create_ap", "--list-running").CombinedOutput()
	if err != nil {
		return iface
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[1] != iface {
			continue
		}
		if len(fields) >= 3 {
			return strings.Trim(fields[2], "()")
		}
		return iface
	}
	return iface
}
