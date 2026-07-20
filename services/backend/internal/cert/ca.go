// ca.go gerencia a autoridade certificadora local usada por
// certificates.go para assinar certificados sob demanda (nunca
// automaticamente por SNI, ao contrario do antigo
// services/worker/cert-proxy). Os parametros criptograficos e a
// importacao da CA legada seguem o mesmo comportamento do cert-proxy
// original - so o armazenamento (Postgres em vez de arquivo) mudou.
package cert

import (
	"bindnet/backend/internal/settings"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// legacyCertProxyPath e o volume cert_proxy_data, montado somente
// leitura - usado uma unica vez por LoadOrImportCA para trazer a CA
// existente do antigo cert-proxy para o Postgres. Nunca escrito pelo
// backend.
const legacyCertProxyPath = "/certproxy-data"

type localCA struct {
	ID             string
	Cert           *x509.Certificate
	Key            *rsa.PrivateKey
	CertificatePEM string
}

// LoadOrImportCA segue o mesmo padrao "load-or-create" de
// loadOrCreateAdmin (auth.go), com um passo intermediario de import:
//  1. Ja existe CA persistida no Postgres -> usa essa, nunca regenera.
//  2. Senao, existe CA legada no volume ro de cert-proxy -> importa para
//     o Postgres (preserva a confianca ja estabelecida nos dispositivos
//     que ja importaram essa CA).
//  3. Senao -> gera uma CA nova com os mesmos parametros do cert-proxy
//     original (RSA 4096, validade de 10 anos).
func LoadOrImportCA(db *sql.DB) (*localCA, error) {
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
			CommonName:   settings.CACommonName(context.Background(), db),
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
