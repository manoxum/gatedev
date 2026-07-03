package main

import "database/sql"

// counterDelta trata um contador atual menor que o ultimo lido como
// "a regra de contagem foi recriada" (reboot do hotspot ou reset de
// IP) em vez de delta negativo - o valor atual passa a ser o delta
// inteiro, exatamente como se fosse trafego novo desde a recriacao.
func counterDelta(last int64, current uint64) int64 {
	currentSigned := int64(current)
	if currentSigned < last {
		return currentSigned
	}
	return currentSigned - last
}

// recordDeviceUsage soma o delta desde a ultima leitura ao acumulado
// do periodo e devolve os proprios deltas - o loop de reconciliacao
// usa esse retorno para debitar exatamente o trafego deste ciclo do
// saldo de credito, sem reler o acumulado inteiro.
func recordDeviceUsage(db *sql.DB, mac string, downloadCounter, uploadCounter uint64) (deltaDown, deltaUp int64, err error) {
	traffic, err := ensureDeviceTrafficRow(db, mac)
	if err != nil {
		return 0, 0, err
	}
	deltaDown = counterDelta(traffic.LastDownloadCounter, downloadCounter)
	deltaUp = counterDelta(traffic.LastUploadCounter, uploadCounter)
	_, err = db.Exec(`
		UPDATE hotspot_device_traffic
		SET download_bytes = download_bytes + $2,
		    upload_bytes = upload_bytes + $3,
		    last_download_counter = $4,
		    last_upload_counter = $5,
		    updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1
	`, mac, deltaDown, deltaUp, int64(downloadCounter), int64(uploadCounter))
	if err != nil {
		return 0, 0, err
	}
	return deltaDown, deltaUp, nil
}

func recordGlobalUsage(db *sql.DB, downloadCounter, uploadCounter uint64) error {
	traffic, err := ensureGlobalTrafficRow(db)
	if err != nil {
		return err
	}
	deltaDown := counterDelta(traffic.LastDownloadCounter, downloadCounter)
	deltaUp := counterDelta(traffic.LastUploadCounter, uploadCounter)
	_, err = db.Exec(`
		UPDATE hotspot_global_traffic
		SET download_bytes = download_bytes + $1,
		    upload_bytes = upload_bytes + $2,
		    last_download_counter = $3,
		    last_upload_counter = $4,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = 'global'
	`, deltaDown, deltaUp, int64(downloadCounter), int64(uploadCounter))
	return err
}

// periodInterval mapeia quota_period para um literal de intervalo
// Postgres fixo (whitelist, nunca interpola texto arbitrario do
// usuario) usado para calcular o proximo period_end.
func periodInterval(period string) string {
	switch period {
	case "weekly":
		return "7 days"
	case "monthly":
		return "30 days"
	default:
		return "1 day"
	}
}

func resetDevicePeriodIfExpired(db *sql.DB, mac, quotaPeriod string) error {
	_, err := db.Exec(`
		UPDATE hotspot_device_traffic
		SET download_bytes = 0, upload_bytes = 0, throttled = false,
		    period_start = CURRENT_TIMESTAMP,
		    period_end = CURRENT_TIMESTAMP + interval '`+periodInterval(quotaPeriod)+`'
		WHERE mac_address = $1 AND period_end <= CURRENT_TIMESTAMP
	`, mac)
	return err
}

func resetGlobalPeriodIfExpired(db *sql.DB, quotaPeriod string) error {
	_, err := db.Exec(`
		UPDATE hotspot_global_traffic
		SET download_bytes = 0, upload_bytes = 0, throttled = false,
		    period_start = CURRENT_TIMESTAMP,
		    period_end = CURRENT_TIMESTAMP + interval '` + periodInterval(quotaPeriod) + `'
		WHERE id = 'global' AND period_end <= CURRENT_TIMESTAMP
	`)
	return err
}

func setDeviceThrottled(db *sql.DB, mac string, throttled bool) error {
	_, err := db.Exec(`UPDATE hotspot_device_traffic SET throttled = $2, updated_at = CURRENT_TIMESTAMP WHERE mac_address = $1`, mac, throttled)
	return err
}

func setGlobalThrottled(db *sql.DB, throttled bool) error {
	_, err := db.Exec(`UPDATE hotspot_global_traffic SET throttled = $1, updated_at = CURRENT_TIMESTAMP WHERE id = 'global'`, throttled)
	return err
}

func deviceQuotaExceeded(limits hotspotLimits, traffic hotspotDeviceTraffic) bool {
	if limits.QuotaBytes == nil {
		return false
	}
	return traffic.DownloadBytes+traffic.UploadBytes >= *limits.QuotaBytes
}

func globalQuotaExceeded(limits hotspotLimits, traffic hotspotGlobalTraffic) bool {
	if limits.QuotaBytes == nil {
		return false
	}
	return traffic.DownloadBytes+traffic.UploadBytes >= *limits.QuotaBytes
}
