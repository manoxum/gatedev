// hotspot_voucher_batches.go gerencia a emissao em lote de vouchers -
// um lote agrupa todos os vouchers criados numa mesma chamada de
// emissao (hotspot_vouchers.go) para permitir listar e imprimir a
// emissao inteira de uma vez no painel.
package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type hotspotVoucherBatch struct {
	ID            string    `json:"id"`
	AmountBytes   int64     `json:"amountBytes"`
	AmountUnit    string    `json:"amountUnit"`
	Quantity      int       `json:"quantity"`
	Note          string    `json:"note,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	ActiveCount   int       `json:"activeCount"`
	RedeemedCount int       `json:"redeemedCount"`
	RevokedCount  int       `json:"revokedCount"`
}

type hotspotVoucherIssueRequest struct {
	AmountBytes int64  `json:"amountBytes"`
	AmountUnit  string `json:"amountUnit"`
	Quantity    int    `json:"quantity"`
	Note        string `json:"note,omitempty"`
}

// voucherAmountUnits e o mesmo conjunto de RateUnit usado no frontend
// (hotspot-limits-types.ts) - validado aqui so para nao gravar lixo na
// coluna com CHECK do Postgres (ver migration
// 20260707050000_hotspot_voucher_batch_amount_unit).
var voucherAmountUnits = map[string]bool{
	"kbit": true, "mbit": true, "gbit": true,
	"kbyte": true, "mbyte": true, "gbyte": true,
}

type hotspotVoucherIssueResponse struct {
	Batch    hotspotVoucherBatch `json:"batch"`
	Vouchers []hotspotVoucher    `json:"vouchers"`
}

func RegisterHotspotVoucherBatchRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, audit *audit.Client) {
	mux.HandleFunc("GET /api/hotspot/voucher-batches", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		batches, err := listVoucherBatches(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(batches)
	}))

	mux.HandleFunc("GET /api/hotspot/voucher-batches/{id}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		batch, err := getVoucherBatch(db, r.PathValue("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "lote nao encontrado", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(batch)
	}))

	mux.HandleFunc("POST /api/hotspot/vouchers", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req hotspotVoucherIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AmountBytes <= 0 {
			http.Error(w, "campo 'amountBytes' deve ser positivo", http.StatusBadRequest)
			return
		}
		if req.Quantity <= 0 {
			req.Quantity = 1
		}
		if req.Quantity > maxVoucherBatchSize {
			http.Error(w, fmt.Sprintf("no maximo %d vouchers por emissao", maxVoucherBatchSize), http.StatusBadRequest)
			return
		}
		if req.AmountUnit == "" {
			req.AmountUnit = "gbyte"
		}
		if !voucherAmountUnits[req.AmountUnit] {
			http.Error(w, "campo 'amountUnit' invalido", http.StatusBadRequest)
			return
		}
		note := strings.TrimSpace(req.Note)
		batch, vouchers, err := insertVoucherBatch(db, req.Quantity, req.AmountBytes, req.AmountUnit, note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "voucher_issued", username, map[string]any{
			"batchId": batch.ID, "quantity": req.Quantity, "amountBytes": req.AmountBytes,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(hotspotVoucherIssueResponse{Batch: batch, Vouchers: vouchers})
	}))
}

func listVoucherBatches(db *sql.DB) ([]hotspotVoucherBatch, error) {
	rows, err := db.Query(`
		SELECT b.id, b.amount_bytes, b.amount_unit, b.quantity, COALESCE(b.note, ''), b.created_at,
		       COUNT(*) FILTER (WHERE v.status = 'active') AS active_count,
		       COUNT(*) FILTER (WHERE v.status = 'redeemed') AS redeemed_count,
		       COUNT(*) FILTER (WHERE v.status = 'revoked') AS revoked_count
		FROM hotspot_voucher_batches b
		LEFT JOIN hotspot_vouchers v ON v.batch_id = b.id
		GROUP BY b.id
		ORDER BY b.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	batches := []hotspotVoucherBatch{}
	for rows.Next() {
		var b hotspotVoucherBatch
		if err := rows.Scan(&b.ID, &b.AmountBytes, &b.AmountUnit, &b.Quantity, &b.Note, &b.CreatedAt,
			&b.ActiveCount, &b.RedeemedCount, &b.RevokedCount); err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

func getVoucherBatch(db *sql.DB, id string) (hotspotVoucherBatch, error) {
	var b hotspotVoucherBatch
	err := db.QueryRow(`
		SELECT b.id, b.amount_bytes, b.amount_unit, b.quantity, COALESCE(b.note, ''), b.created_at,
		       COUNT(*) FILTER (WHERE v.status = 'active') AS active_count,
		       COUNT(*) FILTER (WHERE v.status = 'redeemed') AS redeemed_count,
		       COUNT(*) FILTER (WHERE v.status = 'revoked') AS revoked_count
		FROM hotspot_voucher_batches b
		LEFT JOIN hotspot_vouchers v ON v.batch_id = b.id
		WHERE b.id = $1
		GROUP BY b.id
	`, id).Scan(&b.ID, &b.AmountBytes, &b.AmountUnit, &b.Quantity, &b.Note, &b.CreatedAt,
		&b.ActiveCount, &b.RedeemedCount, &b.RevokedCount)
	return b, err
}

// insertVoucherBatch cria o registro do lote e, em seguida, os
// vouchers vinculados a ele - agrupamento que permite listar e
// imprimir a emissao inteira de uma vez (ver HotspotVouchersCard e
// HotspotVoucherBatchDetail no frontend).
func insertVoucherBatch(db *sql.DB, quantity int, amountBytes int64, amountUnit, note string) (hotspotVoucherBatch, []hotspotVoucher, error) {
	batchID, err := generateVoucherBatchID()
	if err != nil {
		return hotspotVoucherBatch{}, nil, err
	}
	var batch hotspotVoucherBatch
	err = db.QueryRow(`
		INSERT INTO hotspot_voucher_batches (id, amount_bytes, amount_unit, quantity, note)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		RETURNING id, amount_bytes, amount_unit, quantity, COALESCE(note, ''), created_at
	`, batchID, amountBytes, amountUnit, quantity, note).Scan(
		&batch.ID, &batch.AmountBytes, &batch.AmountUnit, &batch.Quantity, &batch.Note, &batch.CreatedAt)
	if err != nil {
		return hotspotVoucherBatch{}, nil, err
	}
	batch.ActiveCount = quantity

	vouchers := make([]hotspotVoucher, 0, quantity)
	for i := 0; i < quantity; i++ {
		voucher, err := insertVoucherWithRetry(db, batchID, amountBytes, note)
		if err != nil {
			return hotspotVoucherBatch{}, nil, err
		}
		vouchers = append(vouchers, voucher)
	}
	return batch, vouchers, nil
}
