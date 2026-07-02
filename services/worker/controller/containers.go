package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

// allowedContainers e a lista fechada de containers que o worker aceita
// controlar por servico do Compose. Qualquer outro nome (inclusive vindo
// de um path malicioso) e rejeitado - o worker nunca executa "exec
// arbitrario".
var allowedContainers = map[string]bool{
	"hotspot":      true,
	"dns-provider": true,
	"nginx-ui":     true,
	"postgres":     true,
	"mongo":        true,
	"minio":        true,
}

func registerContainerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /containers/{name}/start", handleContainerAction("start"))
	mux.HandleFunc("POST /containers/{name}/stop", handleContainerAction("stop"))
	mux.HandleFunc("POST /containers/{name}/restart", handleContainerAction("restart"))
	mux.HandleFunc("GET /containers/{name}/status", handleContainerStatus)
	mux.HandleFunc("GET /containers/{name}/logs", handleContainerLogs)
}

func handleContainerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		service := r.PathValue("name")
		if !allowedContainers[service] {
			http.Error(w, "servico nao permitido", http.StatusForbidden)
			return
		}
		output, err := exec.Command("docker", composeArgs(action, service)...).CombinedOutput()
		if err != nil {
			log.Printf("[worker] erro ao executar docker compose %s %s: %v (%s)", action, service, err, output)
			http.Error(w, string(output), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type containerStatus struct {
	Name      string `json:"name"`
	Running   bool   `json:"running"`
	Status    string `json:"status"`
	Image     string `json:"image"`
	StartedAt string `json:"startedAt,omitempty"`
}

func handleContainerStatus(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("name")
	w.Header().Set("Content-Type", "application/json")
	if !allowedContainers[service] {
		http.Error(w, "servico nao permitido", http.StatusForbidden)
		return
	}

	name, err := composeServiceContainerID(service)
	if err != nil || name == "" {
		_ = json.NewEncoder(w).Encode(containerStatus{Name: service, Running: false, Status: "ausente"})
		return
	}

	format := "{{.State.Running}}|{{.State.Status}}|{{.Config.Image}}|{{.State.StartedAt}}"
	output, err := exec.Command("docker", "inspect", "--format", format, name).CombinedOutput()
	if err != nil {
		_ = json.NewEncoder(w).Encode(containerStatus{Name: service, Running: false, Status: "ausente"})
		return
	}

	parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 4)
	if len(parts) != 4 {
		http.Error(w, "saida inesperada do docker inspect", http.StatusInternalServerError)
		return
	}
	running, _ := strconv.ParseBool(parts[0])
	response := containerStatus{Name: service, Running: running, Status: parts[1], Image: parts[2]}
	if running {
		response.StartedAt = parts[3]
	}
	_ = json.NewEncoder(w).Encode(response)
}

// flushWriter repassa cada escrita imediatamente ao cliente, necessario
// para transmitir "docker logs -f" em tempo real em vez de bufferizar.
type flushWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.flusher != nil {
		fw.flusher.Flush()
	}
	return n, err
}

func handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("name")
	if !allowedContainers[service] {
		http.Error(w, "servico nao permitido", http.StatusForbidden)
		return
	}

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}
	args := composeArgs("logs", "--tail", tail)
	if r.URL.Query().Get("follow") == "true" {
		args = append(args, "-f")
	}
	args = append(args, service)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	flusher, _ := w.(http.Flusher)

	cmd := exec.CommandContext(r.Context(), "docker", args...)
	cmd.Stdout = flushWriter{w, flusher}
	cmd.Stderr = flushWriter{w, flusher}
	if err := cmd.Run(); err != nil {
		log.Printf("[worker] docker compose logs %s encerrou: %v", service, err)
	}
}
