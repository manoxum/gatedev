package shaping

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"

	"bindnet/worker/internal/network"
)

// Zonas wan/local do firewall (cliente->internet e cliente->gateway),
// complementares a zona clients (BINDNET-ISOLATION). Diferente da zona
// clients, estas nao precisam de ap_isolate nem de reiniciar o hotspot:
// sao puro iptables no caminho forward/input, aplicadas ao vivo e
// reconstruidas so quando mudam. Ver firewall_wan.go e firewall_local.go
// para a mecanica de cada chain; aqui ficam a rota, a validacao e o
// utilitario de sincronizacao idempotente por assinatura de comentarios.
const (
	fwWanChain   = "BINDNET-FW-WAN"
	fwLocalChain = "BINDNET-FW-LOCAL"
)

func RegisterFirewallRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hotspot/firewall/apply", handleFirewallApply)
}

type firewallZonePayload struct {
	MAC      string `json:"mac"`
	Protocol string `json:"protocol"`
	DstPorts string `json:"dstPorts"`
	DstHost  string `json:"dstHost"`
	Action   string `json:"action"`
}

type firewallApplyRequest struct {
	Interface   string                `json:"interface"`
	Enabled     bool                  `json:"enabled"`
	WanPolicy   string                `json:"wanPolicy"`
	WanRules    []firewallZonePayload `json:"wanRules"`
	LocalPolicy string                `json:"localPolicy"`
	LocalRules  []firewallZonePayload `json:"localRules"`
}

func handleFirewallApply(w http.ResponseWriter, r *http.Request) {
	var req firewallApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corpo invalido", http.StatusBadRequest)
		return
	}
	if !req.Enabled {
		teardownFirewallZones()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.TrimSpace(req.Interface) == "" {
		http.Error(w, "campo 'interface' obrigatorio", http.StatusBadRequest)
		return
	}
	if !isValidPolicy(req.WanPolicy) || !isValidPolicy(req.LocalPolicy) {
		http.Error(w, "politica de zona invalida", http.StatusBadRequest)
		return
	}
	apIface, uplinkIface, err := resolveShapingInterfaces(req.Interface)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	wanRules, err := sanitizeZoneRules(req.WanRules, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	localRules, err := sanitizeZoneRules(req.LocalRules, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := syncWanChain(apIface, uplinkIface, req.WanPolicy, wanRules); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := syncLocalChain(apIface, req.LocalPolicy, localRules); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isValidPolicy(policy string) bool { return policy == "allow" || policy == "deny" }

// sanitizeZoneRules normaliza/valida cada regra (MAC, protocolo, portas,
// host). allowHost=true so na zona wan (destino externo).
func sanitizeZoneRules(rules []firewallZonePayload, allowHost bool) ([]firewallZonePayload, error) {
	out := make([]firewallZonePayload, 0, len(rules))
	for _, rule := range rules {
		if rule.MAC != "" {
			mac, err := network.NormalizeMAC(rule.MAC)
			if err != nil {
				return nil, fmt.Errorf("mac invalido: %s", rule.MAC)
			}
			rule.MAC = mac
		}
		if !isValidIsolationProtocol(rule.Protocol) {
			return nil, fmt.Errorf("protocolo invalido: %s", rule.Protocol)
		}
		if rule.DstPorts != "" && !isValidPortList(rule.DstPorts) {
			return nil, fmt.Errorf("portas invalidas: %s", rule.DstPorts)
		}
		if rule.DstHost != "" {
			if !allowHost || !isValidHostOrCIDR(rule.DstHost) {
				return nil, fmt.Errorf("host de destino invalido: %s", rule.DstHost)
			}
		}
		if rule.Action != "allow" && rule.Action != "deny" {
			return nil, fmt.Errorf("acao invalida: %s", rule.Action)
		}
		out = append(out, rule)
	}
	return out, nil
}

func isValidHostOrCIDR(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(value)
	return err == nil
}

// chainRule e uma regra a instalar num chain: o comentario (assinatura
// deterministica, usada para detectar mudanca) e os argumentos iptables
// completos apos "-A <chain>".
type chainRule struct {
	comment string
	args    []string
}

// syncChainRules reconstroi o chain so quando a assinatura ordenada de
// comentarios muda (caso comum a cada reconciliacao = no-op, so uma
// leitura). A ordem importa: o firewall casa a primeira regra.
func syncChainRules(chain string, rules []chainRule) error {
	_ = runIptables("-N", chain)
	desired := make([]string, len(rules))
	for i, rule := range rules {
		desired[i] = rule.comment
	}
	if equalStringSlices(chainComments(chain), desired) {
		return nil
	}
	if err := runIptables("-F", chain); err != nil {
		return err
	}
	for _, rule := range rules {
		if err := runIptables(append([]string{"-A", chain}, rule.args...)...); err != nil {
			return err
		}
	}
	return nil
}

// iptablesListing devolve "iptables -S <chain>" (regras em ordem), ou
// "" em erro - usado para ler comentarios e posicoes de jump.
func iptablesListing(chain string) string {
	output, err := exec.Command("iptables", "-w", "-S", chain).CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func chainComments(chain string) []string {
	var comments []string
	for _, line := range strings.Split(iptablesListing(chain), "\n") {
		if idx := strings.Index(line, "--comment "); idx >= 0 {
			comments = append(comments, parseIptablesComment(line[idx+len("--comment "):]))
		}
	}
	return comments
}

func teardownFirewallZones() {
	deleteRulesByComment("", "FORWARD", fwWanJumpComment)
	deleteRulesByComment("", "INPUT", fwLocalJumpComment)
	_ = runIptables("-F", fwWanChain)
	_ = runIptables("-X", fwWanChain)
	_ = runIptables("-F", fwLocalChain)
	_ = runIptables("-X", fwLocalChain)
}
