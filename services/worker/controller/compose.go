package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const composeProjectName = "bindnet"

func registerComposeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/apply", handleHotspotServiceAction("restart"))
	mux.HandleFunc("POST /hotspot/start", handleHotspotServiceAction("start"))
	mux.HandleFunc("POST /hotspot/stop", handleHotspotServiceAction("stop"))
	mux.HandleFunc("GET /hotspot/status", handleHotspotServiceStatus)
	mux.HandleFunc("POST /dns/apply", handleApplyServices([]string{"dns-provider"}))
}

func handleHotspotServiceAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		_ = json.NewDecoder(r.Body).Decode(&config)

		if action != "stop" {
			if err := ensureHotspotContainer(); err != nil {
				log.Printf("[worker] erro ao garantir container do hotspot: %v", err)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			if err := applyComposeServices([]string{"dns-provider"}); err != nil {
				log.Printf("[worker] erro ao reiniciar dns-provider: %v", err)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			// Checa a associacao Wi-Fi cliente o mais tarde possivel,
			// imediatamente antes do "docker exec ... start" abaixo -
			// minimiza a janela entre esta checagem e a checagem
			// equivalente que try_create_ap (entrypoint.sh) faz sozinho
			// alguns segundos depois. Um round-trip a mais aqui (ex.:
			// checar a partir do backend, bem antes do /hotspot/apply e
			// do restart do dns-provider) da tempo de sobra pra uma
			// associacao Wi-Fi marginal cair entre as duas checagens e
			// as duas discordarem - foi exatamente essa janela que expos
			// o bug do NetworkManager brigando pela placa com o hostapd
			// (ver unmanageWifiInterface em network.go).
			unmanageWifiInterfaceIfIdle(config["WIFI_INTERFACE"], config["INTERNET_INTERFACE"])
		}

		output, err := execHotspotEntrypoint(action)
		if err != nil {
			log.Printf("[worker] erro ao executar hotspot %s: %v (%s)", action, err, output)
			http.Error(w, strings.TrimSpace(string(output)), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// unmanageWifiInterfaceIfIdle desgerencia a placa Wi-Fi fisica no
// NetworkManager antes do hotspot subir. Nunca desgerencia/desconecta
// quando WIFI_INTERFACE e INTERNET_INTERFACE sao a MESMA placa (Wi-Fi
// para Wi-Fi de verdade, AP+STA concorrente) - incondicional, sem
// checar se ela esta associada agora: o NetworkManager e o unico que
// mantem essa associacao viva nesse modo (o create_ap so cria a
// interface virtual ap0 ao lado dela, nunca assume a fisica sozinho).
// Checar "esta associada agora?" antes de decidir e uma corrida real -
// confirmado ao vivo que pegar a placa momentaneamente sem associacao
// (ex.: NM ainda no meio da propria reconexao apos um restart) fazia
// desgerenciar/desconectar mesmo assim, travando a placa desconectada
// PARA SEMPRE dali em diante: com "managed no", nada mais tenta
// reconecta-la (o NetworkManager e a unica coisa que faria isso, e
// esta desligado), entao toda tentativa seguinte do hotspot encontra a
// mesma placa sem associacao, indefinidamente, exigindo reconexao
// manual pra sair desse estado. Qualquer outra combinacao (ex.:
// internet via Ethernet) nao tem motivo pra preservar essa associacao,
// mesmo que a placa esteja transitoriamente conectada a alguma rede
// Wi-Fi do usuario sem relacao com o hotspot: deixa-la gerenciada
// nesse caso so faz o NetworkManager continuar escaneando/tentando
// (re)associar essa mesma placa enquanto o hostapd tenta manter o AP
// nela, derrubando o beacon ("Failed to set beacon parameters"/"key
// not allowed", ver Dockerfile). Falha/ausencia de iface nunca bloqueia
// o start.
func unmanageWifiInterfaceIfIdle(wifiInterface, internetInterface string) {
	if wifiInterface == "" {
		return
	}
	if wifiInterface == internetInterface {
		return
	}
	if err := unmanageWifiInterface(wifiInterface); err != nil {
		log.Printf("[worker] aviso: falha ao desgerenciar %s no NetworkManager: %v", wifiInterface, err)
	}
}

func handleHotspotServiceStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	containerID, running, err := serviceContainerRunning("hotspot")
	if err != nil || containerID == "" || !running {
		_ = json.NewEncoder(w).Encode(containerStatus{Name: "hotspot", Running: false, Status: "stopped"})
		return
	}
	output, err := exec.Command("docker", "exec", containerID, "/usr/local/bin/hotspot-entrypoint.sh", "status").CombinedOutput()
	if err != nil {
		log.Printf("[worker] erro ao ler status do hotspot: %v (%s)", err, output)
		_ = json.NewEncoder(w).Encode(containerStatus{Name: "hotspot", Running: false, Status: "unknown"})
		return
	}
	_, _ = w.Write(output)
}

func ensureHotspotContainer() error {
	output, err := exec.Command("docker", composeArgs("up", "-d", "--no-build", "--no-deps", "hotspot")...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	for i := 0; i < 20; i++ {
		_, running, err := serviceContainerRunning("hotspot")
		if err == nil && running {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("container hotspot nao ficou em execucao")
}

func execHotspotEntrypoint(action string) ([]byte, error) {
	containerID, running, err := serviceContainerRunning("hotspot")
	if err != nil {
		return nil, err
	}
	if containerID == "" || !running {
		if action == "stop" {
			return nil, nil
		}
		return nil, fmt.Errorf("container hotspot nao esta em execucao")
	}
	return exec.Command("docker", "exec", containerID, "/usr/local/bin/hotspot-entrypoint.sh", action).CombinedOutput()
}

func serviceContainerRunning(service string) (string, bool, error) {
	containerID, err := composeServiceContainerID(service)
	if err != nil || containerID == "" {
		return "", false, err
	}
	output, err := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerID).CombinedOutput()
	if err != nil {
		return containerID, false, fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return containerID, strings.TrimSpace(string(output)) == "true", nil
}

func applyComposeServices(services []string) error {
	args := composeArgs(append([]string{"up", "-d", "--no-build", "--no-deps"}, services...)...)
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// handleApplyServices recria os containers informados via "docker
// compose up". Os servicos leem a configuracao operacional diretamente
// do banco quando iniciam; o worker nao transporta WIFI_/HOTSPOT_ por env.
func handleApplyServices(services []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := applyComposeServices(services); err != nil {
			log.Printf("[worker] erro ao aplicar config (%v): %v", services, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func composeArgs(args ...string) []string {
	return composeArgsWithFiles(nil, args...)
}

func composeArgsWithFiles(extraFiles []string, args ...string) []string {
	base := []string{
		"compose",
		"--project-name", composeProjectName,
		"--project-directory", "/workspace",
		"--env-file", envPath(),
		"-f", "/workspace/docker-compose.services.yml",
	}
	for _, file := range extraFiles {
		base = append(base, "-f", file)
	}
	return append(base, args...)
}

func composeServiceContainerID(service string) (string, error) {
	output, err := exec.Command("docker", composeArgs("ps", "--all", "-q", service)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			return line, nil
		}
	}
	return "", nil
}
