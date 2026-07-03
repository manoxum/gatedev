package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
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
	Interface    string `json:"interface"`
	DownloadMbps *int   `json:"downloadMbps"`
	UploadMbps   *int   `json:"uploadMbps"`
}

type shapingDeviceRequest struct {
	Interface    string `json:"interface"`
	MAC          string `json:"mac"`
	IP           string `json:"ip"`
	Fwmark       int    `json:"fwmark"`
	DownloadMbps *int   `json:"downloadMbps"`
	UploadMbps   *int   `json:"uploadMbps"`
}

type shapingStatsResponse struct {
	DownloadBytes uint64 `json:"downloadBytes"`
	UploadBytes   uint64 `json:"uploadBytes"`
}

// handleShapingGlobal aplica (ou remove, se os campos vierem nil) o
// teto global de download/upload - download e limitado na saida da
// interface ap0 (rumo ao cliente), upload na saida do uplink (rumo a
// internet).
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
	if err := ensureRootQdisc(apIface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := ensureRootQdisc(uplinkIface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := updateRootCeil(apIface, intOrZero(req.DownloadMbps)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := updateRootCeil(uplinkIface, intOrZero(req.UploadMbps)); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Interface == "" || req.MAC == "" || req.IP == "" || req.Fwmark == 0 {
		http.Error(w, "campos 'interface', 'mac', 'ip' e 'fwmark' obrigatorios", http.StatusBadRequest)
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

	if req.DownloadMbps == nil && req.UploadMbps == nil {
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
	if req.DownloadMbps != nil {
		if err := ensureDeviceClass(apIface, req.Fwmark, *req.DownloadMbps); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		removeDeviceClass(apIface, req.Fwmark)
	}
	if req.UploadMbps != nil {
		if err := ensureDeviceClass(uplinkIface, req.Fwmark, *req.UploadMbps); err != nil {
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

// resolveShapingInterfaces traduz WIFI_INTERFACE para a interface
// virtual real do create_ap (ap0) e devolve o nome do uplink dummy
// estavel - mesma logica de resolveRunningIface (hotspot.go), reusada
// aqui porque tc/iptables so fazem sentido contra os nomes reais.
func resolveShapingInterfaces(iface string) (apIface, uplinkIface string, err error) {
	containerID, containerErr := composeServiceContainerID("hotspot")
	if containerErr != nil || containerID == "" {
		if containerErr == nil {
			containerErr = errors.New("container do hotspot ausente")
		}
		return "", "", containerErr
	}
	apIface = resolveRunningIface(containerID, iface)
	uplinkIface = uplinkInterfaceName()
	return apIface, uplinkIface, nil
}

func uplinkInterfaceName() string {
	if value := strings.TrimSpace(os.Getenv("BINDNET_UPLINK_INTERFACE")); value != "" {
		return value
	}
	return defaultBindnetUplinkInterface
}

func intOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
