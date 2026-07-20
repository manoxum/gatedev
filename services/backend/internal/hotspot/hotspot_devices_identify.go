package hotspot

import (
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func identifyHotspotClient(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac, hostname string) (hotspotDeviceInfo, error) {
	if cached, found, err := hotspotDeviceInfoByMAC(db, mac); err != nil {
		return hotspotDeviceInfo{}, err
	} else if found {
		return cached, nil
	}

	var fingerprint hotspotFingerprintResponse
	if err := worker.Call(ctx, http.MethodGet, "/hotspot/fingerprint?mac="+url.QueryEscape(mac), nil, &fingerprint); err != nil {
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

func liveHotspotHostname(r *http.Request, db *sql.DB, worker *workerapi.Client, mac string) string {
	iface, err := currentHotspotInterface(r.Context(), db)
	if err != nil {
		return ""
	}
	var clients []workerHotspotClient
	err = worker.Call(r.Context(), http.MethodGet, "/hotspot/clients?interface="+url.QueryEscape(iface), nil, &clients)
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
