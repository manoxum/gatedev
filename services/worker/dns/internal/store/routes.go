package store

import (
	"context"
	"database/sql"

	"bindnet/dns-provider/internal/core"
)

// LoadAllRoutes le a tabela discover_routes inteira - usada so para
// hidratar o snapshot em memoria na inicializacao (ver
// cmd/dns-provider/main.go), antes do primeiro ciclo de poll aos peers
// completar.
func LoadAllRoutes(ctx context.Context, db *sql.DB) (map[string]core.Route, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, owner, COALESCE(owner_fingerprint, ''), next_hop, distance, source, state, last_seen_at
		FROM discover_routes
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	routes := map[string]core.Route{}
	for rows.Next() {
		var route core.Route
		if err := rows.Scan(&route.Domain, &route.Owner, &route.OwnerFingerprint, &route.NextHop, &route.Distance, &route.Source, &route.State, &route.LastSeen); err != nil {
			return nil, err
		}
		routes[route.Domain] = route
	}
	return routes, rows.Err()
}

// PersistRoutes substitui a tabela de descoberta pelo snapshot atual no
// Postgres - chamada pela goroutine de poll (pacote discover) ao fim de
// cada ciclo, nunca no caminho quente de resolucao DNS.
func PersistRoutes(ctx context.Context, db *sql.DB, routes []core.Route) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM discover_routes`); err != nil {
		return err
	}
	for _, route := range routes {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO discover_routes (domain, owner, owner_fingerprint, next_hop, distance, source, state, last_seen_at)
			VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8)
		`, route.Domain, route.Owner, route.OwnerFingerprint, route.NextHop, route.Distance, route.Source, route.State, route.LastSeen)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func LoadConfiguredPeers(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT address FROM discover_configured_peers ORDER BY address
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []string
	for rows.Next() {
		var peer string
		if err := rows.Scan(&peer); err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}
	return peers, rows.Err()
}
