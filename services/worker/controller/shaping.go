package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// defaultBindnetUplinkInterface e o mesmo default de
// BINDNET_UPLINK_INTERFACE em services/worker/hotspot/interfaces.sh -
// mantido em sincronia manualmente (a variavel nao esta em
// envSections porque nao e editavel pelo admin, so um detalhe interno
// do uplink virtual).
const defaultBindnetUplinkInterface = "bn-uplink"

func registerShapingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/shaping/global", handleShapingGlobal)
	mux.HandleFunc("POST /hotspot/shaping/device", handleShapingDevice)
	mux.HandleFunc("GET /hotspot/shaping/stats", handleShapingStats)
	mux.HandleFunc("POST /hotspot/shaping/teardown", handleShapingTeardown)
}

type shapingGlobalRequest struct {
	Interface string `json:"interface"`
}

type shapingDeviceRequest struct {
	Interface         string `json:"interface"`
	MAC               string `json:"mac"`
	IP                string `json:"ip"`
	Fwmark            int    `json:"fwmark"`
	DownloadRateValue *int   `json:"downloadRateValue"`
	DownloadRateUnit  string `json:"downloadRateUnit"`
	UploadRateValue   *int   `json:"uploadRateValue"`
	UploadRateUnit    string `json:"uploadRateUnit"`
}

type shapingStatsResponse struct {
	DownloadBytes uint64 `json:"downloadBytes"`
	UploadBytes   uint64 `json:"uploadBytes"`
}

// handleShapingGlobal garante so a contagem agregada de todo o hotspot
// (regras iptables bn-global-up/down, sem casar por MAC/IP) - nao
// existe mais teto de banda global (removido, taxa/cota agora e so por
// dispositivo ou perfil, ver RULE.md e handleShapingDevice). Reaplicada
// todo ciclo pelo backend (reconcileGlobal) so pra alimentar o
// velocimetro/grafico geral do frontend (useGlobalStats/
// useGlobalSpeedHistory), que le esses contadores via
// GET /hotspot/shaping/stats sem mac.
func handleShapingGlobal(w http.ResponseWriter, r *http.Request) {
	var req shapingGlobalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Interface == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}

	apIface, uplinkIface, err := resolveShapingInterfaces(req.Interface)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := ensureShapingChain(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := applyGlobalMarkRules(apIface, uplinkIface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleShapingDevice garante a contagem (sempre) e a classe HTB
// dedicada do dispositivo (so quando ha taxa configurada) - reenviado
// a cada ciclo de reconciliacao do backend, inclusive so pra atualizar
// o IP atual quando o dispositivo renova o DHCP.
func handleShapingDevice(w http.ResponseWriter, r *http.Request) {
	var req shapingDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Interface == "" || req.MAC == "" || req.Fwmark == 0 {
		http.Error(w, "campos 'interface', 'mac' e 'fwmark' obrigatorios", http.StatusBadRequest)
		return
	}

	apIface, uplinkIface, err := resolveShapingInterfaces(req.Interface)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := ensureShapingChain(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := ensureDeviceMarkRules(apIface, uplinkIface, req.MAC, req.IP, req.Fwmark); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.DownloadRateValue == nil && req.UploadRateValue == nil {
		removeDeviceClass(apIface, req.Fwmark)
		removeDeviceClass(uplinkIface, req.Fwmark)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := ensureRootQdisc(apIface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := ensureRootQdisc(uplinkIface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.DownloadRateValue != nil {
		if err := ensureDeviceClass(apIface, req.Fwmark, *req.DownloadRateValue, req.DownloadRateUnit); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		removeDeviceClass(apIface, req.Fwmark)
	}
	if req.UploadRateValue != nil {
		if err := ensureDeviceClass(uplinkIface, req.Fwmark, *req.UploadRateValue, req.UploadRateUnit); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		removeDeviceClass(uplinkIface, req.Fwmark)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleShapingStats devolve os contadores absolutos (nao a taxa) do
// dispositivo (?mac=) ou do total global (?mac= ausente) - o backend
// calcula bps comparando duas leituras sucessivas.
func handleShapingStats(w http.ResponseWriter, r *http.Request) {
	mac := strings.TrimSpace(r.URL.Query().Get("mac"))
	var download, upload uint64
	var err error
	if mac == "" {
		download, upload, err = readGlobalCounters()
	} else {
		download, upload, err = readCounters(mac)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(shapingStatsResponse{DownloadBytes: download, UploadBytes: upload})
}

// handleShapingTeardown limpa contagem/shaping por completo,
// best-effort - chamado quando o hotspot para (os qdiscs tambem
// desaparecem sozinhos junto com a interface, isso e so higiene).
func handleShapingTeardown(w http.ResponseWriter, r *http.Request) {
	var req shapingGlobalRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	flushShapingChain()
	if req.Interface != "" {
		if apIface, uplinkIface, err := resolveShapingInterfaces(req.Interface); err == nil {
			teardownRootQdisc(apIface)
			teardownRootQdisc(uplinkIface)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveShapingInterfaces traduz WIFI_INTERFACE para a interface real
// do create_ap e devolve a interface real de saida para a internet.
// create_ap recebe um dummy estavel (bn-uplink), mas o entrypoint do
// hotspot aplica NAT/forward direto para a interface fisica real; os
// contadores e filtros precisam casar com esse caminho efetivo.
func resolveShapingInterfaces(iface string) (apIface, uplinkIface string, err error) {
	containerID, containerErr := composeServiceContainerID("hotspot")
	if containerErr != nil || containerID == "" {
		if containerErr == nil {
			containerErr = errors.New("container do hotspot ausente")
		}
		return "", "", containerErr
	}
	apIface = resolveRunningIface(containerID, iface)
	uplinkIface = hotspotNATInterface()
	if uplinkIface == "" {
		uplinkIface = defaultRouteInterface()
	}
	if uplinkIface == "" {
		uplinkIface = uplinkInterfaceName()
	}
	return apIface, uplinkIface, nil
}

func hotspotNATInterface() string {
	output, err := exec.Command("iptables", "-w", "-t", "nat", "-S", "BINDNET-HOTSPOT").CombinedOutput()
	if err != nil {
		return ""
	}
	return parseHotspotNATInterface(string(output))
}

func parseHotspotNATInterface(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "-j MASQUERADE") {
			continue
		}
		if iface := valueAfterToken(strings.Fields(line), "-o"); iface != "" {
			return iface
		}
	}
	return ""
}

func defaultRouteInterface() string {
	output, err := exec.Command("ip", "-o", "route", "get", "1.1.1.1").CombinedOutput()
	if err == nil {
		if iface := parseRouteInterface(string(output)); iface != "" {
			return iface
		}
	}
	output, err = exec.Command("ip", "-o", "route", "show", "default").CombinedOutput()
	if err != nil {
		return ""
	}
	return parseRouteInterface(string(output))
}

func parseRouteInterface(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if iface := valueAfterToken(strings.Fields(line), "dev"); iface != "" {
			return iface
		}
	}
	return ""
}

func valueAfterToken(fields []string, token string) string {
	for index, field := range fields {
		if field == token && index+1 < len(fields) {
			return fields[index+1]
		}
	}
	return ""
}

func uplinkInterfaceName() string {
	if value := strings.TrimSpace(os.Getenv("BINDNET_UPLINK_INTERFACE")); value != "" {
		return value
	}
	return defaultBindnetUplinkInterface
}
