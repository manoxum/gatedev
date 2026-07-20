package store

import "database/sql"

// BlockReason distingue POR QUE um dispositivo esta bloqueado agora -
// as 3 fontes sao independentes (bloqueio manual do admin, credito
// esgotado, cota esgotada) mas todas acabam chamando o mesmo primitivo
// de bloqueio ao vivo (applyLiveTrafficBlock) - sem essa distincao aqui
// a listagem so tinha um "blocked" generico, indistinguivel de um
// bloqueio manual. O bloqueio manual em si tem 2 variantes (ver "mode"
// em hotspot_blocklist.go): "deauth" derruba do Wi-Fi
// (BlockReasonManual) e "traffic" so corta o trafego, dispositivo
// continua associado (BlockReasonManualTraffic) - sem distinguir aqui
// as duas apareciam como o mesmo "Bloqueado" pro admin, mesmo o
// dispositivo continuando conectado no modo "traffic". Prioridade
// quando mais de uma fonte esta ativa ao mesmo tempo: manual > credit >
// quota (a acao explicita do admin e a mais "intencional" das tres,
// prevalece na exibicao).
type BlockReason = string

const (
	BlockReasonNone          BlockReason = ""
	BlockReasonManual        BlockReason = "manual"
	BlockReasonManualTraffic BlockReason = "manual_traffic"
	BlockReasonCredit        BlockReason = "credit"
	BlockReasonQuota         BlockReason = "quota"
)

// DeviceBlockReason resolve a fonte de bloqueio de um MAC entre as
// possiveis, com prioridade manual > credit > quota quando mais de uma
// esta ativa ao mesmo tempo (ver comentario em BlockReason acima).
// manualModes mapeia MAC -> "mode" do bloqueio manual (ver
// hotspotBlockedSet).
func DeviceBlockReason(mac string, manualModes map[string]string, credit, quota map[string]bool) BlockReason {
	if mode, ok := manualModes[mac]; ok {
		if mode == "traffic" {
			return BlockReasonManualTraffic
		}
		return BlockReasonManual
	}
	if credit[mac] {
		return BlockReasonCredit
	}
	if quota[mac] {
		return BlockReasonQuota
	}
	return BlockReasonNone
}

// HotspotQuotaBlockedSet devolve o conjunto de MACs com pelo menos um
// periodo de cota (diario/semanal/mensal) bloqueado agora - equivalente
// de hotspotCreditBlockedSet (hotspot_credit.go) para a fonte de
// bloqueio "quota esgotada" (ver SetDeviceQuotaPeriodBlocked em
// hotspot_device_quota_store.go).
func HotspotQuotaBlockedSet(db *sql.DB) (map[string]bool, error) {
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
