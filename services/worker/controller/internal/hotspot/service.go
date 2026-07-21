package hotspot

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"bindnet/worker/internal/compose"
	"bindnet/worker/internal/network"
)

// RegisterServiceRoutes expoe o ciclo de vida do hotspot e o reinicio do
// dns-provider. Os servicos leem a configuracao operacional direto do banco
// quando iniciam; o worker nao transporta WIFI_/HOTSPOT_ por env.
func RegisterServiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/apply", handleHotspotServiceAction("restart"))
	mux.HandleFunc("POST /hotspot/start", handleHotspotServiceAction("start"))
	mux.HandleFunc("POST /hotspot/stop", handleHotspotServiceAction("stop"))
	mux.HandleFunc("GET /hotspot/status", handleHotspotServiceStatus)
	mux.HandleFunc("POST /dns/apply", handleDNSApply)
}

func handleDNSApply(w http.ResponseWriter, r *http.Request) {
	var config map[string]string
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "configuracao DNS invalida", http.StatusBadRequest)
		return
	}
	if err := compose.ApplyServices([]string{"dns-provider"}); err != nil {
		log.Printf("[worker] erro ao reiniciar dns-provider: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := network.SyncHostDNSRoutes(config); err != nil {
		log.Printf("[worker] erro ao sincronizar rotas DNS do host: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleHotspotServiceAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var config map[string]string
		_ = json.NewDecoder(r.Body).Decode(&config)

		if action != "stop" {
			if err := compose.EnsureHotspotContainer(); err != nil {
				log.Printf("[worker] erro ao garantir container do hotspot: %v", err)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			if err := compose.ApplyServices([]string{"dns-provider"}); err != nil {
				log.Printf("[worker] erro ao reiniciar dns-provider: %v", err)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			// Checa a associacao Wi-Fi cliente o mais tarde possivel,
			// imediatamente antes do "docker exec ... start" abaixo -
			// minimiza a janela entre esta checagem e a checagem
			// equivalente que try_create_ap (entrypoint.sh) faz sozinho
			// alguns segundos depois. Um round-trip a mais aqui (ex.:
			// checar a partir do backend, bem antes do /hotspot/apply e do
			// restart do dns-provider) da tempo de sobra pra uma associacao
			// Wi-Fi marginal cair entre as duas checagens e as duas
			// discordarem - foi exatamente essa janela que expos o bug do
			// NetworkManager brigando pela placa com o hostapd (ver
			// unmanageWifiInterface em internal/network).
			unmanageWifiInterfaceIfIdle(config["WIFI_INTERFACE"], config["INTERNET_INTERFACE"])
		}

		output, err := compose.ExecHotspotEntrypoint(action)
		if err != nil {
			log.Printf("[worker] erro ao executar hotspot %s: %v (%s)", action, err, output)
			http.Error(w, strings.TrimSpace(string(output)), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// unmanageWifiInterfaceIfIdle decide, antes do hotspot subir, se a placa
// Wi-Fi fisica fica com o NetworkManager ou sai dele:
//
//   - WIFI_INTERFACE == INTERNET_INTERFACE (Wi-Fi para Wi-Fi): a placa
//     PRECISA continuar gerenciada - o NetworkManager e o unico que mantem
//     a associacao STA viva (o create_ap so cria a interface virtual ap0
//     ao lado dela). Incondicional, sem checar se esta associada agora:
//     checar seria uma corrida real (confirmado ao vivo - pegar a placa
//     momentaneamente sem associacao durante uma reconexao fazia
//     desgerenciar, travando-a desconectada PARA SEMPRE). Alem de nao
//     desgerenciar, RE-gerencia (ManageWifiInterface, idempotente): sem
//     isso, trocar de um modo que desgerenciou a placa (ex.: Ethernet para
//     Wi-Fi com a placa ociosa) para Wi-Fi para Wi-Fi deixava o drop-in
//     orfao e a placa presa "unmanaged", e o hotspot esperava uma
//     reconexao que nunca viria.
//
//   - Placas diferentes COM a placa Wi-Fi associada como cliente: preserva
//     a associacao (mesma topologia de radio do Wi-Fi para Wi-Fi: o
//     entrypoint sobe o AP numa ap0 virtual travada no canal da estacao,
//     ver try_create_ap). E o que evita "partilhar do Ethernet desconecta
//     o Wi-Fi e ele some do sistema": o Wi-Fi cliente do usuario continua
//     funcionando e visivel no NetworkManager, so nao e usado como uplink
//     do hotspot.
//
//   - Placas diferentes SEM associacao: desgerencia/desconecta como antes -
//     o AP vai subir em --no-virt na placa fisica inteira, e deixa-la
//     gerenciada faria o NetworkManager escanear/tentar (re)associar essa
//     mesma placa enquanto o hostapd mantem o AP nela, derrubando o beacon
//     ("Failed to set beacon parameters"/"key not allowed", ver Dockerfile).
//
// Corrida residual: a associacao pode cair entre esta checagem e o primeiro
// try_create_ap (segundos). Nesse caso o AP sobe --no-virt com a placa
// ainda gerenciada - o watchdog de beacon (watchdog.sh) derruba/retenta se
// houver briga, e o NetworkManager reassociando devolve o caminho
// preservado na tentativa seguinte. Falha/ausencia de iface nunca bloqueia
// o start.
func unmanageWifiInterfaceIfIdle(wifiInterface, internetInterface string) {
	if wifiInterface == "" {
		return
	}
	if wifiInterface == internetInterface || network.InterfaceAssociated(wifiInterface) {
		if err := network.ManageWifiInterface(wifiInterface); err != nil {
			log.Printf("[worker] aviso: falha ao garantir %s gerenciada no NetworkManager: %v", wifiInterface, err)
		} else {
			log.Printf("[worker] placa %s mantida gerenciada no NetworkManager (associacao Wi-Fi cliente preservada para o hotspot)", wifiInterface)
		}
		return
	}
	if err := network.UnmanageWifiInterface(wifiInterface); err != nil {
		log.Printf("[worker] aviso: falha ao desgerenciar %s no NetworkManager: %v", wifiInterface, err)
	}
}

func handleHotspotServiceStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	containerID, running, err := compose.ServiceContainerRunning("hotspot")
	if err != nil || containerID == "" || !running {
		_ = json.NewEncoder(w).Encode(compose.ContainerStatus{Name: "hotspot", Running: false, Status: "stopped"})
		return
	}
	output, err := exec.Command("docker", "exec", containerID, "/usr/local/bin/hotspot-entrypoint.sh", "status").CombinedOutput()
	if err != nil {
		log.Printf("[worker] erro ao ler status do hotspot: %v (%s)", err, output)
		_ = json.NewEncoder(w).Encode(compose.ContainerStatus{Name: "hotspot", Running: false, Status: "unknown"})
		return
	}
	_, _ = w.Write(output)
}
