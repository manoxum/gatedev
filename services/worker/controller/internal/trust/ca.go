package trust

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const localCAInstallPath = "/usr/local/share/ca-certificates/bindnet-local-ca.crt"

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /ca/install-local", handleInstallLocalCA)
}

type installLocalCARequest struct {
	CertificatePEM string `json:"certificatePem"`
}

type installLocalCAResponse struct {
	Path          string               `json:"path"`
	Output        string               `json:"output,omitempty"`
	BrowserStores []browserTrustResult `json:"browserStores,omitempty"`
}

func handleInstallLocalCA(w http.ResponseWriter, r *http.Request) {
	var req installLocalCARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.CertificatePEM) == "" {
		http.Error(w, "campo 'certificatePem' obrigatorio", http.StatusBadRequest)
		return
	}
	if err := validateCAPEM(req.CertificatePEM); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := exec.LookPath("update-ca-certificates"); err != nil {
		http.Error(w, "update-ca-certificates nao esta disponivel no worker", http.StatusFailedDependency)
		return
	}

	if err := os.MkdirAll(filepath.Dir(localCAInstallPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(localCAInstallPath, []byte(strings.TrimSpace(req.CertificatePEM)+"\n"), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	output, err := exec.CommandContext(r.Context(), "update-ca-certificates").CombinedOutput()
	if err != nil {
		http.Error(w, strings.TrimSpace(string(output)), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(installLocalCAResponse{
		Path:          localCAInstallPath,
		Output:        strings.TrimSpace(string(output)),
		BrowserStores: importCAIntoBrowserStores(req.CertificatePEM),
	})
}

func validateCAPEM(certificatePEM string) error {
	block, _ := pem.Decode([]byte(certificatePEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("certificatePem deve conter um certificado PEM valido")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return errors.New("certificatePem deve conter um certificado X.509 valido")
	}
	if !cert.IsCA {
		return errors.New("certificatePem deve ser uma CA")
	}
	return nil
}
