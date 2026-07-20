package hotspot

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

// recordDeviceUsage soma o delta desde a ultima leitura aos contadores
// absolutos e devolve os proprios deltas - o loop de reconciliacao usa
// esse retorno para debitar o trafego deste ciclo do saldo de credito
// e/ou incrementar os acumuladores por periodo de cota (ver
// hotspot_device_quota_store.go), sem reler nada mais. Nao acumula
// download_bytes/upload_bytes aqui (colunas legadas, sem leitor Go daqui
// pra frente - ver comentario em hotspot_device_traffic no schema.prisma).
func recordDeviceUsage(db *sql.DB, mac string, downloadCounter, uploadCounter uint64) (deltaDown, deltaUp int64, err error) {
	traffic, err := ensureDeviceTrafficRow(db, mac)
	if err != nil {
		return 0, 0, err
	}
	deltaDown = counterDelta(traffic.LastDownloadCounter, downloadCounter)
	deltaUp = counterDelta(traffic.LastUploadCounter, uploadCounter)
	_, err = db.Exec(`
		UPDATE hotspot_device_traffic
		SET last_download_counter = $2,
		    last_upload_counter = $3,
		    updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1
	`, mac, int64(downloadCounter), int64(uploadCounter))
	if err != nil {
		return 0, 0, err
	}
	return deltaDown, deltaUp, nil
}

// recordGlobalUsage e o equivalente global de recordDeviceUsage -
// devolve os deltas usados pra alimentar o velocimetro/grafico geral
// (ver reconcileGlobalUsage em hotspot_usage_sampling.go). Ainda
// acumula download_bytes/upload_bytes por compatibilidade com a coluna
// existente, mas nada mais le esses dois campos (o limite/cota global
// foi removido - ver RULE.md).
func recordGlobalUsage(db *sql.DB, downloadCounter, uploadCounter uint64) (deltaDown, deltaUp int64, err error) {
	traffic, err := ensureGlobalTrafficRow(db)
	if err != nil {
		return 0, 0, err
	}
	deltaDown = counterDelta(traffic.LastDownloadCounter, downloadCounter)
	deltaUp = counterDelta(traffic.LastUploadCounter, uploadCounter)
	_, err = db.Exec(`
		UPDATE hotspot_global_traffic
		SET download_bytes = download_bytes + $1,
		    upload_bytes = upload_bytes + $2,
		    last_download_counter = $3,
		    last_upload_counter = $4,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = 'global'
	`, deltaDown, deltaUp, int64(downloadCounter), int64(uploadCounter))
	if err != nil {
		return 0, 0, err
	}
	return deltaDown, deltaUp, nil
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
