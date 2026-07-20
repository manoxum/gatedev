package compose

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// defaultEnvPath e o env compartilhado do repositorio, montado em
// /workspace (raiz do repo) dentro do container do worker. O fluxo
// promote usa .env.main; .env fica como fallback para instalacoes
// antigas ainda nao migradas.
const defaultEnvPath = "/workspace/.env.main"

// envSections restringe quais chaves cada "section" pode ler/alterar - o
// backend nunca manda uma chave arbitraria, so uma dessas secoes.
var envSections = map[string][]string{
	"dns": {"DNS_LOCAL_TLDS", "DOMAINS", "DISCOVER_REMOTE_ROUTES", "DISCOVER_NODE_NAME", "DISCOVER_PORT"},
}

func RegisterEnvRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /env", handleEnvGet)
	mux.HandleFunc("PATCH /env", handleEnvPatch)
}

func handleEnvGet(w http.ResponseWriter, r *http.Request) {
	section := r.URL.Query().Get("section")
	keys, ok := envSections[section]
	if !ok {
		http.Error(w, "secao invalida", http.StatusBadRequest)
		return
	}
	path := EnvPath()
	values, err := readEnvValues(path, keys)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(values)
}

func handleEnvPatch(w http.ResponseWriter, r *http.Request) {
	section := r.URL.Query().Get("section")
	allowedKeys, ok := envSections[section]
	if !ok {
		http.Error(w, "secao invalida", http.StatusBadRequest)
		return
	}

	var values map[string]string
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return
	}

	allowed := map[string]bool{}
	for _, key := range allowedKeys {
		allowed[key] = true
	}
	for key := range values {
		if !allowed[key] {
			http.Error(w, fmt.Sprintf("chave '%s' nao pode ser alterada na secao '%s'", key, section), http.StatusForbidden)
			return
		}
	}

	path := EnvPath()
	if err := updateEnvKeys(path, values); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func EnvPath() string {
	if explicit := os.Getenv("BINDNET_ENV_PATH"); explicit != "" {
		return explicit
	}
	if _, err := os.Stat(defaultEnvPath); err == nil {
		return defaultEnvPath
	}
	return filepath.Join("/workspace", ".env")
}

// readEnvValues le o .env e devolve so as chaves pedidas.
func readEnvValues(path string, keys []string) (map[string]string, error) {
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[key] = true
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseEnvLine(scanner.Text())
		if ok && wanted[key] {
			values[key] = value
		}
	}
	return values, scanner.Err()
}

// updateEnvKeys reescreve o .env preservando comentarios, ordem e
// chaves nao mencionadas - so troca o valor das chaves passadas em
// "values" (as que ainda nao existirem sao acrescentadas no final).
// Nunca regenera o arquivo do zero.
func updateEnvKeys(path string, values map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	remaining := make(map[string]string, len(values))
	for key, value := range values {
		remaining[key] = value
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		key, _, ok := parseEnvLine(line)
		if !ok {
			continue
		}
		if newValue, exists := remaining[key]; exists {
			lines[i] = fmt.Sprintf("%s=%s", key, formatEnvValue(newValue))
			delete(remaining, key)
		}
	}
	for key, value := range remaining {
		lines = append(lines, fmt.Sprintf("%s=%s", key, formatEnvValue(value)))
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// parseEnvLine reconhece linhas "CHAVE=valor", ignorando comentarios e
// linhas em branco.
func parseEnvLine(line string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), normalizeEnvValue(parts[1]), true
}

func normalizeEnvValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 {
		return trimmed
	}
	if (strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`)) ||
		(strings.HasPrefix(trimmed, `'`) && strings.HasSuffix(trimmed, `'`)) {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			return unquoted
		}
		return strings.Trim(trimmed, `"'`)
	}
	return trimmed
}

func formatEnvValue(value string) string {
	if value == "" {
		return ""
	}
	needsQuote := false
	for _, r := range value {
		if unicode.IsSpace(r) || r == '"' || r == '\'' || r == '#' || r == '\\' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return value
	}
	return strconv.Quote(value)
}
