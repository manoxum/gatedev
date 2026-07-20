// Package nginx descobre os server_name declarados na configuracao do
// nginx-ui (montada somente leitura no dns-provider) - hosts concretos e
// zonas curinga ("*.dominio") que o dns-provider passa a tratar como
// nomes locais.
package nginx

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"bindnet/dns-provider/internal/config"
)

var serverNameRegex = regexp.MustCompile(`(?s)\bserver_name\s+([^;]+);`)

// Names separa os server_name descobertos em hosts concretos e zonas
// curinga.
type Names struct {
	Hosts map[string]bool
	Zones map[string]bool
}

func LoadNames(root string) Names {
	names := Names{Hosts: map[string]bool{}, Zones: map[string]bool{}}
	if root == "" {
		return names
	}
	if _, err := os.Stat(root); err != nil {
		log.Printf("[dns-provider] aviso: configuracao do nginx-ui indisponivel em %s: %v", root, err)
		return names
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[dns-provider] aviso: falha ao ler %s: %v", path, err)
			return nil
		}
		for _, match := range serverNameRegex.FindAllStringSubmatch(stripNginxComments(string(data)), -1) {
			for _, token := range strings.Fields(match[1]) {
				addName(names, token)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("[dns-provider] aviso: falha ao varrer configuracao do nginx-ui: %v", err)
	}
	return names
}

func addName(names Names, value string) {
	name := config.NormalizeZone(value)
	if name == "" || strings.ContainsAny(name, "$~") || name == "_" || !config.IsValidDomain(name) {
		return
	}
	if strings.HasPrefix(strings.TrimSpace(value), "*.") {
		names.Zones[name] = true
		return
	}
	names.Hosts[name] = true
}

func stripNginxComments(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}
