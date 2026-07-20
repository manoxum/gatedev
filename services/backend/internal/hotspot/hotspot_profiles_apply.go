package hotspot

import (
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
)

// deviceProfileID devolve o perfil vinculado ao MAC, ou o Padrao se o
// dispositivo nunca foi visto (sem linha em hotspot_device_info) ou a
// coluna profile_id estiver nula.
func deviceProfileID(db *sql.DB, mac string) (string, error) {
	var profileID sql.NullString
	err := db.QueryRow(`SELECT profile_id FROM hotspot_device_info WHERE mac_address = $1`, mac).Scan(&profileID)
	if err == sql.ErrNoRows {
		return store.DefaultProfileID, nil
	}
	if err != nil {
		return "", err
	}
	if !profileID.Valid {
		return store.DefaultProfileID, nil
	}
	return profileID.String, nil
}

// assignDeviceProfile so toca a coluna profile_id - mesmo idioma de
// store.RecordDeviceSeen (hotspot_device_info_store.go), nunca sobrescreve
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
func effectiveDeviceLimits(db *sql.DB, mac string) (store.Limits, error) {
	profileID, err := deviceProfileID(db, mac)
	if err != nil {
		return store.Limits{}, err
	}
	profileLimits, found, err := store.GetProfileLimits(db, profileID)
	if err != nil {
		return store.Limits{}, err
	}
	if !found || profileLimits.LimitType != store.LimitTypeCustom {
		return profileLimits, nil
	}
	deviceLimits, found, err := store.GetDeviceLimits(db, mac)
	if err != nil {
		return store.Limits{}, err
	}
	if !found {
		return store.NormalizeDeviceLimitUnits(store.Limits{LimitType: store.LimitTypeUnlimited}), nil
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
func syncDeviceCreditFromProfile(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac string) (hotspotDeviceCredit, error) {
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
	if effective.LimitType != store.LimitTypeCredit {
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
	profile, found, err := store.GetProfile(db, profileID)
	if err != nil {
		return hotspotDeviceCredit{}, err
	}
	if !found || profile.LimitType != store.LimitTypeCredit {
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
