// dns_peers.go expoe a busca manual de servidores Bindnet na rede local.
// A tabela discover_peers guarda o ultimo resultado de busca acionada pelo
// operador; nenhum peer entra automaticamente na malha sem virar
// registro em discover_configured_peers.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

type discoveredPeer struct {
	Address     string   `json:"address"`
	NodeName    string   `json:"nodeName"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Source      string   `json:"source"`
	LastSeenAt  string   `json:"lastSeenAt"`
	Domains     []string `json:"domains,omitempty"`
}

func registerDNSPeerRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, worker *workerClient, audit *auditClient) {
	mux.HandleFunc("GET /api/dns/peers", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(), `
			SELECT address, node_name, COALESCE(fingerprint, ''), COALESCE(array_to_string(domains, ','), ''), source, last_seen_at
			FROM discover_peers ORDER BY node_name, address
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		peers := []discoveredPeer{}
		for rows.Next() {
			var peer discoveredPeer
			var domains string
			if err := rows.Scan(&peer.Address, &peer.NodeName, &peer.Fingerprint, &domains, &peer.Source, &peer.LastSeenAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			peer.Domains = parsePeerList(domains)
			peers = append(peers, peer)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(peers)
	}))

	mux.HandleFunc("POST /api/dns/peers/search", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		if err := worker.call(r.Context(), http.MethodGet, "/env?section=dns", nil, &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		port := strings.TrimSpace(config["DISCOVER_PORT"])
		if port == "" {
			port = "8531"
		}

		var peers []discoveredPeer
		if err := worker.call(r.Context(), http.MethodPost, "/network/peer-scan", map[string]string{"port": port}, &peers); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if err := replaceDiscoveredPeers(r.Context(), db, peers); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "discover_peers_scanned", username, map[string]any{"count": len(peers)})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(peers)
	}))
}

func replaceDiscoveredPeers(ctx context.Context, db *sql.DB, peers []discoveredPeer) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM discover_peers`); err != nil {
		return err
	}
	for _, peer := range peers {
		if strings.TrimSpace(peer.Address) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO discover_peers (address, node_name, fingerprint, domains, source, last_seen_at)
			VALUES ($1, $2, NULLIF($3, ''), string_to_array($4, ','), 'manual-scan', now())
		`, peer.Address, fallbackPeerName(peer), strings.TrimSpace(peer.Fingerprint), strings.Join(normalizeDomains(peer.Domains), ",")); err != nil {
			return err
		}
	}
	return nil
}

func normalizeDomains(domains []string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, domain := range domains {
		value := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ".")))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func loadConfiguredPeerAddresses(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT address FROM discover_configured_peers ORDER BY address
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []string
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, err
		}
		peers = append(peers, address)
	}
	return peers, rows.Err()
}

func replaceConfiguredPeerAddresses(ctx context.Context, db *sql.DB, peers []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM discover_configured_peers`); err != nil {
		return err
	}
	for _, peer := range peers {
		if strings.TrimSpace(peer) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO discover_configured_peers (address, node_name, fingerprint, source, updated_at)
			SELECT $1, p.node_name, p.fingerprint, 'manual', now()
			FROM (SELECT $1::text AS address) input
			LEFT JOIN discover_peers p ON p.address = input.address
			ON CONFLICT (address) DO UPDATE SET
				node_name = COALESCE(EXCLUDED.node_name, discover_configured_peers.node_name),
				fingerprint = COALESCE(EXCLUDED.fingerprint, discover_configured_peers.fingerprint),
				updated_at = EXCLUDED.updated_at
		`, peer); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func fallbackPeerName(peer discoveredPeer) string {
	if strings.TrimSpace(peer.NodeName) != "" {
		return peer.NodeName
	}
	return peer.Address
}
