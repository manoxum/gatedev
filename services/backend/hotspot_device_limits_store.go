package main

import "database/sql"

// normalizeDeviceLimitUnits preenche "mbit"/"gbyte" nas unidades que
// vierem vazias no corpo do PATCH (frontend antigo, ou campo omitido) -
// garante que nunca violamos o CHECK de unidade nem gravamos "" no
// Postgres.
func normalizeDeviceLimitUnits(limits hotspotLimits) hotspotLimits {
	if limits.DownloadRateUnit == "" {
		limits.DownloadRateUnit = rateUnitMbit
	}
	if limits.UploadRateUnit == "" {
		limits.UploadRateUnit = rateUnitMbit
	}
	if limits.LimitType == "" {
		limits.LimitType = limitTypeUnlimited
	}
	// Cota e uma quantidade de dados, nao uma taxa - "gbyte" (nao
	// "mbit") e o default sensato quando vier vazio.
	if limits.DailyQuotaUnit == "" {
		limits.DailyQuotaUnit = rateUnitGbyte
	}
	if limits.WeeklyQuotaUnit == "" {
		limits.WeeklyQuotaUnit = rateUnitGbyte
	}
	if limits.MonthlyQuotaUnit == "" {
		limits.MonthlyQuotaUnit = rateUnitGbyte
	}
	return limits
}

func getDeviceLimits(db *sql.DB, mac string) (hotspotLimits, bool, error) {
	var limits hotspotLimits
	err := db.QueryRow(`
		SELECT download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		       limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
		       monthly_quota_bytes, monthly_quota_unit
		FROM hotspot_device_limits WHERE mac_address = $1
	`, mac).Scan(&limits.DownloadRateValue, &limits.DownloadRateUnit, &limits.UploadRateValue, &limits.UploadRateUnit,
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

func upsertDeviceLimits(db *sql.DB, mac string, limits hotspotLimits) error {
	limits = normalizeDeviceLimitUnits(limits)
	_, err := db.Exec(`
		INSERT INTO hotspot_device_limits (mac_address, download_rate_value, download_rate_unit, upload_rate_value, upload_rate_unit,
		                                    limit_type, daily_quota_bytes, daily_quota_unit, weekly_quota_bytes, weekly_quota_unit,
		                                    monthly_quota_bytes, monthly_quota_unit, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET download_rate_value = EXCLUDED.download_rate_value,
		    download_rate_unit = EXCLUDED.download_rate_unit,
		    upload_rate_value = EXCLUDED.upload_rate_value,
		    upload_rate_unit = EXCLUDED.upload_rate_unit,
		    limit_type = EXCLUDED.limit_type,
		    daily_quota_bytes = EXCLUDED.daily_quota_bytes,
		    daily_quota_unit = EXCLUDED.daily_quota_unit,
		    weekly_quota_bytes = EXCLUDED.weekly_quota_bytes,
		    weekly_quota_unit = EXCLUDED.weekly_quota_unit,
		    monthly_quota_bytes = EXCLUDED.monthly_quota_bytes,
		    monthly_quota_unit = EXCLUDED.monthly_quota_unit,
		    updated_at = CURRENT_TIMESTAMP
	`, mac, limits.DownloadRateValue, limits.DownloadRateUnit, limits.UploadRateValue, limits.UploadRateUnit,
		limits.LimitType, limits.DailyQuotaBytes, limits.DailyQuotaUnit, limits.WeeklyQuotaBytes, limits.WeeklyQuotaUnit,
		limits.MonthlyQuotaBytes, limits.MonthlyQuotaUnit)
	return err
}

// setDeviceLimitType troca so o limit_type de um dispositivo (cria o
// override se nao existir, preserva taxa/cota ja configuradas se
// existir) - usado pelo resgate de voucher (hotspot_vouchers.go) para
// ativar o modo credito automaticamente: e a unica acao de auto-servico
// do usuario final, sem acesso ao painel para trocar o tipo manualmente
// pela aba de limites.
func setDeviceLimitType(db *sql.DB, mac string, t limitType) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_device_limits (mac_address, limit_type)
		VALUES ($1, $2)
		ON CONFLICT (mac_address) DO UPDATE SET limit_type = EXCLUDED.limit_type, updated_at = CURRENT_TIMESTAMP
	`, mac, t)
	return err
}
