package main

import (
	"context"
	"database/sql"
	"time"
)

// applyManualRecharge soma o valor ao saldo (respeitando o plafond, se
// houver) e desbloqueia ao vivo se o dispositivo estava bloqueado por
// falta de credito e o saldo voltou a ficar positivo. O teto/plafond so
// e ajustado pelo formulario de configuracao (upsertDeviceCreditConfig),
// nunca pela recarga - aqui o valor enviado e sempre um incremento.
func applyManualRecharge(ctx context.Context, db *sql.DB, worker *workerClient, mac string, amountBytes int64) (hotspotDeviceCredit, error) {
	if _, err := ensureDeviceCreditRow(db, mac); err != nil {
		return hotspotDeviceCredit{}, err
	}
	var credit hotspotDeviceCredit
	err := db.QueryRow(`
		UPDATE hotspot_device_credit
		SET balance_bytes = CASE
		        WHEN plafond_bytes IS NOT NULL THEN LEAST(balance_bytes + $2, plafond_bytes)
		        ELSE balance_bytes + $2
		    END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1
		RETURNING mac_address, enabled, balance_bytes, recharge_amount_bytes, recharge_period,
		          plafond_bytes, next_recharge_at, blocked_by_credit
	`, mac, amountBytes).Scan(&credit.MACAddress, &credit.Enabled, &credit.BalanceBytes, &credit.RechargeAmountBytes,
		&credit.RechargePeriod, &credit.PlafondBytes, &credit.NextRechargeAt, &credit.BlockedByCredit)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	if err := recordCreditHistory(db, mac, "manual_recharge", amountBytes, credit.BalanceBytes); err != nil {
		return hotspotDeviceCredit{}, err
	}
	if credit.BlockedByCredit && credit.BalanceBytes > 0 {
		if err := unblockDeviceForCredit(db, mac); err != nil {
			return credit, err
		}
		credit.BlockedByCredit = false
		applyLiveCreditBlock(ctx, db, worker, mac, "", false)
	}
	return credit, nil
}

func unblockDeviceForCredit(db *sql.DB, mac string) error {
	_, err := db.Exec(`UPDATE hotspot_device_credit SET blocked_by_credit = false, updated_at = CURRENT_TIMESTAMP WHERE mac_address = $1`, mac)
	return err
}

func blockDeviceForCredit(db *sql.DB, mac string) error {
	_, err := db.Exec(`UPDATE hotspot_device_credit SET blocked_by_credit = true, updated_at = CURRENT_TIMESTAMP WHERE mac_address = $1`, mac)
	return err
}

// upsertDeviceCreditConfig grava a config de recarga. next_recharge_at
// so e recalculado (ancorado a partir de agora) quando o periodo muda
// ou a linha ainda nao tinha nenhum agendamento - trocar so o valor da
// recarga ou o plafond nao reinicia o relogio.
func upsertDeviceCreditConfig(db *sql.DB, mac string, req hotspotCreditConfigRequest) error {
	existing, found, err := getDeviceCreditPeriod(db, mac)
	if err != nil {
		return err
	}
	var nextRechargeAt *time.Time
	switch {
	case req.RechargePeriod == nil:
		nextRechargeAt = nil
	case !found || existing == nil || *existing != *req.RechargePeriod:
		next := time.Now().Add(periodDuration(*req.RechargePeriod))
		nextRechargeAt = &next
	default:
		nextRechargeAt, err = getDeviceNextRechargeAt(db, mac)
		if err != nil {
			return err
		}
	}

	_, err = db.Exec(`
		INSERT INTO hotspot_device_credit (mac_address, enabled, recharge_amount_bytes, recharge_period, plafond_bytes, next_recharge_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET enabled = EXCLUDED.enabled,
		    recharge_amount_bytes = EXCLUDED.recharge_amount_bytes,
		    recharge_period = EXCLUDED.recharge_period,
		    plafond_bytes = EXCLUDED.plafond_bytes,
		    next_recharge_at = EXCLUDED.next_recharge_at,
		    updated_at = CURRENT_TIMESTAMP
	`, mac, req.Enabled, req.RechargeAmountBytes, req.RechargePeriod, req.PlafondBytes, nextRechargeAt)
	return err
}

func periodDuration(period string) time.Duration {
	switch period {
	case "weekly":
		return 7 * 24 * time.Hour
	case "monthly":
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func getDeviceCreditPeriod(db *sql.DB, mac string) (*string, bool, error) {
	var period sql.NullString
	err := db.QueryRow(`SELECT recharge_period FROM hotspot_device_credit WHERE mac_address = $1`, mac).Scan(&period)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !period.Valid {
		return nil, true, nil
	}
	return &period.String, true, nil
}

func getDeviceNextRechargeAt(db *sql.DB, mac string) (*time.Time, error) {
	var next sql.NullTime
	err := db.QueryRow(`SELECT next_recharge_at FROM hotspot_device_credit WHERE mac_address = $1`, mac).Scan(&next)
	if err != nil {
		return nil, err
	}
	if !next.Valid {
		return nil, nil
	}
	return &next.Time, nil
}

// applyAutomaticRecharges avanca a recarga periodica de todo
// dispositivo cujo next_recharge_at ja passou - em loop por dispositivo
// (nao por periodo) para cobrir o caso do backend ter ficado fora do ar
// por mais de um periodo, sempre respeitando o plafond.
func applyAutomaticRecharges(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT mac_address FROM hotspot_device_credit
		WHERE recharge_period IS NOT NULL AND next_recharge_at IS NOT NULL AND next_recharge_at <= CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	var macs []string
	for rows.Next() {
		var mac string
		if err := rows.Scan(&mac); err != nil {
			rows.Close()
			return err
		}
		macs = append(macs, mac)
	}
	rows.Close()

	for _, mac := range macs {
		if err := advanceDeviceRecharge(db, mac); err != nil {
			return err
		}
	}
	return nil
}

func advanceDeviceRecharge(db *sql.DB, mac string) error {
	var balanceBytes, rechargeAmountBytes int64
	err := db.QueryRow(`
		UPDATE hotspot_device_credit
		SET balance_bytes = CASE
		        WHEN plafond_bytes IS NOT NULL THEN LEAST(balance_bytes + COALESCE(recharge_amount_bytes, 0), plafond_bytes)
		        ELSE balance_bytes + COALESCE(recharge_amount_bytes, 0)
		    END,
		    next_recharge_at = next_recharge_at + (
		        CASE recharge_period
		            WHEN 'weekly' THEN interval '7 days'
		            WHEN 'monthly' THEN interval '30 days'
		            ELSE interval '1 day'
		        END
		    ),
		    updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1 AND next_recharge_at <= CURRENT_TIMESTAMP
		RETURNING balance_bytes, COALESCE(recharge_amount_bytes, 0)
	`, mac).Scan(&balanceBytes, &rechargeAmountBytes)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if rechargeAmountBytes <= 0 {
		return nil
	}
	return recordCreditHistory(db, mac, "auto_recharge", rechargeAmountBytes, balanceBytes)
}
