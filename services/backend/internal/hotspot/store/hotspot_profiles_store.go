package store

import (
	"database/sql"
	"errors"
)

// ErrHotspotProfileNameTaken e devolvido por InsertProfile/UpdateProfile
// quando o nome ja esta em uso por outro perfil (violacao da constraint
// UNIQUE em hotspot_profiles.name) - o handler HTTP traduz isso pra 409.
var ErrHotspotProfileNameTaken = errors.New("ja existe um perfil com esse nome")

// ErrHotspotProfileIsDefault e devolvido por DeleteProfile quando o
// alvo e o perfil "Padrao" semeado pela migration - protegido a nivel
// de app (nao ha CHECK no banco que impeca deletar a linha is_default,
// so o indice unico que garante no maximo um is_default=true).
var ErrHotspotProfileIsDefault = errors.New("o perfil padrao nao pode ser removido")

const profileColumns = `
	id, name, is_default,
	download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
	limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
	monthly_quota_bytes, monthly_quota_unit,
	credit_recharge_amount_bytes, credit_recharge_period, credit_plafond_bytes,
	allow_internal_communication
`

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.Name, &p.IsDefault,
		&p.DownloadRateValue, &p.DownloadRateUnit, &p.UploadRateValue, &p.UploadRateUnit,
		&p.LimitType, &p.DailyQuotaBytes, &p.DailyQuotaUnit, &p.WeeklyQuotaBytes, &p.WeeklyQuotaUnit,
		&p.MonthlyQuotaBytes, &p.MonthlyQuotaUnit,
		&p.CreditRechargeAmountBytes, &p.CreditRechargePeriod, &p.CreditPlafondBytes,
		&p.AllowInternalCommunication)
	if err != nil {
		return Profile{}, err
	}
	p.Limits = NormalizeDeviceLimitUnits(p.Limits)
	return p, nil
}

func ListProfiles(db *sql.DB) ([]Profile, error) {
	rows, err := db.Query(`SELECT ` + profileColumns + ` FROM hotspot_profiles ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profiles := []Profile{}
	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func GetProfile(db *sql.DB, id string) (Profile, bool, error) {
	profile, err := scanProfile(db.QueryRow(`SELECT `+profileColumns+` FROM hotspot_profiles WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	return profile, true, nil
}

func InsertProfile(db *sql.DB, req ProfileRequest) (Profile, error) {
	req.Limits = NormalizeDeviceLimitUnits(req.Limits)
	profile, err := scanProfile(db.QueryRow(`
		INSERT INTO hotspot_profiles (
			name, download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
			limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
			monthly_quota_bytes, monthly_quota_unit,
			credit_recharge_amount_bytes, credit_recharge_period, credit_plafond_bytes,
			allow_internal_communication
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING `+profileColumns,
		req.Name, req.DownloadRateValue, req.DownloadRateUnit, req.UploadRateValue, req.UploadRateUnit,
		req.LimitType, req.DailyQuotaBytes, req.DailyQuotaUnit, req.WeeklyQuotaBytes, req.WeeklyQuotaUnit,
		req.MonthlyQuotaBytes, req.MonthlyQuotaUnit,
		req.CreditRechargeAmountBytes, req.CreditRechargePeriod, req.CreditPlafondBytes,
		req.AllowInternalCommunication,
	))
	if isUniqueViolation(err) {
		return Profile{}, ErrHotspotProfileNameTaken
	}
	return profile, err
}

func UpdateProfile(db *sql.DB, id string, req ProfileRequest) (Profile, bool, error) {
	req.Limits = NormalizeDeviceLimitUnits(req.Limits)
	profile, err := scanProfile(db.QueryRow(`
		UPDATE hotspot_profiles
		SET name = $2, download_rate_value = $3, download_rate_unit = $4, upload_rate_value = $5, upload_rate_unit = $6,
		    limit_type = $7, daily_quota_bytes = $8, daily_quota_unit = $9, weekly_quota_bytes = $10, weekly_quota_unit = $11,
		    monthly_quota_bytes = $12, monthly_quota_unit = $13,
		    credit_recharge_amount_bytes = $14, credit_recharge_period = $15, credit_plafond_bytes = $16,
		    allow_internal_communication = $17,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING `+profileColumns,
		id, req.Name, req.DownloadRateValue, req.DownloadRateUnit, req.UploadRateValue, req.UploadRateUnit,
		req.LimitType, req.DailyQuotaBytes, req.DailyQuotaUnit, req.WeeklyQuotaBytes, req.WeeklyQuotaUnit,
		req.MonthlyQuotaBytes, req.MonthlyQuotaUnit,
		req.CreditRechargeAmountBytes, req.CreditRechargePeriod, req.CreditPlafondBytes,
		req.AllowInternalCommunication,
	))
	if isUniqueViolation(err) {
		return Profile{}, false, ErrHotspotProfileNameTaken
	}
	if err == sql.ErrNoRows {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	return profile, true, nil
}

// DeleteProfile reatribui todo dispositivo vinculado ao perfil Padrao
// antes de apagar - nunca deixa hotspot_device_info.profile_id
// apontando para um perfil que nao existe mais.
func DeleteProfile(db *sql.DB, id string) error {
	var isDefault bool
	if err := db.QueryRow(`SELECT is_default FROM hotspot_profiles WHERE id = $1`, id).Scan(&isDefault); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	if isDefault {
		return ErrHotspotProfileIsDefault
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE hotspot_device_info SET profile_id = $1 WHERE profile_id = $2`, DefaultProfileID, id); err != nil {
		return err
	}
	// Regras de comunicacao que referenciam o perfil sao removidas na
	// mesma transacao (refs polimorficas, sem FK no banco - ver
	// hotspot_comm_rules.go): regra orfa apontando pra perfil apagado
	// nunca deve sobrar avaliavel no motor de isolamento.
	if err := deleteCommRulesForProfileTx(tx, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM hotspot_profiles WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func GetProfileLimits(db *sql.DB, id string) (Limits, bool, error) {
	var limits Limits
	err := db.QueryRow(`
		SELECT download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		       limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
		       monthly_quota_bytes, monthly_quota_unit
		FROM hotspot_profiles WHERE id = $1
	`, id).Scan(&limits.DownloadRateValue, &limits.DownloadRateUnit, &limits.UploadRateValue, &limits.UploadRateUnit,
		&limits.LimitType, &limits.DailyQuotaBytes, &limits.DailyQuotaUnit, &limits.WeeklyQuotaBytes, &limits.WeeklyQuotaUnit,
		&limits.MonthlyQuotaBytes, &limits.MonthlyQuotaUnit)
	if err == sql.ErrNoRows {
		return Limits{}, false, nil
	}
	if err != nil {
		return Limits{}, false, err
	}
	return NormalizeDeviceLimitUnits(limits), true, nil
}
