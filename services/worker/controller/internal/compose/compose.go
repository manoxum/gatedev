// Package compose reune as operacoes de docker/docker compose do worker:
// controle de ciclo de vida dos containers do stack, execucao de comandos
// no container do hotspot e resolucao da interface AP real do create_ap.
// E a camada de orquestracao Docker, sem logica de dominio (hotspot/rede).
package compose

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const composeProjectName = "bindnet"

// ApplyServicesHandler recria os containers informados via "docker compose
// up". Os servicos leem a configuracao operacional diretamente do banco
// quando iniciam; o worker nao transporta WIFI_/HOTSPOT_ por env.
func ApplyServicesHandler(services []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ApplyServices(services); err != nil {
			log.Printf("[worker] erro ao aplicar config (%v): %v", services, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func EnsureHotspotContainer() error {
	output, err := exec.Command("docker", composeArgs("up", "-d", "--no-build", "--no-deps", "hotspot")...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	for i := 0; i < 20; i++ {
		_, running, err := ServiceContainerRunning("hotspot")
		if err == nil && running {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("container hotspot nao ficou em execucao")
}

func ExecHotspotEntrypoint(action string) ([]byte, error) {
	containerID, running, err := ServiceContainerRunning("hotspot")
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

func ServiceContainerRunning(service string) (string, bool, error) {
	containerID, err := ServiceContainerID(service)
	if err != nil || containerID == "" {
		return "", false, err
	}
	output, err := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerID).CombinedOutput()
	if err != nil {
		return containerID, false, fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return containerID, strings.TrimSpace(string(output)) == "true", nil
}

func ApplyServices(services []string) error {
	args := composeArgs(append([]string{"up", "-d", "--no-build", "--no-deps"}, services...)...)
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func composeArgs(args ...string) []string {
	return composeArgsWithFiles(nil, args...)
}

func composeArgsWithFiles(extraFiles []string, args ...string) []string {
	base := []string{
		"compose",
		"--project-name", composeProjectName,
		"--project-directory", "/workspace",
		"--env-file", EnvPath(),
		"-f", "/workspace/docker-compose.services.yml",
	}
	for _, file := range extraFiles {
		base = append(base, "-f", file)
	}
	return append(base, args...)
}

func ServiceContainerID(service string) (string, error) {
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

// ResolveRunningIface traduz WIFI_INTERFACE (ex.: "wlp0s20f3") para a
// interface virtual que o create_ap realmente usa quando sobe em modo
// AP+estacao concorrente (ex.: "ap0") - list_clients() no create_ap so
// aceita a interface que ele mesmo esta rastreando; passar a fisica direto
// falha com "not used from create_ap instance" mesmo com clientes
// conectados de verdade. "create_ap --list-running" imprime
// "<pid> <iface-original> (<iface-real>)" quando os dois nomes divergem, ou
// so "<pid> <iface>" quando sao iguais (modo --no-virt) - nesse segundo
// caso ou se nada for encontrado, devolve a interface original sem
// alteracao.
func ResolveRunningIface(containerID, iface string) string {
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
