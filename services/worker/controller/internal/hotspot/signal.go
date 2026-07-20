package hotspot

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
)

// hotspotClientSignal roda "iw dev <iface> station dump" no container do
// hotspot e devolve um mapa MAC (minusculo) -> dBm do campo "signal:"
// (nem todo driver expoe uma linha "signal avg:" separada - so o
// primeiro numero da linha "signal:" e o valor combinado, confirmado
// ao vivo neste host: "signal:  \t-29 [-29, -41] dBm"). Best-effort:
// erro do comando ou driver sem suporte devolve mapa vazio, nunca
// quebra a listagem de clientes em handleHotspotClients (hotspot.go) -
// sinal e so um extra informativo.
func hotspotClientSignal(containerID, iface string) map[string]int {
	output, err := exec.Command("docker", "exec", containerID, "iw", "dev", iface, "station", "dump").CombinedOutput()
	if err != nil {
		return nil
	}

	signals := map[string]int{}
	var currentMAC string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(trimmed, "Station "):
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				currentMAC = strings.ToLower(fields[1])
			}
		case strings.HasPrefix(trimmed, "signal:") && currentMAC != "":
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				if dbm, err := strconv.Atoi(fields[1]); err == nil {
					signals[currentMAC] = dbm
				}
			}
		}
	}
	return signals
}
