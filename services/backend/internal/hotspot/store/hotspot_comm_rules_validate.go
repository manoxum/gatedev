// hotspot_comm_rules_validate.go reune a normalizacao e validacao
// das regras do firewall (shape/zona/L4) - separado do CRUD em
// hotspot_comm_rules.go para manter cada arquivo enxuto.
package store

import (
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// portListPattern valida "80", "80,443", "8000-8100", "53,80,443,1000-2000".
var portListPattern = regexp.MustCompile(`^\d{1,5}(-\d{1,5})?(,\d{1,5}(-\d{1,5})?)*$`)

// NormalizeCommRuleDefaults preenche defaults retrocompativeis (zona
// clients, protocolo any) e zera campos irrelevantes a zona escolhida,
// para o resto da validacao/gravacao raciocinar com valores canonicos.
func NormalizeCommRuleDefaults(req *CommRuleRequest) {
	if req.Zone == "" {
		req.Zone = CommZoneClients
	}
	if req.Protocol == "" {
		req.Protocol = CommProtocolAny
	}
	// Portas so fazem sentido em tcp/udp.
	if req.Protocol != CommProtocolTCP && req.Protocol != CommProtocolUDP {
		req.DstPorts = nil
	}
	// dst_host so existe na zona wan (destino externo); nas outras o
	// destino e implicito (outro cliente ou o gateway).
	if req.Zone != CommZoneWAN {
		req.DstHost = nil
	}
	// Nas zonas wan/local o destino e implicito (internet/gateway) - a
	// ponta de destino nao se aplica.
	if req.Zone == CommZoneWAN || req.Zone == CommZoneLocal {
		req.TargetKind = CommEndpointAny
		req.TargetRef = nil
		req.Direction = CommDirectionTo
	}
}

// ValidateCommRuleShape valida o shape da regra (zona, pontas, L4). A
// existencia do perfil e a normalizacao do MAC ficam no handler (que tem
// acesso ao banco e a normalizeHotspotMAC). Chame NormalizeCommRuleDefaults
// antes - a validacao assume valores ja canonicos.
func ValidateCommRuleShape(req CommRuleRequest) error {
	if req.Zone != CommZoneClients && req.Zone != CommZoneWAN && req.Zone != CommZoneLocal {
		return errors.New("campo 'zone' deve ser 'clients', 'wan' ou 'local'")
	}
	if err := validateCommRuleEndpoints(req); err != nil {
		return err
	}
	if req.Direction != CommDirectionTo && req.Direction != CommDirectionBoth {
		return errors.New("campo 'direction' deve ser 'to' ou 'both'")
	}
	if req.Action != CommActionAllow && req.Action != CommActionDeny {
		return errors.New("campo 'action' deve ser 'allow' ou 'deny'")
	}
	return validateCommRuleL4(req)
}

// validateCommRuleEndpoints valida as pontas conforme a zona: na zona
// 'clients' origem e destino sao pontas concretas; nas zonas 'wan'/
// 'local' o destino e implicito (internet/gateway) e a origem pode ser
// 'any' (todos os clientes).
func validateCommRuleEndpoints(req CommRuleRequest) error {
	if req.Zone == CommZoneClients {
		if req.SourceKind != CommEndpointDevice && req.SourceKind != CommEndpointProfile {
			return errors.New("campo 'sourceKind' deve ser 'device' ou 'profile'")
		}
		if req.SourceRef == "" {
			return errors.New("campo 'sourceRef' obrigatorio")
		}
		switch req.TargetKind {
		case CommEndpointDevice, CommEndpointProfile:
			if req.TargetRef == nil || *req.TargetRef == "" {
				return errors.New("campo 'targetRef' obrigatorio quando 'targetKind' nao e 'any'")
			}
		case CommEndpointAny:
			if req.TargetRef != nil && *req.TargetRef != "" {
				return errors.New("campo 'targetRef' deve ficar vazio quando 'targetKind' e 'any'")
			}
		default:
			return errors.New("campo 'targetKind' deve ser 'device', 'profile' ou 'any'")
		}
		if req.SourceKind == CommEndpointDevice && req.TargetKind == CommEndpointDevice &&
			req.TargetRef != nil && req.SourceRef == *req.TargetRef {
			return errors.New("origem e destino nao podem ser o mesmo dispositivo")
		}
		return nil
	}
	// Zonas wan/local: origem device|profile|any; destino implicito.
	switch req.SourceKind {
	case CommEndpointDevice, CommEndpointProfile:
		if req.SourceRef == "" {
			return errors.New("campo 'sourceRef' obrigatorio")
		}
	case CommEndpointAny:
	default:
		return errors.New("campo 'sourceKind' deve ser 'device', 'profile' ou 'any'")
	}
	return nil
}

func validateCommRuleL4(req CommRuleRequest) error {
	switch req.Protocol {
	case CommProtocolAny, CommProtocolTCP, CommProtocolUDP, CommProtocolICMP:
	default:
		return errors.New("campo 'protocol' deve ser 'any', 'tcp', 'udp' ou 'icmp'")
	}
	if req.DstPorts != nil && *req.DstPorts != "" {
		if req.Protocol != CommProtocolTCP && req.Protocol != CommProtocolUDP {
			return errors.New("portas so podem ser usadas com protocolo 'tcp' ou 'udp'")
		}
		if err := ValidatePortList(*req.DstPorts); err != nil {
			return err
		}
	}
	if req.DstHost != nil && *req.DstHost != "" {
		if req.Zone != CommZoneWAN {
			return errors.New("campo 'dstHost' so e valido na zona 'wan'")
		}
		if !isValidHostOrCIDR(*req.DstHost) {
			return errors.New("campo 'dstHost' deve ser um IP ou CIDR valido")
		}
	}
	return nil
}

func isValidHostOrCIDR(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(value)
	return err == nil
}

// ValidatePortList aceita "80", "80,443", "8000-8100" - portas 1..65535,
// intervalos com inicio <= fim. Devolve nil para lista valida.
func ValidatePortList(list string) error {
	if !portListPattern.MatchString(list) {
		return errors.New("lista de portas invalida (use ex.: 80,443,8000-8100)")
	}
	for _, part := range strings.Split(list, ",") {
		lo, hi, ok := strings.Cut(part, "-")
		start, err := strconv.Atoi(lo)
		if err != nil || start < 1 || start > 65535 {
			return errors.New("porta fora do intervalo 1-65535: " + lo)
		}
		if !ok {
			continue
		}
		end, err := strconv.Atoi(hi)
		if err != nil || end < 1 || end > 65535 {
			return errors.New("porta fora do intervalo 1-65535: " + hi)
		}
		if start > end {
			return errors.New("intervalo de portas invertido: " + part)
		}
	}
	return nil
}
