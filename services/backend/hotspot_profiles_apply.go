package main

import (
	"context"
	"database/sql"
)

// defaultProfileID e o id fixo do perfil "Padrao" semeado pela
// migration 20260707000000_hotspot_profiles - mesmo idioma do literal
// 'global' ja usado por hotspot_global_traffic.id.
const defaultProfileID = "00000000-0000-0000-0000-000000000001"

// deviceProfileID devolve o perfil vinculado ao MAC, ou o Padrao se o
// dispositivo nunca foi visto (sem linha em hotspot_device_info) ou a
// coluna profile_id estiver nula.
func deviceProfileID(db *sql.DB, mac string) (string, error) {
	var profileID sql.NullString
	err := db.QueryRow(`SELECT profile_id FROM hotspot_device_info WHERE mac_address = $1`, mac).Scan(&profileID)
	if err == sql.ErrNoRows {
		return defaultProfileID, nil
	}
	if err != nil {
		return "", err
	}
	if !profileID.Valid {
		return defaultProfileID, nil
	}
	return profileID.String, nil
}

// assignDeviceProfile so toca a coluna profile_id - mesmo idioma de
// recordDeviceSeen (hotspot_device_info_store.go), nunca sobrescreve
// vendor/device_name/os_name/alias/confidence.
func assignDeviceProfile(db *sql.DB, mac, profileID string) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_device_info (mac_address, profile_id)
		VALUES ($1, $2)
		ON CONFLICT (mac_address) DO UPDATE SET profile_id = EXCLUDED.profile_id
	`, mac, profileID)
	return err
}

// effectiveDeviceLimits resolve os limites que devem valer agora para
// um MAC: o perfil vinculado decide o limite inteiro, A MENOS que o
// perfil seja do tipo "custom" - nesse caso (e so nesse caso) o
// override proprio do dispositivo (hotspot_device_limits) e quem
// decide, com "unlimited" como padrao enquanto o dispositivo ainda nao
// tiver configurado nada. Um override deixado de uma configuracao
// antiga fica dormente/ignorado se o perfil vinculado deixar de ser
// "custom".
func effectiveDeviceLimits(db *sql.DB, mac string) (hotspotLimits, error) {
	profileID, err := deviceProfileID(db, mac)
	if err != nil {
		return hotspotLimits{}, err
	}
	profileLimits, found, err := getProfileLimits(db, profileID)
	if err != nil {
		return hotspotLimits{}, err
	}
	if !found || profileLimits.LimitType != limitTypeCustom {
		return profileLimits, nil
	}
	deviceLimits, found, err := getDeviceLimits(db, mac)
	if err != nil {
		return hotspotLimits{}, err
	}
	if !found {
		return normalizeDeviceLimitUnits(hotspotLimits{LimitType: limitTypeUnlimited}), nil
	}
	return deviceLimits, nil
}

// syncDeviceCreditFromProfile mantem a politica de credito
// (rechargeAmount/rechargePeriod/plafond) do dispositivo em dia com o
// perfil vinculado - so age quando Configured=false (o dispositivo
// nunca teve config manual de credito nem resgatou um voucher, ver
// hotspot_vouchers.go). Nunca mexe em balance_bytes fora da regra de
// "so reseta o relogio de recarga se o periodo mudou"
// (computeNextRechargeAt, hotspot_credit_recharge.go).
//
// Quem decide se o credito esta "ativo" de fato e sempre o LimitType
// EFETIVO do dispositivo (effectiveDeviceLimits - perfil, ou o proprio
// override do dispositivo quando o perfil e "custom"), nunca um campo
// "enabled" gravado separadamente nem o LimitType cru do perfil (um
// perfil "custom" cujo dispositivo escolheu "credit" tambem conta como
// credito ativo). A politica de recarga (rechargeAmount/period/plafond),
// porem, so vem do PERFIL quando e o proprio perfil (nao "custom") que
// da origem ao credito - um dispositivo com override "credit" sob um
// perfil "custom" configura sua propria politica via PATCH .../credit
// (hotspot_credit_recharge.go), sem nada para herdar aqui.
func syncDeviceCreditFromProfile(ctx context.Context, db *sql.DB, worker *workerClient, mac string) (hotspotDeviceCredit, error) {
	credit, err := ensureDeviceCreditRow(db, mac)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	if credit.Configured {
		return credit, nil
	}
	effective, err := effectiveDeviceLimits(db, mac)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	if effective.LimitType != limitTypeCredit {
		if credit.BlockedByCredit {
			if err := unblockCreditIfNeeded(ctx, db, worker, mac, &credit); err != nil {
				return hotspotDeviceCredit{}, err
			}
		}
		return credit, nil
	}

	profileID, err := deviceProfileID(db, mac)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	profile, found, err := getProfile(db, profileID)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	if !found || profile.LimitType != limitTypeCredit {
		// credito efetivo veio do override "custom" do dispositivo, nao
		// do perfil - nao ha politica de perfil pra sincronizar aqui.
		return credit, nil
	}

	policy := hotspotCreditConfigRequest{
		RechargeAmountBytes: profile.CreditRechargeAmountBytes,
		RechargePeriod:      profile.CreditRechargePeriod,
		PlafondBytes:        profile.CreditPlafondBytes,
	}
	if creditPolicyMatches(credit, policy) {
		return credit, nil
	}
	return applyCreditPolicy(db, mac, policy)
}

func creditPolicyMatches(credit hotspotDeviceCredit, policy hotspotCreditConfigRequest) bool {
	return equalInt64Ptr(credit.RechargeAmountBytes, policy.RechargeAmountBytes) &&
		equalStringPtr(credit.RechargePeriod, policy.RechargePeriod) &&
		equalInt64Ptr(credit.PlafondBytes, policy.PlafondBytes)
}

func equalInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// applyCreditPolicy grava so as 3 colunas de politica vindas do perfil
// (nunca configured, balance_bytes ou blocked_by_credit) - usada
// exclusivamente por syncDeviceCreditFromProfile, so quando o perfil
// vinculado e do tipo credito (ver ali).
func applyCreditPolicy(db *sql.DB, mac string, policy hotspotCreditConfigRequest) (hotspotDeviceCredit, error) {
	existingPeriod, _, err := getDeviceCreditPeriod(db, mac)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	existingNext, err := getDeviceNextRechargeAt(db, mac)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	nextRechargeAt := computeNextRechargeAt(existingPeriod, existingNext, policy.RechargePeriod)

	var credit hotspotDeviceCredit
	err = db.QueryRow(`
		UPDATE hotspot_device_credit
		SET recharge_amount_bytes = $2, recharge_period = $3, plafond_bytes = $4,
		    next_recharge_at = $5, updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1
		RETURNING mac_address, enabled, balance_bytes, recharge_amount_bytes, recharge_period,
		          plafond_bytes, next_recharge_at, blocked_by_credit, configured
	`, mac, policy.RechargeAmountBytes, policy.RechargePeriod, policy.PlafondBytes, nextRechargeAt).Scan(
		&credit.MACAddress, &credit.Enabled, &credit.BalanceBytes, &credit.RechargeAmountBytes,
		&credit.RechargePeriod, &credit.PlafondBytes, &credit.NextRechargeAt, &credit.BlockedByCredit, &credit.Configured)
	return credit, err
}

// applyProfileShapingLive reaplica ao vivo o shaping de todo
// dispositivo conectado agora que esta vinculado a este perfil -
// chamado depois de editar um perfil, mesmo espirito de
// applyGlobalShapingLive/applyDeviceShapingLive. Nao pula mais quem tem
// override proprio em hotspot_device_limits: desde que o perfil decida
// o limite inteiro a menos que seja "custom" (ver effectiveDeviceLimits),
// um override deixado de uma configuracao antiga so importa se o
// perfil (recem-editado) for "custom" - ensureDeviceShaping ja resolve
// isso sozinho a cada chamada, entao reaplicar em todo mundo e sempre
// seguro/correto.
func applyProfileShapingLive(ctx context.Context, db *sql.DB, worker *workerClient, profileID string) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		return
	}
	clients, err := liveHotspotClients(ctx, worker, iface)
	if err != nil {
		return
	}
	for _, client := range clients {
		id, err := deviceProfileID(db, client.MAC)
		if err != nil || id != profileID {
			continue
		}
		_ = ensureDeviceShaping(ctx, db, worker, iface, client.MAC, client.IP)
		_, _ = syncDeviceCreditFromProfile(ctx, db, worker, client.MAC)
	}
}
