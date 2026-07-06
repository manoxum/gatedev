// hotspot_credit_history.go grava e le o extrato de mutacoes de saldo
// de credito por dispositivo (recarga manual, recarga automatica,
// debito de trafego) - alimenta a "conta corrente" de bytes exibida
// no detalhe do dispositivo.
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

const hotspotCreditHistoryLimit = 100

type hotspotCreditHistoryResponse struct {
	EntryType         string `json:"entryType"`
	AmountBytes       int64  `json:"amountBytes"`
	BalanceAfterBytes int64  `json:"balanceAfterBytes"`
	CreatedAt         string `json:"createdAt"`
}

// recordCreditHistory grava uma linha do extrato - chamada logo apos
// cada UPDATE de balance_bytes bem sucedido (recarga manual, recarga
// automatica, debito), sempre com o saldo ja atualizado.
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

func listCreditHistory(db *sql.DB, mac string, limit int) ([]hotspotCreditHistoryResponse, error) {
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
			BalanceAfterBytes: balanceAfterBytes,
			CreatedAt:         createdAt.Format(time.RFC3339),
		})
	}
	return entries, rows.Err()
}
