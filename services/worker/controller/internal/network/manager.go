package network

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// nmDropin e o arquivo de configuracao que marca a interface Wi-Fi
// como nao-gerenciada pelo NetworkManager, para o hostapd assumir o
// controle dela durante o hotspot.
const nmDropin = "/etc/NetworkManager/conf.d/90-bindnet-hotspot-unmanaged.conf"

type interfaceRequest struct {
	Interface string `json:"interface"`
}

// handleWifiUnmanage e o endpoint HTTP fino sobre UnmanageWifiInterface
// - ver essa funcao para a logica real. Mantido para uso manual/futuro
// (ex.: painel), mas o fluxo critico (POST /hotspot/start) chama
// UnmanageWifiInterface direto, sem round-trip HTTP - ver compose.go.
func handleWifiUnmanage(w http.ResponseWriter, r *http.Request) {
	var req interfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Interface == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}
	if err := UnmanageWifiInterface(req.Interface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnmanageWifiInterface marca a interface fisica (e a virtual "ap0" que
// o create_ap cria) como nao-gerenciada pelo NetworkManager, para o
// hostapd poder assumir o controle dela.
func UnmanageWifiInterface(iface string) error {
	content := fmt.Sprintf("[keyfile]\nunmanaged-devices=interface-name:%s;interface-name:ap0\n", iface)
	if err := os.WriteFile(nmDropin, []byte(content), 0644); err != nil {
		log.Printf("[worker] erro ao escrever %s: %v", nmDropin, err)
		return err
	}
	reloadOutput, reloadErr := reloadNetworkManagerConfig()
	if reloadErr != nil {
		log.Printf("[worker] aviso: 'nmcli general reload conf' falhou; tentando aplicar estado runtime mesmo assim: %v (%s)", reloadErr, reloadOutput)
	}
	if output, err := setDeviceManaged(iface, false); err != nil {
		log.Printf("[worker] erro ao rodar 'nmcli device set %s managed no': %v (%s)", iface, err, output)
		if reloadErr != nil {
			return fmt.Errorf("%s\n%s", strings.TrimSpace(string(reloadOutput)), strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	// ap0 pode nao existir ainda; se existir, tambem fica fora do NetworkManager.
	_, _ = setDeviceManaged("ap0", false)
	// "managed no" so impede o NetworkManager de gerenciar a interface
	// dali pra frente - se ela ja estava associada como cliente a uma
	// rede Wi-Fi, a associacao continua ativa. Com a placa ainda
	// ocupada como estacao, o create_ap falha ao criar a interface AP
	// virtual em qualquer canal/banda ("RTNETLINK answers: Resource
	// busy"), porque o driver nao aceita uma segunda interface virtual
	// enquanto a fisica esta associada. Desconectar aqui garante que a
	// placa esteja livre antes do hotspot tentar assumi-la.
	if output, err := disconnectDevice(iface); err != nil {
		log.Printf("[worker] aviso: 'nmcli device disconnect %s' falhou (pode ja estar desconectado): %v (%s)", iface, err, output)
	}
	return nil
}

// handleWifiManage remove o drop-in e devolve a interface ao controle
// normal do NetworkManager.
func handleWifiManage(w http.ResponseWriter, r *http.Request) {
	var req interfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Interface == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}
	if err := ManageWifiInterface(req.Interface); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ManageWifiInterface remove o drop-in e devolve a interface ao
// NetworkManager (que reconecta sozinho as redes salvas). Idempotente:
// chamavel mesmo quando a placa nunca foi desgerenciada. Usada por
// handleWifiManage (POST /network/wifi-manage, fluxo de stop/recover
// do painel) e por unmanageWifiInterfaceIfIdle (compose.go) nos modos
// que PRESERVAM a associacao Wi-Fi cliente - sem essa segunda chamada,
// trocar de um modo que desgerencia (ex.: Ethernet para Wi-Fi com a
// placa ociosa) para um que preserva (Wi-Fi para Wi-Fi) deixava a
// placa presa "unmanaged"/desconectada para sempre: o NetworkManager e
// o unico que a reconectaria, e estava proibido de toca-la.
func ManageWifiInterface(iface string) error {
	_ = os.Remove(nmDropin)
	reloadOutput, reloadErr := reloadNetworkManagerConfig()
	if reloadErr != nil {
		log.Printf("[worker] aviso: 'nmcli general reload conf' falhou; tentando devolver %s mesmo assim: %v (%s)", iface, reloadErr, reloadOutput)
	}
	output, err := setDeviceManaged(iface, true)
	if err != nil {
		log.Printf("[worker] erro ao rodar 'nmcli device set %s managed yes': %v (%s)", iface, err, output)
		if reloadErr != nil {
			return fmt.Errorf("%s\n%s", strings.TrimSpace(string(reloadOutput)), strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func reloadNetworkManagerConfig() ([]byte, error) {
	return exec.Command("nmcli", "general", "reload", "conf").CombinedOutput()
}

func setDeviceManaged(iface string, managed bool) ([]byte, error) {
	value := "no"
	if managed {
		value = "yes"
	}
	return exec.Command("nmcli", "device", "set", iface, "managed", value).CombinedOutput()
}

// disconnectDevice desassocia a interface de qualquer rede Wi-Fi a que
// ela esteja conectada como cliente. Erro aqui e esperado/inofensivo
// quando a interface ja estava desconectada.
func disconnectDevice(iface string) ([]byte, error) {
	return exec.Command("nmcli", "device", "disconnect", iface).CombinedOutput()
}
