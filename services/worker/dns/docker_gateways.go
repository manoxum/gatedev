package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// discoverDockerGateways devolve os IPv4 atribuidos as bridges Docker do
// host. O dns-provider roda em network_mode: host, entao essas interfaces
// aparecem no mesmo namespace de rede em que ele precisa abrir os sockets.
func discoverDockerGateways(exclude ...string) ([]string, error) {
	excluded := map[string]bool{}
	for _, ip := range exclude {
		if ip != "" {
			excluded[ip] = true
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var gateways []string
	for _, iface := range ifaces {
		if !isDockerBridgeName(iface.Name) || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipv4FromAddr(addr)
			if ip == "" || excluded[ip] || seen[ip] {
				continue
			}
			seen[ip] = true
			gateways = append(gateways, ip)
		}
	}
	sort.Strings(gateways)
	return gateways, nil
}

func discoverHostSourceIPs(raw string, exclude ...string) ([]string, error) {
	excluded := map[string]bool{}
	for _, ip := range exclude {
		if ip != "" {
			excluded[ip] = true
		}
	}
	if strings.TrimSpace(raw) == "" {
		return discoverLANIPs(excluded)
	}

	seen := map[string]bool{}
	var ips []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		ip, err := parseHostSourceIP(value)
		if err != nil {
			return nil, err
		}
		if ip == "" || excluded[ip] || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips, nil
}

func discoverLANIPs(excluded map[string]bool) ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ips []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || isIgnoredLANInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipv4FromAddr(addr)
			if ip == "" || excluded[ip] || seen[ip] {
				continue
			}
			seen[ip] = true
			ips = append(ips, ip)
		}
	}
	sort.Strings(ips)
	return ips, nil
}

func parseHostSourceIP(value string) (string, error) {
	if strings.Contains(value, "/") {
		ip, _, err := net.ParseCIDR(value)
		if err != nil {
			return "", fmt.Errorf("HOST_SOURCE_CIDR invalido: %s", value)
		}
		ip = ip.To4()
		if ip == nil || ip.IsLoopback() {
			return "", fmt.Errorf("HOST_SOURCE_CIDR deve conter IPv4 nao-loopback: %s", value)
		}
		return ip.String(), nil
	}
	ip := net.ParseIP(value).To4()
	if ip == nil || ip.IsLoopback() {
		return "", fmt.Errorf("HOST_SOURCE_CIDR deve conter IPv4 nao-loopback: %s", value)
	}
	return ip.String(), nil
}

func isDockerBridgeName(name string) bool {
	return name == "docker0" || strings.HasPrefix(name, "br-")
}

func isIgnoredLANInterface(name string) bool {
	if name == "lo" || name == "ap0" || isDockerBridgeName(name) {
		return true
	}
	for _, prefix := range []string{"veth", "virbr", "tun", "tap", "wg", "bn-"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func ipv4FromAddr(addr net.Addr) string {
	var ip net.IP
	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return ""
	}
	ip = ip.To4()
	if ip == nil || ip.IsLoopback() {
		return ""
	}
	return ip.String()
}
