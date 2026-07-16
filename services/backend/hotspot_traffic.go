package main

import (
	"database/sql"
	"time"
)

// hotspotDeviceTraffic guarda so o que e genuinamente singular por
// dispositivo: o fwmark (classe HTB/marca iptables dedicada) e os
// contadores absolutos usados para calcular o delta a cada ciclo de
// reconciliacao. O acumulado por periodo de cota (antes period_start/
// period_end/download_bytes/upload_bytes/throttled aqui) mudou pra uma
// linha por (mac, tipo de periodo) em hotspot_device_quota_periods -
// ver hotspot_device_quota_store.go e o comentario no schema.prisma.
type hotspotDeviceTraffic struct {
	MACAddress          string
	Fwmark              int
	LastDownloadCounter int64
	LastUploadCounter   int64
}

type hotspotGlobalTraffic struct {
	PeriodStart         time.Time
	PeriodEnd           time.Time
	DownloadBytes       int64
	UploadBytes         int64
	LastDownloadCounter int64
	LastUploadCounter   int64
	Throttled           bool
}

// getOrCreateDeviceFwmark garante que o dispositivo tenha uma linha em
// hotspot_device_traffic (criada de forma preguicosa, independente de
// limite configurado) e devolve o fwmark atribuido pela sequence -
// nunca hash de MAC, evita colisao.
func getOrCreateDeviceFwmark(db *sql.DB, mac string) (int, error) {
	traffic, err := ensureDeviceTrafficRow(db, mac)
	if err != nil {
		return 0, err
	}
	return traffic.Fwmark, nil
}

// ensureDeviceTrafficRow le a linha existente do dispositivo e so cai
// pro INSERT quando ela realmente nao existe ainda - nunca usa
// "INSERT ... ON CONFLICT DO UPDATE/DO NOTHING" aqui, porque o
// Postgres avalia o DEFAULT da coluna fwmark (nextval da sequence,
// ver hotspot_device_fwmark_seq) ao montar a linha candidata ANTES de
// checar o conflito - ou seja, mesmo um upsert que so atualiza (ou um
// "DO NOTHING") consome e descarta um valor da sequence a cada
// chamada. Essa funcao roda a cada 1s por dispositivo conectado (ver
// reconcileDeviceUsage) e a cada 15s no loop de reconciliacao, entao
// o desperdicio inflava o fwmark rapido o bastante pra estourar os 16
// bits que o classid HTB do tc aceita (ver deviceClassID em
// services/worker/controller/shaping_tc.go), quebrando a classe HTB
// dedicada (e o limite de banda) de qualquer dispositivo visto depois
// disso. Fazendo o SELECT primeiro, o INSERT (que ainda tem o
// ON CONFLICT DO UPDATE como rede de seguranca pra corrida entre
// goroutines) so roda uma vez por MAC genuinamente novo.
func ensureDeviceTrafficRow(db *sql.DB, mac string) (hotspotDeviceTraffic, error) {
	var t hotspotDeviceTraffic
	err := db.QueryRow(`
		SELECT mac_address, fwmark, last_download_counter, last_upload_counter
		FROM hotspot_device_traffic WHERE mac_address = $1
	`, mac).Scan(&t.MACAddress, &t.Fwmark, &t.LastDownloadCounter, &t.LastUploadCounter)
	if err == nil {
		return t, nil
	}
	if err != sql.ErrNoRows {
		return t, err
	}
	err = db.QueryRow(`
		INSERT INTO hotspot_device_traffic (mac_address)
		VALUES ($1)
		ON CONFLICT (mac_address) DO UPDATE SET mac_address = EXCLUDED.mac_address
		RETURNING mac_address, fwmark, last_download_counter, last_upload_counter
	`, mac).Scan(&t.MACAddress, &t.Fwmark, &t.LastDownloadCounter, &t.LastUploadCounter)
	return t, err
}

func ensureGlobalTrafficRow(db *sql.DB) (hotspotGlobalTraffic, error) {
	var t hotspotGlobalTraffic
	err := db.QueryRow(`
		INSERT INTO hotspot_global_traffic (id)
		VALUES ('global')
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING period_start, period_end, download_bytes, upload_bytes,
		          last_download_counter, last_upload_counter, throttled
	`).Scan(&t.PeriodStart, &t.PeriodEnd, &t.DownloadBytes, &t.UploadBytes,
		&t.LastDownloadCounter, &t.LastUploadCounter, &t.Throttled)
	return t, err
}
