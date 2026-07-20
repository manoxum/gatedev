package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	peerScanConcurrentWorkers = 64
)

// RegisterRoutes expoe a busca manual de peers Bindnet na rede local.
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /network/peer-scan", handlePeerScan)
}

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
		log.Printf("[worker] busca de peers: porta invalida recebida: %q", port)
		http.Error(w, "porta invalida", http.StatusBadRequest)
		return
	}

	results, err := scanLocalBindnetPeers(r.Context(), port)
	if err != nil {
		log.Printf("[worker] busca de peers: falhou: %v", err)
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
		log.Printf("[worker] busca de peers: nenhum candidato gerado, nada a testar na porta %s", port)
		return []peerScanResult{}, nil
	}
	started := time.Now()
	log.Printf("[worker] busca de peers: iniciando varredura de %d candidato(s) na porta %s", len(candidates), port)

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
	log.Printf("[worker] busca de peers: concluida em %s - %d candidato(s) testado(s), %d encontrado(s)", time.Since(started).Round(time.Millisecond), len(candidates), len(peers))
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
	log.Printf("[worker] busca de peers: encontrado %s (%s) fingerprint=%s", address, nodeName, strings.TrimSpace(info.Fingerprint))
	return peerScanResult{
		Address:     address,
		NodeName:    nodeName,
		Fingerprint: strings.TrimSpace(info.Fingerprint),
		Source:      "manual-scan",
		LastSeenAt:  time.Now().Format(time.RFC3339),
		Domains:     info.Domains,
	}, true
}

func (r peerScanResult) String() string {
	if r.NodeName == "" {
		return r.Address
	}
	return fmt.Sprintf("%s (%s)", r.NodeName, r.Address)
}
