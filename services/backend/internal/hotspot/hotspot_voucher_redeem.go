package hotspot

import (
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
)

func redeemVoucher(ctx context.Context, db *sql.DB, worker *workerapi.Client, code, mac string) (hotspotDeviceCredit, error) {
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
	`, mac, store.LimitTypeCredit); err != nil {
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
