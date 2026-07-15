// hotspot_device_history.go expoe a listagem de dispositivos que ja se
// conectaram ao hotspot alguma vez (diferente da lista de "conectados
// agora" em hotspot_devices.go e da de "bloqueados" em
// hotspot_blocklist.go) - alimentada por recordDeviceSeen a cada vez
// que um MAC aparece nos clientes ao vivo.
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

type hotspotKnownDevice struct {
	MACAddress  string
	Vendor      sql.NullString
	DeviceName  sql.NullString
	OSName      sql.NullString
	Alias       sql.NullString
	FirstSeenAt sql.NullTime
	LastSeenAt  sql.NullTime
}

type hotspotKnownDeviceResponse struct {
	MAC         string      `json:"mac"`
	Vendor      string      `json:"vendor,omitempty"`
	DeviceName  string      `json:"deviceName,omitempty"`
	OSName      string      `json:"osName,omitempty"`
	Alias       string      `json:"alias,omitempty"`
	FirstSeenAt string      `json:"firstSeenAt,omitempty"`
	LastSeenAt  string      `json:"lastSeenAt,omitempty"`
	Connected   bool        `json:"connected"`
	Blocked     bool        `json:"blocked"`
	BlockReason blockReason `json:"blockReason,omitempty"`
}

func registerHotspotDeviceHistoryRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, worker *workerClient) {
	mux.HandleFunc("GET /api/hotspot/devices/known", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		devices, err := listKnownHotspotDevices(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		connected := map[string]bool{}
		if iface, err := currentHotspotInterface(r.Context(), db); err == nil {
			if live, err := liveHotspotClients(r.Context(), worker, iface); err == nil {
				for _, client := range live {
					connected[client.MAC] = true
				}
			}
		}

		// Mesmas 3 fontes de bloqueio + resolucao de prioridade usadas em
		// listEnrichedHotspotClients (hotspot_devices.go) - sem isso, um
		// dispositivo bloqueado por credito/cota mas desconectado no
		// momento aparecia como "so desconectado" pro frontend, ja que
		// blockReason so vinha resolvido em /api/hotspot/clients (que so
		// lista quem esta conectado agora).
		blocked, err := hotspotBlockedSet(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		blockedByCredit, err := hotspotCreditBlockedSet(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		blockedByQuota, err := hotspotQuotaBlockedSet(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := make([]hotspotKnownDeviceResponse, 0, len(devices))
		for _, device := range devices {
			reason := deviceBlockReason(device.MACAddress, blocked, blockedByCredit, blockedByQuota)
			response = append(response, knownDeviceResponse(device, connected[device.MACAddress], reason))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}

func knownDeviceResponse(device hotspotKnownDevice, connected bool, reason blockReason) hotspotKnownDeviceResponse {
	response := hotspotKnownDeviceResponse{
		MAC:         device.MACAddress,
		Connected:   connected,
		Blocked:     reason != blockReasonNone,
		BlockReason: reason,
	}
	if device.Vendor.Valid {
		response.Vendor = device.Vendor.String
	}
	if device.DeviceName.Valid {
		response.DeviceName = device.DeviceName.String
	}
	if device.OSName.Valid {
		response.OSName = device.OSName.String
	}
	if device.Alias.Valid {
		response.Alias = device.Alias.String
	}
	if device.FirstSeenAt.Valid {
		response.FirstSeenAt = device.FirstSeenAt.Time.Format(time.RFC3339)
	}
	if device.LastSeenAt.Valid {
		response.LastSeenAt = device.LastSeenAt.Time.Format(time.RFC3339)
	}
	return response
}
