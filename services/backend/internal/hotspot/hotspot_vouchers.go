// hotspot_vouchers.go gerencia vouchers (cartoes de recarga) do
// hotspot - emitidos em lote pelo admin com um valor fixo em bytes,
// resgatados uma unica vez pelo proprio dispositivo sem login (ver
// hotspot_portal.go), incrementando o saldo de credito exatamente como
// uma recarga manual (hotspot_credit_recharge.go). A criacao/listagem
// dos lotes (hotspot_voucher_batches) fica em hotspot_voucher_batches.go.
package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/hotspot/store"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// errHotspotVoucherInvalid cobre tanto "codigo nao existe" quanto "ja
// foi usado/revogado" de proposito - devolver mensagens diferentes
// daria a quem tenta codigos ao acaso um oraculo para distinguir os
// dois casos.
var errHotspotVoucherInvalid = errors.New("codigo de voucher invalido ou ja utilizado")

const maxVoucherBatchSize = 100

func RegisterHotspotVoucherRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, audit *audit.Client) {
	RegisterHotspotVoucherBatchRoutes(mux, admin, db, audit)

	mux.HandleFunc("GET /api/hotspot/vouchers", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		vouchers, err := listVouchers(db, r.URL.Query().Get("status"), r.URL.Query().Get("batchId"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vouchers)
	}))

	mux.HandleFunc("DELETE /api/hotspot/vouchers/{code}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
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
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "voucher_revoked", username, map[string]any{"code": code})
		w.WriteHeader(http.StatusNoContent)
	}))
}

func listVouchers(db *sql.DB, status, batchID string) ([]store.Voucher, error) {
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

	vouchers := []store.Voucher{}
	for rows.Next() {
		var v store.Voucher
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
