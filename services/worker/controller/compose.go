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
