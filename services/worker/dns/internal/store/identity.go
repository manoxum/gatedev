package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
)

// EnsureNodeFingerprint devolve a identidade persistente deste no,
// gerando-a na primeira vez de forma concorrencia-segura (ON CONFLICT DO
// NOTHING seguido de SELECT).
func EnsureNodeFingerprint(ctx context.Context, db *sql.DB) (string, error) {
	var fingerprint string
	err := db.QueryRowContext(ctx, `SELECT fingerprint FROM discover_node_identity WHERE id = 1`).Scan(&fingerprint)
	if err == nil {
		return fingerprint, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	generated, err := randomFingerprint()
	if err != nil {
		return "", err
	}

	err = db.QueryRowContext(ctx, `
		INSERT INTO discover_node_identity (id, fingerprint, updated_at)
		VALUES (1, $1, now())
		ON CONFLICT (id) DO NOTHING
		RETURNING fingerprint
	`, generated).Scan(&fingerprint)
	if err == nil {
		return fingerprint, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	err = db.QueryRowContext(ctx, `SELECT fingerprint FROM discover_node_identity WHERE id = 1`).Scan(&fingerprint)
	return fingerprint, err
}

func randomFingerprint() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
