// nginxui_sync.go carrega automaticamente, via API HTTP do nginx-ui, um
// certificado recem-emitido pela CA local (certificates.go) - sem isso o
// usuario precisaria copiar/colar o PEM manualmente na tela de
// certificados do nginx-ui. O login da API do nginx-ui exige um
// handshake proprio (troca de chave publica RSA + payload criptografado
// com RSA-PKCS1v15, ver github.com/0xJacky/Nginx-UI internal/crypto e
// internal/middleware/encrypted_params.go) - replicado aqui porque nao
// ha outra forma documentada de autenticar contra ele por HTTP. A
// importacao local (sem credenciais da API) fica em
// nginxui_local_store.go.
package cert

import (
	"bindnet/backend/internal/platform/config"
	"bindnet/backend/internal/settings"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	nginxUIBackendDataPath        = "/nginx-ui-data"
	nginxUIBackendConfigPath      = "/nginx-config"
	nginxUIContainerNginxConfPath = "/etc/nginx"
)

type nginxUIPublicKeyResponse struct {
	PublicKey string `json:"public_key"`
}

type nginxUILoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Token   string `json:"token"`
}

// nginxUIConfigured indica se as credenciais da API do nginx-ui foram
// configuradas no painel (tabela panel_config) - o sync e um extra
// opcional, nao deve impedir a emissao do certificado (ja persistida no
// Postgres) se o operador nao configurou isso.
func nginxUIConfigured(db *sql.DB) bool {
	username, password := settings.NginxUICredentials(context.Background(), db)
	return username != "" && password != ""
}

func nginxUIBaseURL() string {
	return config.Getenv("NGINX_UI_URL", "http://nginx-ui:9000")
}

// nginxUILogin replica o fluxo de login criptografado do nginx-ui:
// 1) busca a chave publica RSA temporaria da instancia; 2) criptografa
// usuario/senha com ela; 3) troca isso por um token JWT.
func nginxUILogin(db *sql.DB) (string, error) {
	publicKey, err := fetchNginxUIPublicKey()
	if err != nil {
		return "", fmt.Errorf("chave publica do nginx-ui: %w", err)
	}

	username, password := settings.NginxUICredentials(context.Background(), db)
	credentials, err := json.Marshal(map[string]string{
		"name":     username,
		"password": password,
	})
	if err != nil {
		return "", err
	}
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, credentials)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(map[string]string{
		"encrypted_params": base64.StdEncoding.EncodeToString(encrypted),
	})
	if err != nil {
		return "", err
	}

	resp, err := http.Post(nginxUIBaseURL()+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var login nginxUILoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&login); err != nil {
		return "", err
	}
	if login.Token == "" {
		return "", fmt.Errorf("login no nginx-ui falhou: %s", login.Message)
	}
	return login.Token, nil
}

func fetchNginxUIPublicKey() (*rsa.PublicKey, error) {
	body, err := json.Marshal(map[string]any{
		"timestamp":   time.Now().Unix(),
		"fingerprint": "bindnet-backend",
	})
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(nginxUIBaseURL()+"/api/crypto/public_key", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed nginxUIPublicKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(parsed.PublicKey))
	if block == nil {
		return nil, errors.New("chave publica do nginx-ui invalida")
	}
	return x509.ParsePKCS1PublicKey(block.Bytes)
}

// syncCertificateToNginxUI cadastra o certificado no nginx-ui. Quando as
// credenciais do nginx-ui estiverem configuradas, usa a API oficial. Sem
// credenciais, importa diretamente no estado compartilhado do nginx-ui
// (database.db + /etc/nginx/ssl) para que o certificado apareca na tela
// /#/certificates/list logo apos a emissao no painel Bindnet. domain e o
// dominio/CN primario (usado no nome/caminho); sanDomains e a lista
// completa de dominios/IPs do certificado (SAN), incluindo o primario -
// so usada para preencher a coluna "domains" do nginx-ui.
func syncCertificateToNginxUI(db *sql.DB, domain string, sanDomains []string, certificatePEM, privateKeyPEM string) error {
	if nginxUIConfigured(db) {
		if err := syncCertificateToNginxUIAPI(db, domain, certificatePEM, privateKeyPEM); err == nil {
			return nil
		} else {
			log.Printf("[backend] sync via API do nginx-ui falhou para %s, tentando importacao local: %v", domain, err)
		}
	}

	return syncCertificateToNginxUILocal(domain, sanDomains, certificatePEM, privateKeyPEM)
}

func syncCertificateToNginxUIAPI(db *sql.DB, domain, certificatePEM, privateKeyPEM string) error {
	token, err := nginxUILogin(db)
	if err != nil {
		return err
	}

	certPath, keyPath := nginxUICertificatePaths(domain)
	payload, err := json.Marshal(map[string]string{
		"name":                     domain,
		"ssl_certificate_path":     certPath,
		"ssl_certificate_key_path": keyPath,
		"ssl_certificate":          certificatePEM,
		"ssl_certificate_key":      privateKeyPEM,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, nginxUIBaseURL()+"/api/certs", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("nginx-ui respondeu %d ao cadastrar certificado", resp.StatusCode)
	}
	log.Printf("[backend] certificado de %s carregado no nginx-ui", domain)
	return nil
}
