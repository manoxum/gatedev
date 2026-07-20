// hotspot_voucher_codes.go gera os identificadores aleatorios usados
// por vouchers e lotes (services/backend/hotspot_vouchers.go e
// hotspot_voucher_batches.go).
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
)

// generateVoucherCode gera um codigo de 12 caracteres hexadecimais
// (96 bits de entropia via crypto/rand, mesma primitiva usada em
// admin.go para segredos), formatado em grupos de 4 pra facilitar
// digitacao manual: XXXX-XXXX-XXXX.
func generateVoucherCode() (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	hexCode := strings.ToUpper(hex.EncodeToString(raw))
	return hexCode[0:4] + "-" + hexCode[4:8] + "-" + hexCode[8:12], nil
}

// GenerateVoucherBatchID gera um identificador de 16 caracteres
// hexadecimais (64 bits de entropia via crypto/rand) para uso na URL
// da pagina de detalhe do lote - sem os hifens do codigo do voucher
// porque este nao e digitado manualmente por ninguem.
func GenerateVoucherBatchID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// InsertVoucherWithRetry tenta algumas vezes em caso de colisao de
// codigo (extremamente improvavel com 96 bits de entropia, mas o custo
// de tratar e minimo) antes de desistir.
func InsertVoucherWithRetry(db *sql.DB, batchID string, amountBytes int64, note string) (Voucher, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateVoucherCode()
		if err != nil {
			return Voucher{}, err
		}
		var v Voucher
		err = db.QueryRow(`
			INSERT INTO hotspot_vouchers (code, batch_id, amount_bytes, note)
			VALUES ($1, $2, $3, NULLIF($4, ''))
			RETURNING code, batch_id, amount_bytes, status, COALESCE(note, ''), created_at
		`, code, batchID, amountBytes, note).Scan(&v.Code, &v.BatchID, &v.AmountBytes, &v.Status, &v.Note, &v.CreatedAt)
		if err == nil {
			return v, nil
		}
		if isUniqueViolation(err) {
			lastErr = err
			continue
		}
		return Voucher{}, err
	}
	return Voucher{}, fmt.Errorf("falha ao gerar codigo de voucher unico: %w", lastErr)
}
