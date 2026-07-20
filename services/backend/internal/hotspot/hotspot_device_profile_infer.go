package hotspot

import (
	"strings"
)

type inferredHotspotProfile struct {
	DeviceName string
	OSName     string
	Confidence int
}

func inferHotspotDeviceProfile(vendor, hostname string, fingerprint hotspotFingerprintResponse) inferredHotspotProfile {
	lowerVendor := strings.ToLower(vendor)
	lowerHost := strings.ToLower(hostname)
	lowerDHCPVendor := strings.ToLower(fingerprint.DHCPVendor)
	options := "," + fingerprint.DHCPFingerprint + ","

	switch {
	case strings.Contains(lowerDHCPVendor, "android") || strings.Contains(lowerHost, "android"):
		return profile("Dispositivo Android", "Android", 76)
	case strings.Contains(lowerDHCPVendor, "msft") || strings.Contains(options, ",44,") && strings.Contains(options, ",46,"):
		return profile("Computador Windows", "Windows", 72)
	case strings.Contains(lowerHost, "iphone") || strings.Contains(lowerHost, "ipad"):
		return profile("Dispositivo iOS", "iOS", 78)
	case strings.Contains(lowerHost, "macbook") || strings.Contains(lowerHost, "imac") || strings.Contains(lowerHost, "mac-"):
		return profile("Computador macOS", "macOS", 74)
	case strings.Contains(lowerVendor, "apple"):
		return profile("Dispositivo Apple", "iOS/macOS", 56)
	case strings.Contains(lowerDHCPVendor, "udhcp") || strings.Contains(lowerDHCPVendor, "busybox"):
		return profile("Dispositivo embarcado", "Linux/embarcado", 62)
	case strings.Contains(lowerDHCPVendor, "dhcpcd"):
		return profile("Dispositivo Unix-like", "Linux/Unix", 52)
	case isLikelyIoTVendor(lowerVendor):
		return profile("Dispositivo IoT/rede", "Linux/embarcado", 48)
	case vendor != "":
		return profile("Dispositivo de "+vendor, "", 36)
	default:
		return inferredHotspotProfile{}
	}
}

func profile(deviceName, osName string, confidence int) inferredHotspotProfile {
	return inferredHotspotProfile{DeviceName: deviceName, OSName: osName, Confidence: confidence}
}

func isLikelyIoTVendor(vendor string) bool {
	for _, token := range []string{
		"espressif", "tuya", "tp-link", "tplink", "zte", "huawei", "ubiquiti",
		"amazon", "google", "roku", "ring", "hikvision", "dahua", "sonoff",
		"xiaomi", "philips lighting", "shelly",
	} {
		if strings.Contains(vendor, token) {
			return true
		}
	}
	return false
}
