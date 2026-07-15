package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
)

type hotspotMACActionRequest struct {
	Interface string `json:"interface"`
	MAC       string `json:"mac"`
}

func handleHotspotBlock(w http.ResponseWriter, r *http.Request) {
	handleHotspotMACAction(w, r, true)
}

func handleHotspotUnblock(w http.ResponseWriter, r *http.Request) {
	handleHotspotMACAction(w, r, false)
}

func handleHotspotMACAction(w http.ResponseWriter, r *http.Request, block bool) {
	var req hotspotMACActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return
	}
	req.Interface = strings.TrimSpace(req.Interface)
	mac, err := normalizeMAC(req.MAC)
	if err != nil {
		http.Error(w, "mac invalido", http.StatusBadRequest)
		return
	}
	if req.Interface == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}

	if err := applyHostapdMACAction(req.Interface, mac, block); err != nil {
		log.Printf("[worker] erro ao aplicar ACL do hotspot para %s: %v", mac, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func applyHostapdMACAction(iface, mac string, block bool) error {
	containerID, err := composeServiceContainerID("hotspot")
	if err != nil || containerID == "" {
		if err == nil {
			err = errors.New("container do hotspot ausente")
		}
		return err
	}

	realIface := resolveRunningIface(containerID, iface)
	ctrlDir, err := hotspotControlDir(containerID, iface, realIface)
	if err != nil {
		return err
	}

	action := []string{"hostapd_cli", "-p", ctrlDir, "-i", realIface, "deny_acl"}
	if block {
		action = append(action, "ADD_MAC", mac)
	} else {
		action = append(action, "DEL_MAC", mac)
	}
	output, err := exec.Command("docker", append([]string{"exec", containerID}, action...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostapd_cli deny_acl falhou: %s: %w", strings.TrimSpace(string(output)), err)
	}

	if block {
		output, err = exec.Command("docker", "exec", containerID, "hostapd_cli", "-p", ctrlDir, "-i", realIface, "deauthenticate", mac).CombinedOutput()
		if err != nil {
			return fmt.Errorf("hostapd_cli deauthenticate falhou: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}
	return nil
}

func hotspotControlDir(containerID, iface, realIface string) (string, error) {
	output, err := exec.Command("docker", "exec", containerID, "sh", "-c", `
set -eu
for path in "/tmp/create_ap.$1.conf."*/hostapd_ctrl/"$2" /tmp/create_ap.*.conf.*/hostapd_ctrl/"$2"; do
  if [ -e "$path" ]; then
    dirname "$path"
    exit 0
  fi
done
exit 1
`, "sh", iface, realIface).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("diretorio de controle do hostapd nao encontrado para %s: %s", realIface, strings.TrimSpace(string(output)))
	}
	ctrlDir := strings.TrimSpace(string(output))
	if ctrlDir == "" {
		return "", fmt.Errorf("diretorio de controle do hostapd vazio para %s", realIface)
	}
	return ctrlDir, nil
}

func normalizeMAC(raw string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return strings.ToLower(hw.String()), nil
}
