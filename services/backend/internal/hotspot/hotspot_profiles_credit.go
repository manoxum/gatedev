package hotspot

import (
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
)

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
func applyProfileShapingLive(ctx context.Context, db *sql.DB, worker *workerapi.Client, profileID string) {
	iface, err := store.HotspotWifiInterface(ctx, db)
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
