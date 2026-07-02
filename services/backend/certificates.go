// certificates.go implementa a gestao de certificados do painel:
// gera/importa uma CA local e emite/lista/revoga certificados assinados
// por ela, sob demanda via API (nunca automaticamente por SNI, ao
// contrario do antigo services/worker/cert-proxy). Os parametros
// criptograficos e a validacao de dominio sao os mesmos do cert-proxy
// original - so o gatilho (request HTTP explicito em vez de handshake
// TLS) e o armazenamento (Postgres em vez de arquivo) mudaram.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// legacyCertProxyPath e o volume cert_proxy_data, montado
	// somente leitura - usado uma unica vez por loadOrImportCA para
	// trazer a CA existente do antigo cert-proxy para o Postgres. Nunca
	// escrito pelo backend.
	legacyCertProxyPath = "/certproxy-data"
	defaultDomain       = "localhost.local"
)

type localCA struct {
	ID             string
	Cert           *x509.Certificate
	Key            *rsa.PrivateKey
	CertificatePEM string
}

// loadOrImportCA segue o mesmo padrao "load-or-create" de
// loadOrCreateAdmin (auth.go), com um passo intermediario de import:
//  1. Ja existe CA persistida no Postgres -> usa essa, nunca regenera.
//  2. Senao, existe CA legada no volume ro de cert-proxy -> importa para
//     o Postgres (preserva a confianca ja estabelecida nos dispositivos
//     que ja importaram essa CA).
//  3. Senao -> gera uma CA nova com os mesmos parametros do cert-proxy
//     original (RSA 4096, validade de 10 anos).
func loadOrImportCA(db *sql.DB) (*localCA, error) {
	ca, err := readCAFromPostgres(db)
	if err != nil {
		return nil, err
	}
	if ca != nil {
		log.Println("[backend] CA local existente carregada do Postgres")
		return ca, nil
	}

	certPath := filepath.Join(legacyCertProxyPath, "ca.crt")
	keyPath := filepath.Join(legacyCertProxyPath, "ca.key")
	if fileExists(certPath) && fileExists(keyPath) {
		certPEM, err := os.ReadFile(certPath)
		if err != nil {
			return nil, err
		}
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		cert, key, err := parseCAPEM(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("CA legada em %s invalida: %w", legacyCertProxyPath, err)
		}
		ca, err := insertCA(db, cert, string(certPEM), string(keyPEM), "imported")
		if err != nil {
			return nil, err
		}
		ca.Key = key
		log.Println("[backend] CA local legada (cert_proxy_data) importada para o Postgres - dispositivos ja confiando na CA continuam funcionando")
		return ca, nil
	}

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}
	serialNumber, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   getenv("CA_COMMON_NAME", "Bindnet Local Development CA"),
			Organization: []string{"Bindnet Docker Stack"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	ca, err = insertCA(db, tpl, string(certPEM), string(keyPEM), "generated")
	if err != nil {
		return nil, err
	}
	ca.Key = key
	log.Println("[backend] nova CA local gerada e persistida no Postgres")
	return ca, nil
}

func readCAFromPostgres(db *sql.DB) (*localCA, error) {
	var id, certificatePEM, privateKeyPEM string
	err := db.QueryRow(`SELECT id, certificate_pem, private_key_pem FROM ca LIMIT 1`).Scan(&id, &certificatePEM, &privateKeyPEM)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cert, key, err := parseCAPEM([]byte(certificatePEM), []byte(privateKeyPEM))
	if err != nil {
		return nil, err
	}
	return &localCA{ID: id, Cert: cert, Key: key, CertificatePEM: certificatePEM}, nil
}

func insertCA(db *sql.DB, cert *x509.Certificate, certificatePEM, privateKeyPEM, source string) (*localCA, error) {
	organization := "Bindnet Docker Stack"
	if len(cert.Subject.Organization) > 0 {
		organization = cert.Subject.Organization[0]
	}
	var id string
	err := db.QueryRow(
		`INSERT INTO ca (common_name, organization, serial_number, certificate_pem, private_key_pem, issued_at, expires_at, source)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		cert.Subject.CommonName, organization, cert.SerialNumber.String(),
		certificatePEM, privateKeyPEM, cert.NotBefore, cert.NotAfter, source,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &localCA{ID: id, Cert: cert, CertificatePEM: certificatePEM}, nil
}

// parseCAPEM segue a mesma decodificacao PEM/PKCS1 usada no antigo
// services/worker/cert-proxy/main.go:parseCA.
func parseCAPEM(certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, errors.New("certificado CA PEM invalido")
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, errors.New("chave CA PEM invalida")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

// normalizeDomain e uma copia byte a byte da funcao homonima do antigo
// cert-proxy: minusculo, sem porta, sem "." final; cai para
// defaultDomain se algum rotulo nao passar na validacao [a-z0-9-].
func normalizeDomain(value string) string {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if domain == "" {
		return defaultDomain
	}
	if h, _, err := net.SplitHostPort(domain); err == nil {
		domain = h
	}
	if ip := net.ParseIP(domain); ip != nil {
		return domain
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return defaultDomain
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return defaultDomain
			}
		}
	}
	return domain
}

type certificateResponse struct {
	ID          string   `json:"id"`
	Domain      string   `json:"domain"`
	CommonName  string   `json:"commonName"`
	DNSNames    []string `json:"dnsNames,omitempty"`
	IPAddresses []string `json:"ipAddresses,omitempty"`
	IssuedAt    string   `json:"issuedAt"`
	ExpiresAt   string   `json:"expiresAt"`
	RevokedAt   *string  `json:"revokedAt,omitempty"`
}

// issueCertificate sempre cria uma linha nova (sem cache de arquivo por
// dominio, ao contrario do antigo certificadoPara) - emitir e agora uma
// acao explicita do usuario via UI, nao um lookup implicito por SNI.
func issueCertificate(db *sql.DB, ca *localCA, rawDomain string) (*certificateResponse, error) {
	domain := normalizeDomain(rawDomain)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serialNumber, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(2, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	var dnsNames, ipAddresses []string
	if ip := net.ParseIP(domain); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
		ipAddresses = []string{domain}
	} else {
		tpl.DNSNames = []string{domain}
		dnsNames = []string{domain}
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	var id string
	err = db.QueryRow(
		`INSERT INTO certificates (domain, common_name, dns_names, ip_addresses, serial_number, certificate_pem, private_key_pem, issued_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		domain, domain, dnsNames, ipAddresses, serialNumber.String(),
		string(certPEM), string(keyPEM), tpl.NotBefore, tpl.NotAfter,
	).Scan(&id)
	if err != nil {
		return nil, err
	}

	log.Printf("[backend] certificado emitido para %s", domain)
	return &certificateResponse{
		ID: id, Domain: domain, CommonName: domain,
		DNSNames: dnsNames, IPAddresses: ipAddresses,
		IssuedAt: tpl.NotBefore.Format(time.RFC3339), ExpiresAt: tpl.NotAfter.Format(time.RFC3339),
	}, nil
}

func listCertificates(db *sql.DB) ([]certificateResponse, error) {
	rows, err := db.Query(
		`SELECT id, domain, common_name, dns_names, ip_addresses, issued_at, expires_at, revoked_at
		 FROM certificates ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []certificateResponse{}
	for rows.Next() {
		var r certificateResponse
		var issuedAt, expiresAt time.Time
		var revokedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.Domain, &r.CommonName, &r.DNSNames, &r.IPAddresses, &issuedAt, &expiresAt, &revokedAt); err != nil {
			return nil, err
		}
		r.IssuedAt = issuedAt.Format(time.RFC3339)
		r.ExpiresAt = expiresAt.Format(time.RFC3339)
		if revokedAt.Valid {
			formatted := revokedAt.Time.Format(time.RFC3339)
			r.RevokedAt = &formatted
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// revokeCertificate seta revoked_at em vez de deletar a linha -
// mantem o historico visivel na listagem (com status "revogado").
func revokeCertificate(db *sql.DB, id string) (string, error) {
	var domain string
	err := db.QueryRow(
		`UPDATE certificates SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL RETURNING domain`,
		id,
	).Scan(&domain)
	return domain, err
}

func certificatePEMByID(db *sql.DB, id string) (domain, certificatePEM string, err error) {
	err = db.QueryRow(`SELECT domain, certificate_pem FROM certificates WHERE id = $1`, id).Scan(&domain, &certificatePEM)
	return domain, certificatePEM, err
}

type issueCertificateRequest struct {
	Domain string `json:"domain"`
}

// registerCertificateRoutes substitui o antigo registrarRotasCertProxy
// (so leitura). Todas as rotas exigem sessao - diferente do antigo
// cert-proxy, que servia /ca.crt anonimamente na porta 80; esse acesso
// anonimo nao existe mais (nada escuta em 80/443 depois da remocao do
// cert-proxy).
func registerCertificateRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, ca *localCA, audit *auditClient) {
	mux.HandleFunc("GET /api/certificates", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		certificates, err := listCertificates(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(certificates)
	}))

	mux.HandleFunc("POST /api/certificates", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req issueCertificateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Domain == "" {
			http.Error(w, "campo 'domain' obrigatorio", http.StatusBadRequest)
			return
		}
		username, _ := sessionUser(r, admin)
		cert, err := issueCertificate(db, ca, req.Domain)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		audit.record(r.Context(), "certificate_issued", username, map[string]any{"id": cert.ID, "domain": cert.Domain})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(cert)
	}))

	mux.HandleFunc("DELETE /api/certificates/{id}", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		username, _ := sessionUser(r, admin)
		domain, err := revokeCertificate(db, id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "certificado nao encontrado ou ja revogado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		audit.record(r.Context(), "certificate_revoked", username, map[string]any{"id": id, "domain": domain})
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/certificates/{id}/download", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("GET /api/certificates/ca", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		serveCertificate(w, ca.CertificatePEM, "bindnet-local-ca.crt")
	}))
}

func serveCertificate(w http.ResponseWriter, pemContent, filename string) {
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte(pemContent))
}
