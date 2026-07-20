package netdetect

import (
	"fmt"
	"net"
	"time"
)

// WaitForIPs bloqueia ate os IPs informados existirem como enderecos reais
// na maquina (equivalente ao loop do antigo docker-entrypoint.sh) - existe
// porque dns-provider depende do hotspot ter criado a interface/gateway
// antes. 127.0.0.1 nunca precisa ser esperado (sempre existe).
func WaitForIPs(ips []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		missing := missingIPs(ips)
		if len(missing) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("IPs ainda ausentes apos %s: %v (verifique se o hotspot subiu e se as bridges Docker/HOST_SOURCE_CIDR/HOTSPOT_GATEWAY estao corretos)", timeout, missing)
		}
		time.Sleep(2 * time.Second)
	}
}

func missingIPs(ips []string) []string {
	var missing []string
	for _, ip := range ips {
		if ip == "" || ip == "127.0.0.1" {
			continue
		}
		if !ipExists(ip) {
			missing = append(missing, ip)
		}
	}
	return missing
}

func ipExists(ip string) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	target := net.ParseIP(ip)
	if target == nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipNet.IP.Equal(target) {
			return true
		}
	}
	return false
}
