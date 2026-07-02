package main

import (
	"encoding/json"
	"net/http"
)

var monitoredServices = []struct {
	Key     string
	Service string
}{
	{"hotspot", "hotspot"},
	{"dns", "dns-provider"},
	{"nginxUi", "nginx-ui"},
	{"postgres", "postgres"},
	{"mongo", "mongo"},
	{"minio", "minio"},
}

type serviceStatus struct {
	Key       string `json:"key"`
	Running   bool   `json:"running"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt,omitempty"`
}

// registerDashboardRoutes agrega o status dos servicos monitorados do
// stack numa unica chamada, usada pela tela inicial do painel.
func registerDashboardRoutes(mux *http.ServeMux, worker *workerClient, admin *administrator) {
	mux.HandleFunc("GET /api/dashboard", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		result := make([]serviceStatus, 0, len(monitoredServices))
		for _, item := range monitoredServices {
			var status struct {
				Running   bool   `json:"rodando"`
				Status    string `json:"status"`
				StartedAt string `json:"iniciadoEm"`
			}
			if err := worker.call(r.Context(), http.MethodGet, "/containers/"+item.Service+"/status", nil, &status); err != nil {
				result = append(result, serviceStatus{Key: item.Key, Status: "indisponivel"})
				continue
			}
			result = append(result, serviceStatus{
				Key:       item.Key,
				Running:   status.Running,
				Status:    status.Status,
				StartedAt: status.StartedAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
}
