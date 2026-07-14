// hotspot_credit_history.go grava e le a conta corrente de credito por
// dispositivo (recarga manual, recarga automatica, resgate de voucher
// e sessoes, ativas ou encerradas, como debito) - tudo no Postgres. O
// debito bruto por ciclo de reconciliacao (alto volume) mora no Mongo
// com TTL (ver hotspot_credit_trace.go) e so aparece agregado por
// sessao aqui (ver listSessionMovements em hotspot_sessions.go) - o
// extrato detalhado de uma sessao especifica fica atras do clique em
// GET .../sessions/{id}/consumption.
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

const hotspotCreditHistoryLimit = 100

type hotspotCreditHistoryResponse struct {
	EntryType         string  `json:"entryType"`
	AmountBytes       int64   `json:"amountBytes"`
	BalanceAfterBytes *int64  `json:"balanceAfterBytes,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	SessionID         *int64  `json:"sessionId,omitempty"`
	StartedAt         *string `json:"startedAt,omitempty"`
	EndedAt           *string `json:"endedAt,omitempty"`
}

// recordCreditHistory grava uma linha do extrato no Postgres - chamada
// logo apos cada UPDATE de balance_bytes por recarga manual, recarga
// automatica ou resgate de voucher (nunca por debito de trafego, ver
// creditTraceClient.recordDebit e listSessionMovements).
func recordCreditHistory(db *sql.DB, mac, entryType string, amountBytes, balanceAfterBytes int64) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_device_credit_history (mac_address, entry_type, amount_bytes, balance_after_bytes)
		VALUES ($1, $2, $3, $4)
	`, mac, entryType, amountBytes, balanceAfterBytes)
	return err
}

func registerHotspotCreditHistoryRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/credit/history", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		entries, err := listCreditHistory(db, mac, hotspotCreditHistoryLimit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))
}

// listCreditHistory mescla recarga/resgate de voucher (Postgres) com
// sessoes, ativas ou encerradas, como debito (Postgres, ver
// listSessionMovements) numa unica conta corrente ordenada por data -
// nenhuma das duas fontes toca o Mongo aqui, so o clique num debito
// especifico (GET .../sessions/{id}/consumption) busca o trace bruto
// la.
func listCreditHistory(db *sql.DB, mac string, limit int) ([]hotspotCreditHistoryResponse, error) {
	entries, err := listCreditRechargeHistory(db, mac, limit)
	if err != nil {
		return nil, err
	}
	debits, err := listSessionMovements(db, mac, limit)
	if err != nil {
		return nil, err
	}
	entries = append(entries, debits...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt > entries[j].CreatedAt })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func listCreditRechargeHistory(db *sql.DB, mac string, limit int) ([]hotspotCreditHistoryResponse, error) {
	rows, err := db.Query(`
		SELECT entry_type, amount_bytes, balance_after_bytes, created_at
		FROM hotspot_device_credit_history
		WHERE mac_address = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, mac, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []hotspotCreditHistoryResponse{}
	for rows.Next() {
		var entryType string
		var amountBytes, balanceAfterBytes int64
		var createdAt time.Time
		if err := rows.Scan(&entryType, &amountBytes, &balanceAfterBytes, &createdAt); err != nil {
			return nil, err
		}
		entries = append(entries, hotspotCreditHistoryResponse{
			EntryType:         entryType,
			AmountBytes:       amountBytes,
			BalanceAfterBytes: &balanceAfterBytes,
			CreatedAt:         createdAt.Format(time.RFC3339),
		})
	}
	return entries, rows.Err()
}
