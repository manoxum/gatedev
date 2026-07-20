package hotspot

import (
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/workerapi"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type workerHotspotClient struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	SignalDBM *int   `json:"signalDbm,omitempty"`
}

type hotspotClientResponse struct {
	MAC         string      `json:"mac"`
	IP          string      `json:"ip"`
	Hostname    string      `json:"hostname"`
	Vendor      string      `json:"vendor,omitempty"`
	DeviceName  string      `json:"deviceName,omitempty"`
	OSName      string      `json:"osName,omitempty"`
	Confidence  int         `json:"confidence,omitempty"`
	Alias       string      `json:"alias,omitempty"`
	Blocked     bool        `json:"blocked"`
	BlockReason blockReason `json:"blockReason,omitempty"`
	ProfileID   string      `json:"profileId,omitempty"`
	ProfileName string      `json:"profileName,omitempty"`
	SignalDBM   *int        `json:"signalDbm,omitempty"`
}

type hotspotDeviceInfo struct {
	MACAddress string
	Vendor     sql.NullString
	DeviceName sql.NullString
	OSName     sql.NullString
	Confidence sql.NullInt64
	Alias      sql.NullString
}

type hotspotFingerprintResponse struct {
	DHCPFingerprint string `json:"dhcpFingerprint,omitempty"`
	DHCPVendor      string `json:"dhcpVendor,omitempty"`
}

func RegisterHotspotDeviceRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, worker *workerapi.Client) {
	mux.HandleFunc("POST /api/hotspot/clients/{mac}/identify", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		hostname := liveHotspotHostname(r, db, worker, mac)
		info, err := identifyHotspotClient(r.Context(), db, worker, mac, hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(infoToClientFields(mac, info, blockReasonNone))
	}))

	RegisterHotspotDeviceIdentityRoute(mux, admin, db)
}

func listEnrichedHotspotClients(r *http.Request, db *sql.DB, worker *workerapi.Client) ([]hotspotClientResponse, error) {
	iface, err := currentHotspotInterface(r.Context(), db)
	if err != nil {
		return nil, err
	}
	var live []workerHotspotClient
	if err := worker.Call(r.Context(), http.MethodGet, "/hotspot/clients?interface="+url.QueryEscape(iface), nil, &live); err != nil {
		return nil, err
	}

	info, err := hotspotDeviceInfoMap(db)
	if err != nil {
		return nil, err
	}
	blocked, err := hotspotBlockedSet(db)
	if err != nil {
		return nil, err
	}
	blockedByCredit, err := hotspotCreditBlockedSet(db)
	if err != nil {
		return nil, err
	}
	blockedByQuota, err := hotspotQuotaBlockedSet(db)
	if err != nil {
		return nil, err
	}
	profiles, err := hotspotDeviceProfileRefs(db)
	if err != nil {
		return nil, err
	}

	clients := make([]hotspotClientResponse, 0, len(live))
	for _, client := range live {
		mac, err := normalizeHotspotMAC(client.MAC)
		if err != nil {
			mac = strings.ToLower(strings.TrimSpace(client.MAC))
		}
		if err := recordDeviceSeen(db, mac); err != nil {
			log.Printf("[backend] falha ao registrar %s como visto: %v", mac, err)
		}
		enriched := infoToClientFields(mac, info[mac], deviceBlockReason(mac, blocked, blockedByCredit, blockedByQuota))
		enriched.IP = client.IP
		enriched.Hostname = client.Hostname
		enriched.SignalDBM = client.SignalDBM
		if ref, found := profiles[mac]; found {
			enriched.ProfileID = ref.ID
			enriched.ProfileName = ref.Name
		}
		clients = append(clients, enriched)
	}
	return clients, nil
}

func infoToClientFields(mac string, info hotspotDeviceInfo, reason blockReason) hotspotClientResponse {
	client := hotspotClientResponse{MAC: mac, Blocked: reason != blockReasonNone, BlockReason: reason}
	if info.Vendor.Valid {
		client.Vendor = info.Vendor.String
	}
	if info.DeviceName.Valid {
		client.DeviceName = info.DeviceName.String
	}
	if info.OSName.Valid {
		client.OSName = info.OSName.String
	}
	if info.Confidence.Valid {
		client.Confidence = int(info.Confidence.Int64)
	}
	if info.Alias.Valid {
		client.Alias = info.Alias.String
	}
	return client
}
