// captive_portal.go implementa o redirecionamento automatico do
// portal cativo para dispositivos bloqueados por falta de credito -
// usado SO pelo fluxo de credito (ver hotspot_credit_recharge.go no
// backend), nunca pelo bloqueio manual do admin (blocklist
// mode="traffic" continua sem portal cativo - um dispositivo banido
// pelo admin nao deve ser incentivado a "resgatar um voucher" para
// voltar). Reaproveita o mesmo chain/idioma de traffic_block.go, so
// que na tabela nat/PREROUTING (avaliada ANTES do filter/FORWARD onde
// vive o DROP existente) - a requisicao HTTP do dispositivo bloqueado e
// desviada para um responder local minimo que sempre devolve um
// redirect 302 para a pagina de autoatendimento, antes mesmo do DROP
// ser avaliado. So intercepta a porta 80 (HTTPS nao da pra redirecionar
// sem quebrar o certificado - limitacao universal de qualquer portal
// cativo; a sondagem de deteccao do proprio SO do dispositivo usa HTTP
// simples, entao isso nao compromete a deteccao automatica).
package shaping

import (
	"bindnet/worker/internal/network"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const captivePortalRedirectPort = 8880

var (
	captivePortalMu  sync.RWMutex
	captivePortalURL = ""
)

type hotspotCaptivePortalRequest struct {
	Interface string `json:"interface"`
	MAC       string `json:"mac"`
	PortalURL string `json:"portalUrl,omitempty"`
}

func RegisterCaptivePortalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/captiveportal/enable", handleCaptivePortalEnable)
	mux.HandleFunc("POST /hotspot/captiveportal/disable", handleCaptivePortalDisable)
}

func handleCaptivePortalEnable(w http.ResponseWriter, r *http.Request) {
	var req hotspotCaptivePortalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return
	}
	mac, err := network.NormalizeMAC(req.MAC)
	if err != nil {
		http.Error(w, "mac invalido", http.StatusBadRequest)
		return
	}
	req.Interface = strings.TrimSpace(req.Interface)
	req.PortalURL = strings.TrimSpace(req.PortalURL)
	if req.Interface == "" || req.PortalURL == "" {
		http.Error(w, "campos 'interface' e 'portalUrl' obrigatorios", http.StatusBadRequest)
		return
	}
	setCaptivePortalURL(req.PortalURL)

	apIface, _, err := resolveShapingInterfaces(req.Interface)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := ensureDeviceCaptiveRedirect(apIface, mac); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleCaptivePortalDisable(w http.ResponseWriter, r *http.Request) {
	var req hotspotCaptivePortalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return
	}
	mac, err := network.NormalizeMAC(req.MAC)
	if err != nil {
		http.Error(w, "mac invalido", http.StatusBadRequest)
		return
	}
	removeDeviceCaptiveRedirect(mac)
	w.WriteHeader(http.StatusNoContent)
}

// ensureDeviceCaptiveRedirect insere, na tabela nat (nao no
// filter/FORWARD onde vive o DROP de traffic_block.go), uma regra
// REDIRECT por MAC que desvia so a porta 80 para o responder local -
// PREROUTING e avaliada antes da decisao de roteamento (FORWARD vs
// INPUT), entao essa regra intercepta o pacote antes mesmo do DROP
// existente ser avaliado, sem precisar reordenar nada entre os dois
// chains.
func ensureDeviceCaptiveRedirect(apIface, mac string) error {
	comment := "bn-captiveportal-" + mac
	if iptablesCheck(
		"-t", "nat", "-C", "PREROUTING",
		"-i", apIface, "-m", "mac", "--mac-source", mac,
		"-p", "tcp", "--dport", "80",
		"-m", "comment", "--comment", comment, "-j", "REDIRECT", "--to-port", strconv.Itoa(captivePortalRedirectPort),
	) == nil {
		return nil
	}
	return runIptables(
		"-t", "nat", "-I", "PREROUTING", "1",
		"-i", apIface, "-m", "mac", "--mac-source", mac,
		"-p", "tcp", "--dport", "80",
		"-m", "comment", "--comment", comment, "-j", "REDIRECT", "--to-port", strconv.Itoa(captivePortalRedirectPort),
	)
}

func removeDeviceCaptiveRedirect(mac string) {
	deleteRulesByComment("nat", "PREROUTING", "bn-captiveportal-"+mac)
}

func setCaptivePortalURL(url string) {
	captivePortalMu.Lock()
	defer captivePortalMu.Unlock()
	captivePortalURL = url
}

func getCaptivePortalURL() string {
	captivePortalMu.RLock()
	defer captivePortalMu.RUnlock()
	return captivePortalURL
}

// StartCaptivePortalResponder sobe um servidor HTTP minimo que a regra
// REDIRECT acima desvia o trafego de dispositivos bloqueados para ca -
// responde QUALQUER requisicao com um redirect para a pagina de
// autoatendimento. Escuta em todas as interfaces (0.0.0.0): o alvo
// REDIRECT do iptables e o endereco primario da interface que recebeu
// o pacote (o IP do gateway do hotspot), nunca 127.0.0.1. Chamado uma
// vez no boot do controller (main.go); inofensivo quando o hotspot
// esta parado, ja que nada e redirecionado pra ele nesse caso.
func StartCaptivePortalResponder() {
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			url := getCaptivePortalURL()
			if url == "" {
				http.Error(w, "portal indisponivel", http.StatusServiceUnavailable)
				return
			}
			http.Redirect(w, r, url, http.StatusFound)
		})
		addr := "0.0.0.0:" + strconv.Itoa(captivePortalRedirectPort)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[worker] responder do portal cativo encerrou: %v", err)
		}
	}()
}
