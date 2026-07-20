package hotspot

import (
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/workerapi"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type hotspotBlockedDevice struct {
	MACAddress string    `json:"macAddress"`
	Note       string    `json:"note,omitempty"`
	Mode       string    `json:"mode"`
	BlockedAt  time.Time `json:"blockedAt"`
}

type hotspotBlockRequest struct {
	MAC  string `json:"mac"`
	Note string `json:"note,omitempty"`
	// Mode: "deauth" (padrao) derruba o dispositivo do Wi-Fi (hostapd);
	// "traffic" so bloqueia o trafego via iptables, dispositivo continua
	// conectado. Ver applyLiveBlockForMode.
	Mode string `json:"mode,omitempty"`
}

func RegisterHotspotBlocklistRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, worker *workerapi.Client) {
	mux.HandleFunc("GET /api/hotspot/blocklist", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		devices, err := listHotspotBlockedDevices(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(devices)
	}))

	mux.HandleFunc("POST /api/hotspot/blocklist", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req hotspotBlockRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		mac, err := normalizeHotspotMAC(req.MAC)
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		mode := strings.TrimSpace(req.Mode)
		if mode == "" {
			mode = "deauth"
		}
		if mode != "deauth" && mode != "traffic" {
			http.Error(w, "campo 'mode' deve ser 'deauth' ou 'traffic'", http.StatusBadRequest)
			return
		}
		device, err := upsertHotspotBlockedDevice(db, mac, strings.TrimSpace(req.Note), mode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyLiveBlockForMode(r.Context(), db, worker, mac, mode, true)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(device)
	}))

	mux.HandleFunc("DELETE /api/hotspot/blocklist/{mac}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		mode, found, err := getHotspotBlockedDeviceMode(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := deleteHotspotBlockedDevice(db, mac); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if found {
			applyLiveBlockForMode(r.Context(), db, worker, mac, mode, false)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// hotspotBlockedSet devolve o conjunto de MACs bloqueados manualmente
// pelo admin, mapeado para o "mode" do bloqueio ("deauth" ou
// "traffic") - usado por store.DeviceBlockReason para distinguir bloqueio
// completo (desconectado do Wi-Fi) de corte so de trafego (dispositivo
// continua associado).
func hotspotBlockedSet(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT mac_address, mode FROM hotspot_blocked_devices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocked := map[string]string{}
	for rows.Next() {
		var mac, mode string
		if err := rows.Scan(&mac, &mode); err != nil {
			return nil, err
		}
		blocked[mac] = mode
	}
	return blocked, rows.Err()
}

func listHotspotBlockedDevices(db *sql.DB) ([]hotspotBlockedDevice, error) {
	rows, err := db.Query(`
		SELECT mac_address, COALESCE(note, ''), mode, blocked_at
		FROM hotspot_blocked_devices
		ORDER BY blocked_at DESC, mac_address
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := []hotspotBlockedDevice{}
	for rows.Next() {
		var device hotspotBlockedDevice
		if err := rows.Scan(&device.MACAddress, &device.Note, &device.Mode, &device.BlockedAt); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func upsertHotspotBlockedDevice(db *sql.DB, mac, note, mode string) (hotspotBlockedDevice, error) {
	var device hotspotBlockedDevice
	err := db.QueryRow(`
		INSERT INTO hotspot_blocked_devices (mac_address, note, mode, blocked_at)
		VALUES ($1, NULLIF($2, ''), $3, CURRENT_TIMESTAMP)
		ON CONFLICT (mac_address) DO UPDATE
		SET note = EXCLUDED.note,
		    mode = EXCLUDED.mode,
		    blocked_at = CURRENT_TIMESTAMP
		RETURNING mac_address, COALESCE(note, ''), mode, blocked_at
	`, mac, note, mode).Scan(&device.MACAddress, &device.Note, &device.Mode, &device.BlockedAt)
	return device, err
}

func getHotspotBlockedDeviceMode(db *sql.DB, mac string) (mode string, found bool, err error) {
	err = db.QueryRow(`SELECT mode FROM hotspot_blocked_devices WHERE mac_address = $1`, mac).Scan(&mode)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return mode, true, nil
}

func deleteHotspotBlockedDevice(db *sql.DB, mac string) error {
	_, err := db.Exec(`DELETE FROM hotspot_blocked_devices WHERE mac_address = $1`, mac)
	return err
}
