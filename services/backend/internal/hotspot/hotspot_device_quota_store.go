package hotspot

import (
	"database/sql"
	"time"
)

const (
	quotaPeriodDaily   = "daily"
	quotaPeriodWeekly  = "weekly"
	quotaPeriodMonthly = "monthly"
)

func isValidQuotaPeriodType(t string) bool {
	switch t {
	case quotaPeriodDaily, quotaPeriodWeekly, quotaPeriodMonthly:
		return true
	default:
		return false
	}
}

// hotspotDeviceQuotaPeriod e o acumulado do periodo corrente de UM dos
// 3 tetos possiveis (diario/semanal/mensal) de UM dispositivo - so
// existe linha para o periodo que o admin efetivamente configurou (ver
// ensureDeviceQuotaPeriodRow), nunca as 3 de uma vez se so 1 ou 2
// estiverem em uso. "Blocked" e bloqueio rigido (nunca throttle) - ver
// reconcileDeviceQuota.
type hotspotDeviceQuotaPeriod struct {
	MACAddress    string
	PeriodType    string
	DownloadBytes int64
	UploadBytes   int64
	PeriodStart   time.Time
	PeriodEnd     time.Time
	Blocked       bool
}

func ensureDeviceQuotaPeriodRow(db *sql.DB, mac, periodType string) (hotspotDeviceQuotaPeriod, error) {
	var p hotspotDeviceQuotaPeriod
	err := db.QueryRow(`
		INSERT INTO hotspot_device_quota_periods (mac_address, period_type)
		VALUES ($1, $2)
		ON CONFLICT (mac_address, period_type) DO UPDATE SET mac_address = EXCLUDED.mac_address
		RETURNING mac_address, period_type, download_bytes, upload_bytes, period_start, period_end, blocked
	`, mac, periodType).Scan(&p.MACAddress, &p.PeriodType, &p.DownloadBytes, &p.UploadBytes,
		&p.PeriodStart, &p.PeriodEnd, &p.Blocked)
	return p, err
}

// resetDeviceQuotaPeriodIfExpired e o equivalente por periodo de
// resetGlobalPeriodIfExpired (hotspot_quota.go) - reusa o mesmo
// periodInterval (whitelist fixa "daily"/"weekly"/"monthly").
func resetDeviceQuotaPeriodIfExpired(db *sql.DB, mac, periodType string) error {
	_, err := db.Exec(`
		UPDATE hotspot_device_quota_periods
		SET download_bytes = 0, upload_bytes = 0, blocked = false,
		    period_start = CURRENT_TIMESTAMP,
		    period_end = CURRENT_TIMESTAMP + interval '`+periodInterval(periodType)+`'
		WHERE mac_address = $1 AND period_type = $2 AND period_end <= CURRENT_TIMESTAMP
	`, mac, periodType)
	return err
}

// resetDeviceQuotaPeriodNow e a acao manual do botao "Resetar" (ver
// requisito de reset por periodo) - zera o acumulado e reinicia a
// janela a partir de agora, mesmo que o periodo atual ainda nao tenha
// expirado. Nao desbloqueia sozinho o dispositivo (outro periodo
// configurado pode continuar estourado) - quem decide isso e
// reconcileDeviceQuota no proximo ciclo/aplicacao ao vivo.
func resetDeviceQuotaPeriodNow(db *sql.DB, mac, periodType string) error {
	_, err := db.Exec(`
		UPDATE hotspot_device_quota_periods
		SET download_bytes = 0, upload_bytes = 0, blocked = false,
		    period_start = CURRENT_TIMESTAMP,
		    period_end = CURRENT_TIMESTAMP + interval '`+periodInterval(periodType)+`',
		    updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1 AND period_type = $2
	`, mac, periodType)
	return err
}

func incrementDeviceQuotaPeriod(db *sql.DB, mac, periodType string, deltaDown, deltaUp int64) (hotspotDeviceQuotaPeriod, error) {
	var p hotspotDeviceQuotaPeriod
	err := db.QueryRow(`
		UPDATE hotspot_device_quota_periods
		SET download_bytes = download_bytes + $3, upload_bytes = upload_bytes + $4, updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1 AND period_type = $2
		RETURNING mac_address, period_type, download_bytes, upload_bytes, period_start, period_end, blocked
	`, mac, periodType, deltaDown, deltaUp).Scan(&p.MACAddress, &p.PeriodType, &p.DownloadBytes, &p.UploadBytes,
		&p.PeriodStart, &p.PeriodEnd, &p.Blocked)
	return p, err
}

func setDeviceQuotaPeriodBlocked(db *sql.DB, mac, periodType string, blocked bool) error {
	_, err := db.Exec(`
		UPDATE hotspot_device_quota_periods SET blocked = $3, updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1 AND period_type = $2
	`, mac, periodType, blocked)
	return err
}

func deviceQuotaPeriodExceeded(quotaBytes *int64, usage hotspotDeviceQuotaPeriod) bool {
	if quotaBytes == nil {
		return false
	}
	return usage.DownloadBytes+usage.UploadBytes >= *quotaBytes
}

// configuredQuotaPeriods devolve os pares (tipo, teto) dos periodos que
// o admin efetivamente configurou em limits (ignora os que ficaram
// nil) - usada tanto por reconcileDeviceQuota quanto por
// listDeviceQuotaPeriods, pra nunca duas listas saírem dessincronizadas
// de quais periodos "contam".
func configuredQuotaPeriods(limits hotspotLimits) []struct {
	Type  string
	Quota *int64
	Unit  rateUnit
} {
	all := []struct {
		Type  string
		Quota *int64
		Unit  rateUnit
	}{
		{quotaPeriodDaily, limits.DailyQuotaBytes, limits.DailyQuotaUnit},
		{quotaPeriodWeekly, limits.WeeklyQuotaBytes, limits.WeeklyQuotaUnit},
		{quotaPeriodMonthly, limits.MonthlyQuotaBytes, limits.MonthlyQuotaUnit},
	}
	configured := make([]struct {
		Type  string
		Quota *int64
		Unit  rateUnit
	}, 0, len(all))
	for _, p := range all {
		if p.Quota != nil {
			configured = append(configured, p)
		}
	}
	return configured
}

// reconcileDeviceQuota e o analogo de reconcileDeviceCredit para os ate
// 3 periodos de cota configurados: reseta cada periodo expirado,
// incrementa com o trafego deste ciclo, e bloqueia ao vivo (mesma
// infra do bloqueio por credito - applyLiveTrafficBlock +
// applyCaptivePortalRedirect) assim que QUALQUER periodo configurado
// estoura. So chamada quando limits.LimitType == limitTypeQuota (ver
// hotspot_reconcile.go).
