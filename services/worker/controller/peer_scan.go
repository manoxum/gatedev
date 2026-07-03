package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPeerScanPort       = "8531"
	peerScanRequestTimeout    = 700 * time.Millisecond
	peerScanOverallTimeout    = 8 * time.Second
	peerScanMaxCandidates     = 768
	peerScanConcurrentWorkers = 64
)

type peerScanRequest struct {
	Port string `json:"port"`
}

type peerScanResult struct {
	Address     string   `json:"address"`
	NodeName    string   `json:"nodeName"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Source      string   `json:"source"`
	LastSeenAt  string   `json:"lastSeenAt"`
	Domains     []string `json:"domains,omitempty"`
}

type remoteDiscoverInfo struct {
	NodeName    string   `json:"nodeName"`
	Fingerprint string   `json:"fingerprint"`
	Domains     []string `json:"domains"`
}

func handlePeerScan(w http.ResponseWriter, r *http.Request) {
	var req peerScanRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	port := strings.TrimSpace(req.Port)
	if port == "" {
		port = defaultPeerScanPort
	}
	if _, err := strconv.Atoi(port); err != nil {
		http.Error(w, "porta invalida", http.StatusBadRequest)
		return
	}

	results, err := scanLocalBindnetPeers(r.Context(), port)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func scanLocalBindnetPeers(parent context.Context, port string) ([]peerScanResult, error) {
	candidates, localIPs, err := peerScanCandidates()
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []peerScanResult{}, nil
	}

	ctx, cancel := context.WithTimeout(parent, peerScanOverallTimeout)
	defer cancel()

	jobs := make(chan string)
	results := make(chan peerScanResult)
	var wg sync.WaitGroup
	for i := 0; i < peerScanConcurrentWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				if localIPs[ip] {
					continue
				}
				if result, ok := probeBindnetPeer(ctx, ip, port); ok {
					results <- result
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, ip := range candidates {
			select {
			case <-ctx.Done():
				return
			case jobs <- ip:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	byAddress := map[string]peerScanResult{}
	for result := range results {
		byAddress[result.Address] = result
	}

	peers := make([]peerScanResult, 0, len(byAddress))
	for _, result := range byAddress {
		peers = append(peers, result)
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].Address < peers[j].Address })
	return peers, nil
}

func probeBindnetPeer(ctx context.Context, ip, port string) (peerScanResult, bool) {
	requestCtx, cancel := context.WithTimeout(ctx, peerScanRequestTimeout)
	defer cancel()

	url := "http://" + net.JoinHostPort(ip, port) + "/discover/info"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return peerScanResult{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return peerScanResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return peerScanResult{}, false
	}

	var info remoteDiscoverInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return peerScanResult{}, false
	}
	address := net.JoinHostPort(ip, port)
	nodeName := strings.TrimSpace(info.NodeName)
	if nodeName == "" {
		nodeName = address
	}
	return peerScanResult{
		Address:     address,
		NodeName:    nodeName,
		Fingerprint: strings.TrimSpace(info.Fingerprint),
		Source:      "manual-scan",
		LastSeenAt:  time.Now().Format(time.RFC3339),
		Domains:     info.Domains,
	}, true
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
		if iface.Flags&net.FlagUp == 0 || isVirtualInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, network, ok := ipv4NetworkFromAddr(addr)
			if !ok {
				continue
			}
			localIPs[ip.String()] = true
			for _, candidate := range boundedIPv4Hosts(ip, network) {
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

func (r peerScanResult) String() string {
	if r.NodeName == "" {
		return r.Address
	}
	return fmt.Sprintf("%s (%s)", r.NodeName, r.Address)
}
