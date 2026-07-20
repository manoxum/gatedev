package compose

import (
	"os"
	"path/filepath"
)

// defaultEnvPath e o env compartilhado do repositorio, montado em
// /workspace (raiz do repo) dentro do container do worker. O fluxo
// promote usa .env.main; .env fica como fallback para instalacoes
// antigas ainda nao migradas.
const defaultEnvPath = "/workspace/.env.main"

func EnvPath() string {
	if explicit := os.Getenv("BINDNET_ENV_PATH"); explicit != "" {
		return explicit
	}
	if _, err := os.Stat(defaultEnvPath); err == nil {
		return defaultEnvPath
	}
	return filepath.Join("/workspace", ".env")
}
