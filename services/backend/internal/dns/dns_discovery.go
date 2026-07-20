package dns

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const nginxConfigPath = "/nginx-config"

var nginxServerNameRegex = regexp.MustCompile(`(?s)\bserver_name\s+([^;]+);`)

type discoveredServer struct {
	Name   string `json:"name"`
	Zone   string `json:"zone"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	File   string `json:"file,omitempty"`
}

func discoveredServersFromNginx(root string) []discoveredServer {
	serversByName := map[string]discoveredServer{}
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, match := range nginxServerNameRegex.FindAllStringSubmatch(stripNginxComments(string(data)), -1) {
			for _, token := range strings.Fields(match[1]) {
				if server, ok := normalizeNginxServer(token, root, path); ok {
					serversByName[server.Name] = server
				}
			}
		}
		return nil
	})

	servers := make([]discoveredServer, 0, len(serversByName))
	for _, server := range serversByName {
		servers = append(servers, server)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	return servers
}

func normalizeNginxServer(value, root, path string) (discoveredServer, bool) {
	trimmed := strings.TrimSpace(value)
	name := normalizeDNSZone(trimmed)
	if name == "" || name == "_" || strings.ContainsAny(name, "$~") || !isValidDNSDomain(name) {
		return discoveredServer{}, false
	}
	kind := "host"
	if strings.HasPrefix(trimmed, "*.") {
		kind = "zona"
	}
	file, err := filepath.Rel(root, path)
	if err != nil {
		file = path
	}
	return discoveredServer{Name: name, Zone: name, Source: "nginx-ui", Kind: kind, File: file}, true
}

// normalizeDNSZone descarta prefixos wildcard ("*.", "**.", ".") e o
// ponto final, igual ao normalizeZone do dns-provider.
func normalizeDNSZone(value string) string {
	name := strings.ToLower(strings.TrimSpace(value))
	name = strings.TrimLeft(name, "*.")
	return strings.TrimSuffix(name, ".")
}

func isValidDNSDomain(domain string) bool {
	for _, label := range strings.Split(domain, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
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
