package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type workerHotspotClient struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

type hotspotClientResponse struct {
	MAC        string `json:"mac"`
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	Vendor     string `json:"vendor,omitempty"`
	DeviceName string `json:"deviceName,omitempty"`
	OSName     string `json:"osName,omitempty"`
	Confidence int    `json:"confidence,omitempty"`
	Blocked    bool   `json:"blocked"`
}

type hotspotDeviceInfo struct {
	MACAddress string
	Vendor     sql.NullString
	DeviceName sql.NullString
	OSName     sql.NullString
	Confidence sql.NullInt64
}

type hotspotFingerprintResponse struct {
	DHCPFingerprint string `json:"dhcpFingerprint,omitempty"`
	DHCPVendor      string `json:"dhcpVendor,omitempty"`
}

func registerHotspotDeviceRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, worker *workerClient) {
	mux.HandleFunc("POST /api/hotspot/clients/{mac}/identify", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		hostname := liveHotspotHostname(r, worker, mac)
		info, err := identifyHotspotClient(r.Context(), db, worker, mac, hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(infoToClientFields(mac, info, false))
	}))
}

func listEnrichedHotspotClients(r *http.Request, db *sql.DB, worker *workerClient) ([]hotspotClientResponse, error) {
	iface, err := currentHotspotInterface(r, worker)
	if err != nil {
		return nil, err
	}
	var live []workerHotspotClient
	if err := worker.call(r.Context(), http.MethodGet, "/hotspot/clients?interface="+url.QueryEscape(iface), nil, &live); err != nil {
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

	clients := make([]hotspotClientResponse, 0, len(live))
	for _, client := range live {
		mac, err := normalizeHotspotMAC(client.MAC)
		if err != nil {
			mac = strings.ToLower(strings.TrimSpace(client.MAC))
		}
		enriched := infoToClientFields(mac, info[mac], blocked[mac] || blockedByCredit[mac])
		enriched.IP = client.IP
		enriched.Hostname = client.Hostname
		clients = append(clients, enriched)
	}
	return clients, nil
}

func infoToClientFields(mac string, info hotspotDeviceInfo, blocked bool) hotspotClientResponse {
	client := hotspotClientResponse{MAC: mac, Blocked: blocked}
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
	return client
}

func identifyHotspotClient(ctx context.Context, db *sql.DB, worker *workerClient, mac, hostname string) (hotspotDeviceInfo, error) {
	if cached, found, err := hotspotDeviceInfoByMAC(db, mac); err != nil {
		return hotspotDeviceInfo{}, err
	} else if found {
		return cached, nil
	}

	var fingerprint hotspotFingerprintResponse
	if err := worker.call(ctx, http.MethodGet, "/hotspot/fingerprint?mac="+url.QueryEscape(mac), nil, &fingerprint); err != nil {
		return hotspotDeviceInfo{}, err
	}

	vendor, vendorErr := lookupMACVendor(ctx, mac)
	profile := inferHotspotDeviceProfile(vendor, hostname, fingerprint)
	info := hotspotDeviceInfo{
		MACAddress: mac,
		Vendor:     sql.NullString{String: vendor, Valid: vendor != ""},
		DeviceName: sql.NullString{String: profile.DeviceName, Valid: profile.DeviceName != ""},
		OSName:     sql.NullString{String: profile.OSName, Valid: profile.OSName != ""},
		Confidence: sql.NullInt64{Int64: int64(profile.Confidence), Valid: profile.Confidence > 0},
	}
	if !hotspotDeviceInfoHasData(info) {
		if vendorErr != nil {
			return hotspotDeviceInfo{}, vendorErr
		}
		return hotspotDeviceInfo{}, fmt.Errorf("nao foi possivel identificar o fabricante localmente para %s", mac)
	}
	if err := upsertHotspotDeviceInfo(db, info); err != nil {
		return hotspotDeviceInfo{}, err
	}
	return info, nil
}

func liveHotspotHostname(r *http.Request, worker *workerClient, mac string) string {
	iface, err := currentHotspotInterface(r, worker)
	if err != nil {
		return ""
	}
	var clients []workerHotspotClient
	err = worker.call(r.Context(), http.MethodGet, "/hotspot/clients?interface="+url.QueryEscape(iface), nil, &clients)
	if err != nil {
		return ""
	}
	for _, client := range clients {
		normalized, err := normalizeHotspotMAC(client.MAC)
		if err == nil && normalized == mac {
			return client.Hostname
		}
	}
	return ""
}

func normalizeHotspotMAC(raw string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return strings.ToLower(hw.String()), nil
}
