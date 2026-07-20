// nginxui.go expoe, so leitura, o segredo de instalacao que o nginx-ui
// gera no primeiro boot (arquivo oculto no volume nginx_ui_data) - sem
// isso, o usuario precisa ler o arquivo direto no volume Docker para
// destravar a tela de configuracao inicial do nginx-ui. Some sozinho
// do painel assim que o arquivo deixar de existir (apos o nginx-ui
// concluir a propria configuracao inicial e remover o segredo).
package cert

import (
	"bindnet/backend/internal/auth"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

const nginxUIInstallSecretPath = "/nginx-ui-data/.install_secret"

type nginxUIInstallSecretResponse struct {
	Secret *string `json:"secret"`
}

func RegisterNginxUIRoutes(mux *http.ServeMux, admin *auth.Administrator) {
	mux.HandleFunc("GET /api/nginx-ui/install-secret", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, err := os.ReadFile(nginxUIInstallSecretPath)
		if errors.Is(err, os.ErrNotExist) {
			_ = json.NewEncoder(w).Encode(nginxUIInstallSecretResponse{})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		secret := strings.TrimSpace(string(data))
		_ = json.NewEncoder(w).Encode(nginxUIInstallSecretResponse{Secret: &secret})
	}))
}
