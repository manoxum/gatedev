package main

import (
	"context"
	"database/sql"
	"log"
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
func reconcileDeviceQuota(ctx context.Context, db *sql.DB, worker *workerClient, mac, ip string, limits hotspotLimits, deltaDown, deltaUp int64) error {
	anyExceeded := false
	for _, p := range configuredQuotaPeriods(limits) {
		if _, err := ensureDeviceQuotaPeriodRow(db, mac, p.Type); err != nil {
			return err
		}
		if err := resetDeviceQuotaPeriodIfExpired(db, mac, p.Type); err != nil {
			return err
		}
		usage, err := incrementDeviceQuotaPeriod(db, mac, p.Type, deltaDown, deltaUp)
		if err != nil {
			return err
		}
		exceeded := deviceQuotaPeriodExceeded(p.Quota, usage)
		if exceeded != usage.Blocked {
			if err := setDeviceQuotaPeriodBlocked(db, mac, p.Type, exceeded); err != nil {
				return err
			}
		}
		anyExceeded = anyExceeded || exceeded
	}
	applyLiveTrafficBlock(ctx, db, worker, mac, ip, anyExceeded)
	applyCaptivePortalRedirect(ctx, db, worker, mac, anyExceeded)
	return nil
}

// clearStaleDeviceQuotaBlock desfaz um bloqueio por cota deixado de um
// LimitType anterior - chamada quando o dispositivo nao e (mais) do
// tipo "quota" (trocou pra credito/ilimitado), mesmo espirito do
// desbloqueio em syncDeviceCreditFromProfile para credito. No-op se
// nenhum periodo estiver marcado como bloqueado.
func clearStaleDeviceQuotaBlock(ctx context.Context, db *sql.DB, worker *workerClient, mac, ip string) error {
	rows, err := db.Query(`SELECT period_type FROM hotspot_device_quota_periods WHERE mac_address = $1 AND blocked = true`, mac)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		stale = append(stale, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	for _, t := range stale {
		if err := setDeviceQuotaPeriodBlocked(db, mac, t, false); err != nil {
			return err
		}
	}
	applyLiveTrafficBlock(ctx, db, worker, mac, ip, false)
	applyCaptivePortalRedirect(ctx, db, worker, mac, false)
	return nil
}

// applyDeviceQuotaLive reavalia e reaplica ao vivo o bloqueio por cota
// de um dispositivo (mesmo espirito de applyDeviceShapingLive) - usada
// depois do reset manual de um periodo, ja que so zerar aquele periodo
// nao deve desbloquear o MAC sozinho se outro periodo configurado
// ainda estiver estourado. Deltas 0 - so reavalia o estado atual, nao
// soma trafego novo. So age se o dispositivo estiver conectado agora e
// for do tipo cota (senao nao ha nada pra reaplicar; o proximo ciclo de
// reconciliacao cobre).
func applyDeviceQuotaLive(ctx context.Context, db *sql.DB, worker *workerClient, mac string) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] reset de cota do dispositivo %s persistido, mas nao foi possivel ler WIFI_INTERFACE: %v", mac, err)
		return
	}
	ip, found := liveHotspotClientIP(ctx, worker, iface, mac)
	if !found {
		return
	}
	limits, err := effectiveDeviceLimits(db, mac)
	if err != nil {
		log.Printf("[backend] reset de cota do dispositivo %s persistido, mas falha ao reler limites: %v", mac, err)
		return
	}
	if limits.LimitType != limitTypeQuota {
		return
	}
	if err := reconcileDeviceQuota(ctx, db, worker, mac, ip, limits, 0, 0); err != nil {
		log.Printf("[backend] reset de cota do dispositivo %s persistido, mas aplicacao ao vivo falhou: %v", mac, err)
	}
}

type hotspotDeviceQuotaPeriodResponse struct {
	PeriodType    string   `json:"periodType"`
	QuotaBytes    int64    `json:"quotaBytes"`
	QuotaUnit     rateUnit `json:"quotaUnit"`
	DownloadBytes int64    `json:"downloadBytes"`
	UploadBytes   int64    `json:"uploadBytes"`
	PeriodStart   string   `json:"periodStart"`
	PeriodEnd     string   `json:"periodEnd"`
	Blocked       bool     `json:"blocked"`
}

// listDeviceQuotaPeriods devolve so os periodos efetivamente
// configurados (via limits.LimitType/limits.*QuotaBytes) - usada pelo
// GET admin (hotspot_device_quota.go) e pelo portal de autoatendimento
// (hotspot_portal.go).
func listDeviceQuotaPeriods(db *sql.DB, mac string, limits hotspotLimits) ([]hotspotDeviceQuotaPeriodResponse, error) {
	configured := configuredQuotaPeriods(limits)
	response := make([]hotspotDeviceQuotaPeriodResponse, 0, len(configured))
	for _, p := range configured {
		usage, err := ensureDeviceQuotaPeriodRow(db, mac, p.Type)
		if err != nil {
			return nil, err
		}
		response = append(response, hotspotDeviceQuotaPeriodResponse{
			PeriodType:    p.Type,
			QuotaBytes:    *p.Quota,
			QuotaUnit:     p.Unit,
			DownloadBytes: usage.DownloadBytes,
			UploadBytes:   usage.UploadBytes,
			PeriodStart:   usage.PeriodStart.Format(time.RFC3339),
			PeriodEnd:     usage.PeriodEnd.Format(time.RFC3339),
			Blocked:       usage.Blocked,
		})
	}
	return response, nil
}
