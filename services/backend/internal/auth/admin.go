package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	adminPath          = "/data/admin.json"
	passwordIterations = 200_000
)

type Administrator struct {
	Username  string `json:"username"`
	SaltHex   string `json:"saltHex"`
	HashHex   string `json:"hashHex"`
	SecretHex string `json:"secretHex"`
}

func LoadOrCreate() (*Administrator, error) {
	return loadOrCreateAdminAt(adminPath)
}

// loadOrCreateAdminAt cria o administrador inicial ou atualiza o hash
// persistido quando ADMIN_USERNAME/ADMIN_PASSWORD mudam no .env.
func loadOrCreateAdminAt(path string) (*Administrator, error) {
	admin, exists, err := loadAdmin(path)
	if err != nil {
		return nil, err
	}

	username, password, configured, err := readAdminCredentials()
	if err != nil {
		return nil, err
	}
	if !configured {
		if exists {
			return admin, nil
		}
		return nil, errors.New("ADMIN_USERNAME e ADMIN_PASSWORD sao obrigatorios no primeiro boot do backend")
	}
	if exists && admin.Username == username && admin.validPassword(password) {
		return admin, nil
	}

	admin, err = newAdministrator(username, password)
	if err != nil {
		return nil, err
	}
	if err := saveAdmin(path, admin); err != nil {
		return nil, err
	}
	if exists {
		log.Println("[backend] credenciais de administrador atualizadas a partir do .env")
	} else {
		log.Println("[backend] credenciais de administrador criadas a partir do .env")
	}
	return admin, nil
}

func loadAdmin(path string) (*Administrator, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var admin Administrator
	if err := json.Unmarshal(data, &admin); err != nil {
		return nil, false, err
	}
	return &admin, true, nil
}

func readAdminCredentials() (string, string, bool, error) {
	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" && password == "" {
		return "", "", false, nil
	}
	if username == "" || password == "" {
		return "", "", false, errors.New("ADMIN_USERNAME e ADMIN_PASSWORD devem ser definidos juntos")
	}
	return username, password, true, nil
}

func newAdministrator(username, password string) (*Administrator, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return &Administrator{
		Username:  username,
		SaltHex:   hex.EncodeToString(salt),
		HashHex:   hex.EncodeToString(derivePassword(password, salt)),
		SecretHex: hex.EncodeToString(secret),
	}, nil
}

func saveAdmin(path string, admin *Administrator) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(admin, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// derivePassword deriva uma chave a partir da senha usando HMAC-SHA256
// iterado, sem adicionar dependencias fora da biblioteca padrao.
func derivePassword(password string, salt []byte) []byte {
	key := append([]byte{}, salt...)
	for i := 0; i < passwordIterations; i++ {
		mac := hmac.New(sha256.New, []byte(password))
		mac.Write(key)
		key = mac.Sum(nil)
	}
	return key
}

func (a *Administrator) validPassword(password string) bool {
	salt, err := hex.DecodeString(a.SaltHex)
	if err != nil {
		return false
	}
	expectedHash, err := hex.DecodeString(a.HashHex)
	if err != nil {
		return false
	}
	receivedHash := derivePassword(password, salt)
	return subtle.ConstantTimeCompare(receivedHash, expectedHash) == 1
}

func (a *Administrator) secret() []byte {
	secret, _ := hex.DecodeString(a.SecretHex)
	return secret
}
