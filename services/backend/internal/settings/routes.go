package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
)

type response struct {
	// CACommonName e o CN que sera usado se/quando uma CA nova for gerada.
	CACommonName string `json:"caCommonName"`
	// CACurrentCommonName e o CN da CA que existe hoje (somente leitura):
	// trocar CACommonName nao reemite nem renomeia essa CA.
	CACurrentCommonName string `json:"caCurrentCommonName"`
	CAGenerated         bool   `json:"caGenerated"`
	NginxUIUsername     string `json:"nginxUiUsername"`
	// NginxUIConfigured evita devolver a senha ao frontend - a tela so
	// precisa saber se ja existe uma configurada.
	NginxUIConfigured bool `json:"nginxUiConfigured"`
}

// request usa ponteiro em cada campo para distinguir "nao enviado" de
// "enviado vazio" - so assim da pra limpar a senha do nginx-ui de proposito
// sem que um PATCH parcial apague o que nao foi tocado.
type request struct {
	CACommonName    *string `json:"caCommonName"`
	NginxUIUsername *string `json:"nginxUiUsername"`
	NginxUIPassword *string `json:"nginxUiPassword"`
}

func RegisterRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, aud *audit.Client) {
	mux.HandleFunc("GET /api/settings", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		values, err := GetAll(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		currentCN, generated, err := currentCA(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		caCommonName := values[KeyCACommonName]
		if caCommonName == "" {
			caCommonName = DefaultCACommonName
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response{
			CACommonName:        caCommonName,
			CACurrentCommonName: currentCN,
			CAGenerated:         generated,
			NginxUIUsername:     values[KeyNginxUIUsername],
			NginxUIConfigured:   values[KeyNginxUIUsername] != "" && values[KeyNginxUIPassword] != "",
		})
	}))

	mux.HandleFunc("PATCH /api/settings", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}

		values := map[string]string{}
		if req.CACommonName != nil {
			values[KeyCACommonName] = *req.CACommonName
		}
		if req.NginxUIUsername != nil {
			values[KeyNginxUIUsername] = *req.NginxUIUsername
		}
		if req.NginxUIPassword != nil {
			values[KeyNginxUIPassword] = *req.NginxUIPassword
		}
		if len(values) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := Save(r.Context(), db, values); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		username, _ := auth.SessionUser(r, admin)
		aud.Record(r.Context(), "config_changed", username, map[string]any{"section": "settings"})
		w.WriteHeader(http.StatusNoContent)
	}))
}

// currentCA le o CN da CA ja persistida (tabela ca, escrita por
// internal/cert). Leitura direta de propósito: settings e a camada de
// baixo, entao nao pode importar cert (seria ciclo - cert importa settings
// para descobrir o CN a usar ao gerar a CA).
func currentCA(ctx context.Context, db *sql.DB) (commonName string, generated bool, err error) {
	err = db.QueryRowContext(ctx, `SELECT common_name FROM ca LIMIT 1`).Scan(&commonName)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return commonName, true, nil
}
