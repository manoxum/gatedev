package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

const composeProjectName = "bindnet"

func registerComposeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/apply", handleApplyServices([]string{"hotspot", "dns-provider"}))
	mux.HandleFunc("POST /dns/apply", handleApplyServices([]string{"dns-provider"}))
}

// handleApplyServices recria os containers informados via "docker
// compose up", unica forma de fazer um container ja existente reler o
// env_file - "docker restart"/"docker start" reaproveita o ambiente com
// que o container foi criado originalmente, sem pegar valores novos do
// .env. --no-build evita reconstruir a imagem, so recria o container.
func handleApplyServices(services []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args := composeArgs(append([]string{"up", "-d", "--no-build"}, services...)...)
		output, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			log.Printf("[worker] erro ao aplicar config (%v): %v (%s)", services, err, output)
			http.Error(w, string(output), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func composeArgs(args ...string) []string {
	base := []string{"compose", "--project-name", composeProjectName, "--project-directory", "/workspace"}
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
