// hotspot_vouchers.go gerencia vouchers (cartoes de recarga) do
// hotspot - emitidos em lote pelo admin com um valor fixo em bytes,
// resgatados uma unica vez pelo proprio dispositivo sem login (ver
// hotspot_portal.go), incrementando o saldo de credito exatamente como
// uma recarga manual (hotspot_credit_recharge.go). A criacao/listagem
// dos lotes (hotspot_voucher_batches) fica em hotspot_voucher_batches.go.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// errHotspotVoucherInvalid cobre tanto "codigo nao existe" quanto "ja
// foi usado/revogado" de proposito - devolver mensagens diferentes
// daria a quem tenta codigos ao acaso um oraculo para distinguir os
// dois casos.
var errHotspotVoucherInvalid = errors.New("codigo de voucher invalido ou ja utilizado")

const maxVoucherBatchSize = 100

type hotspotVoucher struct {
	Code          string     `json:"code"`
	BatchID       string     `json:"batchId,omitempty"`
	AmountBytes   int64      `json:"amountBytes"`
	Status        string     `json:"status"`
	Note          string     `json:"note,omitempty"`
	RedeemedByMAC string     `json:"redeemedByMac,omitempty"`
	RedeemedAt    *time.Time `json:"redeemedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

func registerHotspotVoucherRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, audit *auditClient) {
	registerHotspotVoucherBatchRoutes(mux, admin, db, audit)

	mux.HandleFunc("GET /api/hotspot/vouchers", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		vouchers, err := listVouchers(db, r.URL.Query().Get("status"), r.URL.Query().Get("batchId"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vouchers)
	}))

	mux.HandleFunc("DELETE /api/hotspot/vouchers/{code}", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("code")
		revoked, err := revokeVoucher(db, code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !revoked {
			http.Error(w, "voucher inexistente ou ja usado/revogado", http.StatusConflict)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "voucher_revoked", username, map[string]any{"code": code})
		w.WriteHeader(http.StatusNoContent)
	}))
}

func listVouchers(db *sql.DB, status, batchID string) ([]hotspotVoucher, error) {
	query := `
		SELECT code, COALESCE(batch_id, ''), amount_bytes, status, COALESCE(note, ''), COALESCE(redeemed_by_mac, ''), redeemed_at, created_at
		FROM hotspot_vouchers
	`
	conditions := []string{}
	args := []any{}
	if status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if batchID != "" {
		args = append(args, batchID)
		conditions = append(conditions, fmt.Sprintf("batch_id = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY created_at DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vouchers := []hotspotVoucher{}
	for rows.Next() {
		var v hotspotVoucher
		var redeemedAt sql.NullTime
		if err := rows.Scan(&v.Code, &v.BatchID, &v.AmountBytes, &v.Status, &v.Note, &v.RedeemedByMAC, &redeemedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		if redeemedAt.Valid {
			v.RedeemedAt = &redeemedAt.Time
		}
		vouchers = append(vouchers, v)
	}
	return vouchers, rows.Err()
}

func revokeVoucher(db *sql.DB, code string) (bool, error) {
	result, err := db.Exec(`UPDATE hotspot_vouchers SET status = 'revoked' WHERE code = $1 AND status = 'active'`, code)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// redeemVoucher e a operacao atomica de resgate: reivindica o codigo
// (so uma vez, garantido pelo "WHERE status='active'" + o lock de linha
// implicito do UPDATE) e credita o saldo do MAC chamador. O dispositivo
// que resgata um voucher passa a ter credito com saldo real rastreado -
// marca configured=true para que uma edicao futura do perfil vinculado
// nao apague esse saldo/config silenciosamente (ver
// syncDeviceCreditFromProfile em hotspot_profiles_apply.go).
func redeemVoucher(ctx context.Context, db *sql.DB, worker *workerClient, code, mac string) (hotspotDeviceCredit, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	defer tx.Rollback()

	var amountBytes int64
	err = tx.QueryRowContext(ctx, `
		UPDATE hotspot_vouchers
		SET status = 'redeemed', redeemed_by_mac = $1, redeemed_at = CURRENT_TIMESTAMP
		WHERE code = $2 AND status = 'active'
		RETURNING amount_bytes
	`, mac, code).Scan(&amountBytes)
	if err == sql.ErrNoRows {
		return hotspotDeviceCredit{}, errHotspotVoucherInvalid
	}
	if err != nil {
		return hotspotDeviceCredit{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hotspot_device_credit (mac_address, enabled, configured)
		VALUES ($1, true, true)
		ON CONFLICT (mac_address) DO UPDATE SET enabled = true, configured = true
	`, mac); err != nil {
		return hotspotDeviceCredit{}, err
	}

	// Resgatar um voucher e a unica acao de auto-servico do usuario final
	// (sem acesso ao painel para escolher o tipo de limitacao pela aba de
	// limites) - por isso forca o dispositivo para o tipo "credito" aqui,
	// preservando taxa/cota se ja houver override (ver setDeviceLimitType).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hotspot_device_limits (mac_address, limit_type)
		VALUES ($1, $2)
		ON CONFLICT (mac_address) DO UPDATE SET limit_type = EXCLUDED.limit_type, updated_at = CURRENT_TIMESTAMP
	`, mac, limitTypeCredit); err != nil {
		return hotspotDeviceCredit{}, err
	}

	var credit hotspotDeviceCredit
	err = tx.QueryRowContext(ctx, `
		UPDATE hotspot_device_credit
		SET balance_bytes = CASE
		        WHEN plafond_bytes IS NOT NULL THEN LEAST(balance_bytes + $2, plafond_bytes)
		        ELSE balance_bytes + $2
		    END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1
		RETURNING mac_address, enabled, balance_bytes, recharge_amount_bytes, recharge_period,
		          plafond_bytes, next_recharge_at, blocked_by_credit, configured
	`, mac, amountBytes).Scan(&credit.MACAddress, &credit.Enabled, &credit.BalanceBytes, &credit.RechargeAmountBytes,
		&credit.RechargePeriod, &credit.PlafondBytes, &credit.NextRechargeAt, &credit.BlockedByCredit, &credit.Configured)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hotspot_device_credit_history (mac_address, entry_type, amount_bytes, balance_after_bytes)
		VALUES ($1, 'voucher_redemption', $2, $3)
	`, mac, amountBytes, credit.BalanceBytes); err != nil {
		return hotspotDeviceCredit{}, err
	}

	if err := tx.Commit(); err != nil {
		return hotspotDeviceCredit{}, err
	}

	if credit.BalanceBytes > 0 {
		_ = unblockCreditIfNeeded(ctx, db, worker, mac, &credit)
	}
	return credit, nil
}
