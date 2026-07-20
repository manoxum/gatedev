package setup

import (
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/workerapi"
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

// RegisterDashboardRoutes agrega o status dos servicos monitorados do
// stack numa unica chamada, usada pela tela inicial do painel.
func RegisterDashboardRoutes(mux *http.ServeMux, worker *workerapi.Client, admin *auth.Administrator) {
	mux.HandleFunc("GET /api/dashboard", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		result := make([]serviceStatus, 0, len(monitoredServices))
		for _, item := range monitoredServices {
			var status struct {
				Running   bool   `json:"rodando"`
				Status    string `json:"status"`
				StartedAt string `json:"iniciadoEm"`
			}
			if err := worker.Call(r.Context(), http.MethodGet, "/containers/"+item.Service+"/status", nil, &status); err != nil {
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
