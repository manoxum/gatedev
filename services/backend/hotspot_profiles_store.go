package main

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// errHotspotProfileNameTaken e devolvido por insertProfile/updateProfile
// quando o nome ja esta em uso por outro perfil (violacao da constraint
// UNIQUE em hotspot_profiles.name) - o handler HTTP traduz isso pra 409.
var errHotspotProfileNameTaken = errors.New("ja existe um perfil com esse nome")

// errHotspotProfileIsDefault e devolvido por deleteProfile quando o
// alvo e o perfil "Padrao" semeado pela migration - protegido a nivel
// de app (nao ha CHECK no banco que impeca deletar a linha is_default,
// so o indice unico que garante no maximo um is_default=true).
var errHotspotProfileIsDefault = errors.New("o perfil padrao nao pode ser removido")

const profileColumns = `
	id, name, is_default,
	download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
	limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
	monthly_quota_bytes, monthly_quota_unit,
	credit_recharge_amount_bytes, credit_recharge_period, credit_plafond_bytes
`

func scanProfile(row interface{ Scan(...any) error }) (hotspotProfile, error) {
	var p hotspotProfile
	err := row.Scan(&p.ID, &p.Name, &p.IsDefault,
		&p.DownloadRateValue, &p.DownloadRateUnit, &p.UploadRateValue, &p.UploadRateUnit,
		&p.LimitType, &p.DailyQuotaBytes, &p.DailyQuotaUnit, &p.WeeklyQuotaBytes, &p.WeeklyQuotaUnit,
		&p.MonthlyQuotaBytes, &p.MonthlyQuotaUnit,
		&p.CreditRechargeAmountBytes, &p.CreditRechargePeriod, &p.CreditPlafondBytes)
	if err != nil {
		return hotspotProfile{}, err
	}
	p.hotspotLimits = normalizeDeviceLimitUnits(p.hotspotLimits)
	return p, nil
}

func listProfiles(db *sql.DB) ([]hotspotProfile, error) {
	rows, err := db.Query(`SELECT ` + profileColumns + ` FROM hotspot_profiles ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profiles := []hotspotProfile{}
	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func getProfile(db *sql.DB, id string) (hotspotProfile, bool, error) {
	profile, err := scanProfile(db.QueryRow(`SELECT `+profileColumns+` FROM hotspot_profiles WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return hotspotProfile{}, false, nil
	}
	if err != nil {
		return hotspotProfile{}, false, err
	}
	return profile, true, nil
}

func insertProfile(db *sql.DB, req hotspotProfileRequest) (hotspotProfile, error) {
	req.hotspotLimits = normalizeDeviceLimitUnits(req.hotspotLimits)
	profile, err := scanProfile(db.QueryRow(`
		INSERT INTO hotspot_profiles (
			name, download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
			limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
			monthly_quota_bytes, monthly_quota_unit,
			credit_recharge_amount_bytes, credit_recharge_period, credit_plafond_bytes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING `+profileColumns,
		req.Name, req.DownloadRateValue, req.DownloadRateUnit, req.UploadRateValue, req.UploadRateUnit,
		req.LimitType, req.DailyQuotaBytes, req.DailyQuotaUnit, req.WeeklyQuotaBytes, req.WeeklyQuotaUnit,
		req.MonthlyQuotaBytes, req.MonthlyQuotaUnit,
		req.CreditRechargeAmountBytes, req.CreditRechargePeriod, req.CreditPlafondBytes,
	))
	if isUniqueViolation(err) {
		return hotspotProfile{}, errHotspotProfileNameTaken
	}
	return profile, err
}

func updateProfile(db *sql.DB, id string, req hotspotProfileRequest) (hotspotProfile, bool, error) {
	req.hotspotLimits = normalizeDeviceLimitUnits(req.hotspotLimits)
	profile, err := scanProfile(db.QueryRow(`
		UPDATE hotspot_profiles
		SET name = $2, download_rate_value = $3, download_rate_unit = $4, upload_rate_value = $5, upload_rate_unit = $6,
		    limit_type = $7, daily_quota_bytes = $8, daily_quota_unit = $9, weekly_quota_bytes = $10, weekly_quota_unit = $11,
		    monthly_quota_bytes = $12, monthly_quota_unit = $13,
		    credit_recharge_amount_bytes = $14, credit_recharge_period = $15, credit_plafond_bytes = $16,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING `+profileColumns,
		id, req.Name, req.DownloadRateValue, req.DownloadRateUnit, req.UploadRateValue, req.UploadRateUnit,
		req.LimitType, req.DailyQuotaBytes, req.DailyQuotaUnit, req.WeeklyQuotaBytes, req.WeeklyQuotaUnit,
		req.MonthlyQuotaBytes, req.MonthlyQuotaUnit,
		req.CreditRechargeAmountBytes, req.CreditRechargePeriod, req.CreditPlafondBytes,
	))
	if isUniqueViolation(err) {
		return hotspotProfile{}, false, errHotspotProfileNameTaken
	}
	if err == sql.ErrNoRows {
		return hotspotProfile{}, false, nil
	}
	if err != nil {
		return hotspotProfile{}, false, err
	}
	return profile, true, nil
}

// deleteProfile reatribui todo dispositivo vinculado ao perfil Padrao
// antes de apagar - nunca deixa hotspot_device_info.profile_id
// apontando para um perfil que nao existe mais.
func deleteProfile(db *sql.DB, id string) error {
	var isDefault bool
	if err := db.QueryRow(`SELECT is_default FROM hotspot_profiles WHERE id = $1`, id).Scan(&isDefault); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	if isDefault {
		return errHotspotProfileIsDefault
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE hotspot_device_info SET profile_id = $1 WHERE profile_id = $2`, defaultProfileID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM hotspot_profiles WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func getProfileLimits(db *sql.DB, id string) (hotspotLimits, bool, error) {
	var limits hotspotLimits
	err := db.QueryRow(`
		SELECT download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		       limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
		       monthly_quota_bytes, monthly_quota_unit
		FROM hotspot_profiles WHERE id = $1
	`, id).Scan(&limits.DownloadRateValue, &limits.DownloadRateUnit, &limits.UploadRateValue, &limits.UploadRateUnit,
		&limits.LimitType, &limits.DailyQuotaBytes, &limits.DailyQuotaUnit, &limits.WeeklyQuotaBytes, &limits.WeeklyQuotaUnit,
		&limits.MonthlyQuotaBytes, &limits.MonthlyQuotaUnit)
	if err == sql.ErrNoRows {
		return hotspotLimits{}, false, nil
	}
	if err != nil {
		return hotspotLimits{}, false, err
	}
	return normalizeDeviceLimitUnits(limits), true, nil
}

type hotspotProfileRef struct {
	ID   string
	Name string
}

// hotspotDeviceProfileRefs devolve o perfil (id+nome) efetivamente
// vinculado a cada MAC que ja tem linha em hotspot_device_info -
// dispositivos sem linha ainda (nunca vistos) nao aparecem aqui;
// listEnrichedHotspotClients trata a ausencia como perfil Padrao. O
// join sempre resolve para algum perfil (no minimo o Padrao, protegido
// de remocao) mesmo que profile_id esteja NULL numa linha antiga.
func hotspotDeviceProfileRefs(db *sql.DB) (map[string]hotspotProfileRef, error) {
	rows, err := db.Query(`
		SELECT i.mac_address, p.id, p.name
		FROM hotspot_device_info i
		JOIN hotspot_profiles p ON p.id = COALESCE(i.profile_id, $1)
	`, defaultProfileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refs := map[string]hotspotProfileRef{}
	for rows.Next() {
		var mac string
		var ref hotspotProfileRef
		if err := rows.Scan(&mac, &ref.ID, &ref.Name); err != nil {
			return nil, err
		}
		refs[mac] = ref
	}
	return refs, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
