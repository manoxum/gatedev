package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
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
		          plafond_bytes, next_recharge_at, blocked_by_credit, configured
	`, mac, amountBytes).Scan(&credit.MACAddress, &credit.Enabled, &credit.BalanceBytes, &credit.RechargeAmountBytes,
		&credit.RechargePeriod, &credit.PlafondBytes, &credit.NextRechargeAt, &credit.BlockedByCredit, &credit.Configured)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	if err := recordCreditHistory(db, mac, "manual_recharge", amountBytes, credit.BalanceBytes); err != nil {
		return hotspotDeviceCredit{}, err
	}
	if credit.BalanceBytes > 0 {
		if err := unblockCreditIfNeeded(ctx, db, worker, mac, &credit); err != nil {
			return credit, err
		}
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

// unblockCreditIfNeeded desbloqueia ao vivo um dispositivo que estava
// bloqueado por falta de credito (no-op se nao estava) - compartilhado
// por applyManualRecharge, redeemVoucher (hotspot_vouchers.go) e
// syncDeviceCreditFromProfile (hotspot_profiles_apply.go).
func unblockCreditIfNeeded(ctx context.Context, db *sql.DB, worker *workerClient, mac string, credit *hotspotDeviceCredit) error {
	if !credit.BlockedByCredit {
		return nil
	}
	if err := unblockDeviceForCredit(db, mac); err != nil {
		return err
	}
	credit.BlockedByCredit = false
	applyLiveTrafficBlock(ctx, db, worker, mac, "", false)
	applyCaptivePortalRedirect(ctx, db, worker, mac, false)
	return nil
}

// applyCaptivePortalRedirect liga/desliga o redirecionamento
// automatico do portal cativo para um MAC bloqueado por falta de
// credito (nunca para o bloqueio manual do admin, ver
// services/worker/controller/captive_portal.go) - best-effort, so
// loga em falha, ja que o bloqueio de trafego em si (applyLiveTrafficBlock)
// e a garantia principal e ja foi aplicado separadamente.
func applyCaptivePortalRedirect(ctx context.Context, db *sql.DB, worker *workerClient, mac string, enable bool) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		return
	}
	if !enable {
		if err := worker.call(ctx, http.MethodPost, "/hotspot/captiveportal/disable", map[string]string{"interface": iface, "mac": mac}, nil); err != nil {
			log.Printf("[backend] falha ao desligar portal cativo de %s: %v", mac, err)
		}
		return
	}
	portalURL, err := hotspotPortalURL(ctx, db)
	if err != nil {
		log.Printf("[backend] falha ao montar URL do portal cativo para %s: %v", mac, err)
		return
	}
	if err := worker.call(ctx, http.MethodPost, "/hotspot/captiveportal/enable",
		map[string]string{"interface": iface, "mac": mac, "portalUrl": portalURL}, nil); err != nil {
		log.Printf("[backend] falha ao ligar portal cativo de %s: %v", mac, err)
	}
}

// computeNextRechargeAt decide o proximo agendamento de recarga
// periodica: mantem o relogio atual quando o periodo nao mudou (so
// trocar o valor da recarga ou o plafond nao reinicia o relogio), e so
// ancora um novo relogio a partir de agora quando o periodo muda ou
// nunca existiu. Reusada por upsertDeviceCreditConfig (config manual do
// admin) e por applyCreditPolicy (hotspot_profiles_apply.go, config
// vinda do perfil).
func computeNextRechargeAt(existingPeriod *string, existingNext *time.Time, newPeriod *string) *time.Time {
	switch {
	case newPeriod == nil:
		return nil
	case existingPeriod == nil || *existingPeriod != *newPeriod:
		next := time.Now().Add(periodDuration(*newPeriod))
		return &next
	default:
		return existingNext
	}
}

// upsertDeviceCreditConfig grava a config de recarga definida a mao
// pelo admin - marca configured=true, o que faz o dispositivo parar de
// herdar politica de credito do perfil vinculado (ver
// syncDeviceCreditFromProfile).
func upsertDeviceCreditConfig(db *sql.DB, mac string, req hotspotCreditConfigRequest) error {
	existingPeriod, _, err := getDeviceCreditPeriod(db, mac)
	if err != nil {
		return err
	}
	existingNext, err := getDeviceNextRechargeAt(db, mac)
	if err != nil {
		return err
	}
	nextRechargeAt := computeNextRechargeAt(existingPeriod, existingNext, req.RechargePeriod)

	_, err = db.Exec(`
		INSERT INTO hotspot_device_credit (mac_address, recharge_amount_bytes, recharge_period, plafond_bytes, next_recharge_at, configured, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET recharge_amount_bytes = EXCLUDED.recharge_amount_bytes,
		    recharge_period = EXCLUDED.recharge_period,
		    plafond_bytes = EXCLUDED.plafond_bytes,
		    next_recharge_at = EXCLUDED.next_recharge_at,
		    configured = true,
		    updated_at = CURRENT_TIMESTAMP
	`, mac, req.RechargeAmountBytes, req.RechargePeriod, req.PlafondBytes, nextRechargeAt)
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
	if err == sql.ErrNoRows {
		return nil, nil
	}
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
