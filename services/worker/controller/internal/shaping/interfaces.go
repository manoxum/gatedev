package shaping

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"bindnet/worker/internal/compose"
)

// defaultBindnetUplinkInterface e o mesmo default de
// BINDNET_UPLINK_INTERFACE em services/worker/hotspot/interfaces.sh -
// mantido em sincronia manualmente (a variavel nao e editavel pelo admin,
// so um detalhe interno do uplink virtual).
const defaultBindnetUplinkInterface = "bn-uplink"

// resolveShapingInterfaces traduz WIFI_INTERFACE para a interface real do
// create_ap e devolve a interface real de saida para a internet. create_ap
// recebe um dummy estavel (bn-uplink), mas o entrypoint do hotspot aplica
// NAT/forward direto para a interface fisica real; os contadores e filtros
// precisam casar com esse caminho efetivo.
func resolveShapingInterfaces(iface string) (apIface, uplinkIface string, err error) {
	containerID, containerErr := compose.ServiceContainerID("hotspot")
	if containerErr != nil || containerID == "" {
		if containerErr == nil {
			containerErr = errors.New("container do hotspot ausente")
		}
		return "", "", containerErr
	}
	apIface = compose.ResolveRunningIface(containerID, iface)
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
