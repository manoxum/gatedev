// Package shaping controla contagem/limite de banda por dispositivo e o
// bloqueio de trafego (por credito) e portal cativo do hotspot, via
// iptables (mangle/filter/nat) e tc (HTB). Resolve as interfaces AP/uplink
// reais do create_ap pelo pacote compose.
package shaping

import (
	"encoding/json"
	"net/http"
	"strings"
)

func RegisterShapingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/shaping/global", handleShapingGlobal)
	mux.HandleFunc("POST /hotspot/shaping/device", handleShapingDevice)
	mux.HandleFunc("GET /hotspot/shaping/stats", handleShapingStats)
	mux.HandleFunc("POST /hotspot/shaping/teardown", handleShapingTeardown)
}

type shapingGlobalRequest struct {
	Interface string `json:"interface"`
}

type shapingDeviceRequest struct {
	Interface string `json:"interface"`
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Fwmark    int    `json:"fwmark"`
	// Taxa aceita valor fracionario (1.5MB/s, 17.5KB/s), por isso *float64
	// e nao *int - o tc parseia decimal no argumento de taxa (ver rate() em
	// tc.go). nil = sem limite.
	DownloadRateValue *float64 `json:"downloadRateValue"`
	DownloadRateUnit  string   `json:"downloadRateUnit"`
	UploadRateValue   *float64 `json:"uploadRateValue"`
	UploadRateUnit    string   `json:"uploadRateUnit"`
}

type shapingStatsResponse struct {
	DownloadBytes uint64 `json:"downloadBytes"`
	UploadBytes   uint64 `json:"uploadBytes"`
}

// handleShapingGlobal garante so a contagem agregada de todo o hotspot
// (regras iptables bn-global-up/down, sem casar por MAC/IP) - nao existe
// mais teto de banda global (removido, taxa/cota agora e so por dispositivo
// ou perfil, ver RULE.md e handleShapingDevice). Reaplicada todo ciclo pelo
// backend (reconcileGlobal) so pra alimentar o velocimetro/grafico geral do
// frontend (useGlobalStats/useGlobalSpeedHistory), que le esses contadores
// via GET /hotspot/shaping/stats sem mac.
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

// handleShapingDevice garante a contagem (sempre) e a classe HTB dedicada
// do dispositivo (so quando ha taxa configurada) - reenviado a cada ciclo
// de reconciliacao do backend, inclusive so pra atualizar o IP atual quando
// o dispositivo renova o DHCP.
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

// handleShapingTeardown limpa contagem/shaping por completo, best-effort -
// chamado quando o hotspot para (os qdiscs tambem desaparecem sozinhos
// junto com a interface, isso e so higiene).
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
