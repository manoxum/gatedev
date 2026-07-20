package peer

import (
	"bindnet/worker/internal/network"
	"errors"
	"log"
	"net"
	"sort"
	"strings"
)

const peerScanMaxCandidates = 768

// isVirtualScanInterface filtra interfaces que nunca fazem sentido varrer
// em busca de peers: "lo" e as geradas por Docker/VPN/bridge/uplink dummy
// Bindnet. Ao contrario de isVirtualInterface (network.go), NAO exclui
// "ap0" - a interface AP virtual do hotspot e exatamente onde clientes do
// proprio hotspot (candidatos legitimos a peer) estao; excluir "ap0" aqui
// criaria um ponto cego que isVirtualInterface nao tem motivo pra ter,
// ja que seu proposito e outro (seletor de interface do painel).
func isVirtualScanInterface(name string) bool {
	if name == "lo" {
		return true
	}
	for _, prefix := range network.VirtualInterfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func peerScanCandidates() ([]string, map[string]bool, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}

	localIPs := map[string]bool{}
	seen := map[string]bool{}
	var candidates []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			log.Printf("[worker] busca de peers: pulando interface %s (down)", iface.Name)
			continue
		}
		if isVirtualScanInterface(iface.Name) {
			log.Printf("[worker] busca de peers: pulando interface %s (virtual)", iface.Name)
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		before := len(candidates)
		for _, addr := range addrs {
			ip, network, ok := ipv4NetworkFromAddr(addr)
			if !ok {
				continue
			}
			localIPs[ip.String()] = true
			hosts := boundedIPv4Hosts(ip, network)
			if len(hosts) == 0 {
				log.Printf("[worker] busca de peers: interface %s (%s) nao gerou candidatos - mascara de rede degenerada (ex.: /32)? verifique a configuracao de rede", iface.Name, ip.String())
			}
			for _, candidate := range hosts {
				if seen[candidate] {
					continue
				}
				seen[candidate] = true
				candidates = append(candidates, candidate)
				if len(candidates) >= peerScanMaxCandidates {
					break
				}
			}
		}
		log.Printf("[worker] busca de peers: interface %s contribuiu %d candidato(s)", iface.Name, len(candidates)-before)
	}
	sort.Strings(candidates)
	return candidates, localIPs, nil
}

func ipv4NetworkFromAddr(addr net.Addr) (net.IP, *net.IPNet, bool) {
	ipNet, ok := addr.(*net.IPNet)
	if !ok {
		return nil, nil, false
	}
	ip := ipNet.IP.To4()
	if ip == nil || ip.IsLoopback() {
		return nil, nil, false
	}
	return ip, ipNet, true
}

func boundedIPv4Hosts(localIP net.IP, network *net.IPNet) []string {
	localIP = localIP.To4()
	if localIP == nil {
		return nil
	}
	_, bits := network.Mask.Size()
	if bits != 32 {
		return nil
	}
	start := ipv4ToUint32(network.IP.To4())
	mask := ipv4ToUint32(net.IP(network.Mask).To4())
	first := (start & mask) + 1
	last := (start | ^mask) - 1
	if last < first {
		return nil
	}
	if total := last - first + 1; total <= peerScanMaxCandidates {
		var hosts []string
		for value := first; value <= last; value++ {
			hosts = append(hosts, uint32ToIPv4(value).String())
		}
		return hosts
	}

	var hosts []string
	add := func(value uint32) {
		if value < first || value > last || len(hosts) >= peerScanMaxCandidates {
			return
		}
		hosts = append(hosts, uint32ToIPv4(value).String())
	}

	local := ipv4ToUint32(localIP)
	local24 := &net.IPNet{IP: net.IPv4(localIP[0], localIP[1], localIP[2], 0), Mask: net.CIDRMask(24, 32)}
	local24Start := ipv4ToUint32(local24.IP.To4())
	local24Mask := ipv4ToUint32(net.IP(local24.Mask).To4())
	local24First := (local24Start & local24Mask) + 1
	local24Last := (local24Start | ^local24Mask) - 1
	for value := local24First; value <= local24Last && len(hosts) < peerScanMaxCandidates; value++ {
		add(value)
	}
	for step := uint32(1); len(hosts) < peerScanMaxCandidates; step++ {
		progressed := false
		if local >= step && local-step >= first {
			add(local - step)
			progressed = true
		}
		if local+step >= local && local+step <= last {
			add(local + step)
			progressed = true
		}
		if !progressed {
			break
		}
	}
	return hosts
}

func ipv4ToUint32(ip net.IP) uint32 {
	if ip == nil || len(ip) != net.IPv4len {
		panic(errors.New("invalid IPv4 address"))
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIPv4(value uint32) net.IP {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}
