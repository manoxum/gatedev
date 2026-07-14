package main

import "database/sql"

// blockReason distingue POR QUE um dispositivo esta bloqueado agora -
// as 3 fontes sao independentes (bloqueio manual do admin, credito
// esgotado, cota esgotada) mas todas acabam chamando o mesmo primitivo
// de bloqueio ao vivo (applyLiveTrafficBlock) - sem essa distincao aqui
// a listagem so tinha um "blocked" generico, indistinguivel de um
// bloqueio manual. Prioridade quando mais de uma fonte esta ativa ao
// mesmo tempo: manual > credit > quota (a acao explicita do admin e a
// mais "intencional" das tres, prevalece na exibicao).
type blockReason = string

const (
	blockReasonNone   blockReason = ""
	blockReasonManual blockReason = "manual"
	blockReasonCredit blockReason = "credit"
	blockReasonQuota  blockReason = "quota"
)

// deviceBlockReason resolve a fonte de bloqueio de um MAC entre as 3
// possiveis, com prioridade manual > credit > quota quando mais de uma
// esta ativa ao mesmo tempo (ver comentario em blockReason acima).
func deviceBlockReason(mac string, manual, credit, quota map[string]bool) blockReason {
	if manual[mac] {
		return blockReasonManual
	}
	if credit[mac] {
		return blockReasonCredit
	}
	if quota[mac] {
		return blockReasonQuota
	}
	return blockReasonNone
}

// hotspotQuotaBlockedSet devolve o conjunto de MACs com pelo menos um
// periodo de cota (diario/semanal/mensal) bloqueado agora - equivalente
// de hotspotCreditBlockedSet (hotspot_credit.go) para a fonte de
// bloqueio "quota esgotada" (ver setDeviceQuotaPeriodBlocked em
// hotspot_device_quota_store.go).
func hotspotQuotaBlockedSet(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT DISTINCT mac_address FROM hotspot_device_quota_periods WHERE blocked`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocked := map[string]bool{}
	for rows.Next() {
		var mac string
		if err := rows.Scan(&mac); err != nil {
			return nil, err
		}
		blocked[mac] = true
	}
	return blocked, rows.Err()
}
