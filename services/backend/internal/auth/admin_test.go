package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateAdminUpdatesFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "old-password")

	admin, err := loadOrCreateAdminAt(path)
	if err != nil {
		t.Fatalf("loadOrCreateAdminAt() erro inicial = %v", err)
	}
	if !admin.validPassword("old-password") {
		t.Fatal("senha inicial deveria ser valida")
	}
	oldSecret := admin.SecretHex

	t.Setenv("ADMIN_PASSWORD", "new-password")
	admin, err = loadOrCreateAdminAt(path)
	if err != nil {
		t.Fatalf("loadOrCreateAdminAt() erro ao atualizar = %v", err)
	}
	if admin.validPassword("old-password") {
		t.Fatal("senha antiga nao deveria continuar valida")
	}
	if !admin.validPassword("new-password") {
		t.Fatal("senha nova deveria ser valida")
	}
	if admin.SecretHex == oldSecret {
		t.Fatal("segredo de sessao deveria mudar ao trocar credenciais")
	}
}

func TestLoadOrCreateAdminUsesPersistedWhenEnvIsAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "kept-password")

	created, err := loadOrCreateAdminAt(path)
	if err != nil {
		t.Fatalf("loadOrCreateAdminAt() erro inicial = %v", err)
	}

	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_PASSWORD", "")
	loaded, err := loadOrCreateAdminAt(path)
	if err != nil {
		t.Fatalf("loadOrCreateAdminAt() erro ao carregar = %v", err)
	}
	if loaded.HashHex != created.HashHex || loaded.SecretHex != created.SecretHex {
		t.Fatal("administrador persistido deveria ser reutilizado sem env")
	}
}

func TestLoadOrCreateAdminRequiresEnvOnFirstBoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_PASSWORD", "")

	if _, err := loadOrCreateAdminAt(path); err == nil {
		t.Fatal("primeiro boot sem credenciais deveria falhar")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("admin.json nao deveria ser criado: %v", err)
	}
}
