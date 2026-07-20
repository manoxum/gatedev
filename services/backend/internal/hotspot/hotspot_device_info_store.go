package hotspot

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// errHotspotAliasTaken e devolvido por updateHotspotDeviceIdentity
// quando o alias ja esta em uso por outro dispositivo (violacao da
// constraint UNIQUE em hotspot_device_info.alias) - o handler HTTP
// traduz isso pra 409 Conflict.
var errHotspotAliasTaken = errors.New("alias ja esta em uso por outro dispositivo")

func hotspotDeviceInfoMap(db *sql.DB) (map[string]hotspotDeviceInfo, error) {
	rows, err := db.Query(`
		SELECT mac_address, vendor, device_name, os_name, confidence, alias
		FROM hotspot_device_info
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	infos := map[string]hotspotDeviceInfo{}
	for rows.Next() {
		var info hotspotDeviceInfo
		if err := rows.Scan(&info.MACAddress, &info.Vendor, &info.DeviceName, &info.OSName, &info.Confidence, &info.Alias); err != nil {
			return nil, err
		}
		infos[info.MACAddress] = info
	}
	return infos, rows.Err()
}

func hotspotDeviceInfoByMAC(db *sql.DB, mac string) (hotspotDeviceInfo, bool, error) {
	var info hotspotDeviceInfo
	err := db.QueryRow(`
		SELECT mac_address, vendor, device_name, os_name, confidence, alias
		FROM hotspot_device_info
		WHERE mac_address = $1
	`, mac).Scan(&info.MACAddress, &info.Vendor, &info.DeviceName, &info.OSName, &info.Confidence, &info.Alias)
	if err == nil {
		return info, hotspotDeviceInfoHasData(info), nil
	}
	if err == sql.ErrNoRows {
		return hotspotDeviceInfo{}, false, nil
	}
	return hotspotDeviceInfo{}, false, err
}

// hotspotIdentityEdit e um patch parcial dos campos editaveis a mao
// pelo admin (modal "Identificar" no frontend) - ponteiro nil = campo
// omitido no corpo do PATCH, mantem o valor atual; ponteiro apontando
// para "" = limpa o campo explicitamente. Ver updateHotspotDeviceIdentity.
type hotspotIdentityEdit struct {
	Alias      *string
	Vendor     *string
	DeviceName *string
	OSName     *string
}

func mergeNullableString(current sql.NullString, override *string) sql.NullString {
	if override == nil {
		return current
	}
	return sql.NullString{String: *override, Valid: *override != ""}
}

// updateHotspotDeviceIdentity grava edicoes manuais de alias/vendor/
// deviceName/osName - so mexe nos campos presentes em edit (os demais
// preservam o valor atual, lido primeiro do banco), ao contrario de
// upsertHotspotDeviceInfo (fluxo automatico de "Buscar
// automaticamente", que sempre substitui os 3 campos + confidence
// juntos). Edicao manual marca confidence=100 quando vendor/deviceName/
// osName ficam preenchidos (sinaliza "definido a mao", nao heuristica).
func updateHotspotDeviceIdentity(db *sql.DB, mac string, edit hotspotIdentityEdit) error {
	current, _, err := hotspotDeviceInfoByMAC(db, mac)
	if err != nil {
		return err
	}

	alias := mergeNullableString(current.Alias, edit.Alias)
	vendor := mergeNullableString(current.Vendor, edit.Vendor)
	deviceName := mergeNullableString(current.DeviceName, edit.DeviceName)
	osName := mergeNullableString(current.OSName, edit.OSName)

	confidence := current.Confidence
	if edit.Vendor != nil || edit.DeviceName != nil || edit.OSName != nil {
		confidence = sql.NullInt64{}
		if vendor.Valid || deviceName.Valid || osName.Valid {
			confidence = sql.NullInt64{Int64: 100, Valid: true}
		}
	}

	_, err = db.Exec(`
		INSERT INTO hotspot_device_info (mac_address, alias, vendor, device_name, os_name, confidence, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET alias = EXCLUDED.alias,
		    vendor = EXCLUDED.vendor,
		    device_name = EXCLUDED.device_name,
		    os_name = EXCLUDED.os_name,
		    confidence = EXCLUDED.confidence,
		    fetched_at = EXCLUDED.fetched_at
	`, mac, nullableString(alias), nullableString(vendor), nullableString(deviceName), nullableString(osName), nullableInt(confidence))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errHotspotAliasTaken
		}
		return err
	}
	return nil
}

func hotspotDeviceInfoHasData(info hotspotDeviceInfo) bool {
	return (info.Vendor.Valid && info.Vendor.String != "") ||
		(info.DeviceName.Valid && info.DeviceName.String != "") ||
		(info.OSName.Valid && info.OSName.String != "") ||
		info.Confidence.Valid
}

func upsertHotspotDeviceInfo(db *sql.DB, info hotspotDeviceInfo) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_device_info (mac_address, vendor, device_name, os_name, confidence, fetched_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET vendor = EXCLUDED.vendor,
		    device_name = EXCLUDED.device_name,
		    os_name = EXCLUDED.os_name,
		    confidence = EXCLUDED.confidence,
		    fetched_at = CURRENT_TIMESTAMP
	`, info.MACAddress, nullableString(info.Vendor), nullableString(info.DeviceName), nullableString(info.OSName), nullableInt(info.Confidence))
	return err
}

// recordDeviceSeen marca que o MAC apareceu na lista de clientes ao
// vivo agora - chamado a cada listagem de clientes
// (listEnrichedHotspotClients). first_seen_at so e gravado na
// primeira vez (INSERT); conflitos so atualizam last_seen_at, nunca
// mexem em vendor/device_name/os_name/alias/confidence (campos
// exclusivos dos fluxos de identificacao/edicao manual).
func recordDeviceSeen(db *sql.DB, mac string) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_device_info (mac_address, first_seen_at, last_seen_at)
		VALUES ($1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE SET last_seen_at = CURRENT_TIMESTAMP
	`, mac)
	return err
}

// listKnownHotspotDevices devolve todo dispositivo ja visto conectado
// alguma vez (first_seen_at nao nulo - exclui linhas criadas so por
// identificacao/alias manual num MAC que nunca chegou a conectar).
func listKnownHotspotDevices(db *sql.DB) ([]hotspotKnownDevice, error) {
	rows, err := db.Query(`
		SELECT mac_address, vendor, device_name, os_name, alias, first_seen_at, last_seen_at
		FROM hotspot_device_info
		WHERE first_seen_at IS NOT NULL
		ORDER BY last_seen_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := []hotspotKnownDevice{}
	for rows.Next() {
		var device hotspotKnownDevice
		if err := rows.Scan(&device.MACAddress, &device.Vendor, &device.DeviceName, &device.OSName,
			&device.Alias, &device.FirstSeenAt, &device.LastSeenAt); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullableInt(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
