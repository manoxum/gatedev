// certificates_routes.go expoe via HTTP a gestao de certificados
// (certificates.go/ca.go). Substitui o antigo registrarRotasCertProxy
// (so leitura). Quase todas as rotas exigem sessao - excecao
// deliberada: GET /api/mesh/ca, que devolve so o certificado publico
// da CA (nunca a chave privada) sem autenticacao, para que outros nos
// da malha Bindnet consigam buscar essa CA e o usuario decidir se
// confia nela, igual ao antigo cert-proxy (que servia /ca.crt
// anonimamente na porta 80) - so que agora escopado a essa unica rota.
package cert

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/workerapi"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

// issueCertificateRequest.Domains vira um unico certificado com todos
// os dominios/IPs como SAN - ver comentario de issueCertificate em
// certificates.go. Aceita dominio curinga (ex.: "*.mydomain") em
// qualquer posicao da lista. ValidityQuantity/ValidityUnit sao
// opcionais - vazios/invalidos caem para o padrao de 2 anos.
type issueCertificateRequest struct {
	Domains          []string `json:"domains"`
	ValidityQuantity int      `json:"validityQuantity,omitempty"`
	ValidityUnit     string   `json:"validityUnit,omitempty"`
}

type installLocalCARequest struct {
	CertificatePEM string `json:"certificatePem,omitempty"`
}

type installLocalCAWorkerRequest struct {
	CertificatePEM string `json:"certificatePem"`
}

type browserTrustResult struct {
	Store     string `json:"store"`
	Path      string `json:"path"`
	Installed bool   `json:"installed"`
	Error     string `json:"error,omitempty"`
}

type installLocalCAResponse struct {
	Path          string               `json:"path"`
	Output        string               `json:"output,omitempty"`
	BrowserStores []browserTrustResult `json:"browserStores,omitempty"`
}

func RegisterCertificateRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, ca *localCA, worker *workerapi.Client, audit *audit.Client) {
	mux.HandleFunc("GET /api/certificates", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		certificates, err := listCertificates(db, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(certificates)
	}))

	mux.HandleFunc("GET /api/certificates/revoked", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		certificates, err := listCertificates(db, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(certificates)
	}))

	mux.HandleFunc("POST /api/certificates", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req issueCertificateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !hasNonEmptyDomain(req.Domains) {
			http.Error(w, "campo 'domains' obrigatorio (ao menos um dominio ou IP)", http.StatusBadRequest)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		cert, err := issueCertificate(db, ca, req.Domains, req.ValidityQuantity, req.ValidityUnit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		audit.Record(r.Context(), "certificate_issued", username, map[string]any{
			"id": cert.ID, "domain": cert.Domain, "domains": append(cert.DNSNames, cert.IPAddresses...),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(cert)
	}))

	mux.HandleFunc("DELETE /api/certificates/{id}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		username, _ := auth.SessionUser(r, admin)
		domain, err := revokeCertificate(db, id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "certificado nao encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := removeCertificateFromNginxUI(domain); err != nil {
			http.Error(w, "certificado revogado no Bindnet, mas falhou ao remover do nginx-ui: "+err.Error(), http.StatusInternalServerError)
			return
		}
		audit.Record(r.Context(), "certificate_revoked", username, map[string]any{"id": id, "domain": domain})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("DELETE /api/certificates/{id}/permanent", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		username, _ := auth.SessionUser(r, admin)
		domain, err := revokedCertificateDomainByID(db, id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "certificado nao encontrado", http.StatusNotFound)
			return
		}
		if errors.Is(err, errCertificateNotRevoked) {
			http.Error(w, "apenas certificados revogados podem ser eliminados permanentemente", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := removeCertificateFromNginxUI(domain); err != nil {
			http.Error(w, "falhou ao limpar certificado no nginx-ui: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := permanentlyDeleteCertificate(db, id); errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "certificado revogado nao encontrado", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		audit.Record(r.Context(), "certificate_permanently_deleted", username, map[string]any{"id": id, "domain": domain})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/certificates/{id}/download", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		domain, certificatePEM, err := certificatePEMByID(db, r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "certificado nao encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		serveCertificate(w, certificatePEM, domain+".crt")
	}))

	mux.HandleFunc("GET /api/certificates/ca", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		serveCertificate(w, ca.CertificatePEM, "bindnet-local-ca.crt")
	}))

	// GET /api/mesh/ca e a unica rota deste arquivo sem auth.RequireSession -
	// ver comentario de pacote no topo do arquivo.
	mux.HandleFunc("GET /api/mesh/ca", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		serveCertificate(w, ca.CertificatePEM, "bindnet-local-ca.crt")
	})

	mux.HandleFunc("POST /api/certificates/ca/install-local", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		username, _ := auth.SessionUser(r, admin)
		var req installLocalCARequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		certificatePEM := ca.CertificatePEM
		if req.CertificatePEM != "" {
			certificatePEM = req.CertificatePEM
		}
		var response installLocalCAResponse
		if err := worker.Call(r.Context(), http.MethodPost, "/ca/install-local", installLocalCAWorkerRequest{
			CertificatePEM: certificatePEM,
		}, &response); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		audit.Record(r.Context(), "ca_installed_local", username, map[string]any{"path": response.Path})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

func serveCertificate(w http.ResponseWriter, pemContent, filename string) {
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte(pemContent))
}
