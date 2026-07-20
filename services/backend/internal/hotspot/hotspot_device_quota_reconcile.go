package hotspot

import (
	"bindnet/backend/internal/hotspot/store"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"log"
	"time"
)

func reconcileDeviceQuota(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac, ip string, limits store.Limits, deltaDown, deltaUp int64) error {
	anyExceeded := false
	for _, p := range store.ConfiguredQuotaPeriods(limits) {
		if _, err := store.EnsureDeviceQuotaPeriodRow(db, mac, p.Type); err != nil {
			return err
		}
		if err := store.ResetDeviceQuotaPeriodIfExpired(db, mac, p.Type); err != nil {
			return err
		}
		usage, err := store.IncrementDeviceQuotaPeriod(db, mac, p.Type, deltaDown, deltaUp)
		if err != nil {
			return err
		}
		exceeded := store.DeviceQuotaPeriodExceeded(p.Quota, usage)
		if exceeded != usage.Blocked {
			if err := store.SetDeviceQuotaPeriodBlocked(db, mac, p.Type, exceeded); err != nil {
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
func clearStaleDeviceQuotaBlock(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac, ip string) error {
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
		if err := store.SetDeviceQuotaPeriodBlocked(db, mac, t, false); err != nil {
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
func applyDeviceQuotaLive(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac string) {
	iface, err := store.HotspotWifiInterface(ctx, db)
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
	if limits.LimitType != store.LimitTypeQuota {
		return
	}
	if err := reconcileDeviceQuota(ctx, db, worker, mac, ip, limits, 0, 0); err != nil {
		log.Printf("[backend] reset de cota do dispositivo %s persistido, mas aplicacao ao vivo falhou: %v", mac, err)
	}
}

type hotspotDeviceQuotaPeriodResponse struct {
	PeriodType    string         `json:"periodType"`
	QuotaBytes    int64          `json:"quotaBytes"`
	QuotaUnit     store.RateUnit `json:"quotaUnit"`
	DownloadBytes int64          `json:"downloadBytes"`
	UploadBytes   int64          `json:"uploadBytes"`
	PeriodStart   string         `json:"periodStart"`
	PeriodEnd     string         `json:"periodEnd"`
	Blocked       bool           `json:"blocked"`
}

// listDeviceQuotaPeriods devolve so os periodos efetivamente
// configurados (via limits.LimitType/limits.*QuotaBytes) - usada pelo
// GET admin (hotspot_device_quota.go) e pelo portal de autoatendimento
// (hotspot_portal.go).
func listDeviceQuotaPeriods(db *sql.DB, mac string, limits store.Limits) ([]hotspotDeviceQuotaPeriodResponse, error) {
	configured := store.ConfiguredQuotaPeriods(limits)
	response := make([]hotspotDeviceQuotaPeriodResponse, 0, len(configured))
	for _, p := range configured {
		usage, err := store.EnsureDeviceQuotaPeriodRow(db, mac, p.Type)
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
