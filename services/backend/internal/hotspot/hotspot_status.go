package hotspot

import "regexp"

// bandRegex/channelRegex casam com a linha que try_create_ap
// (services/worker/hotspot/entrypoint.sh) sempre loga antes de tentar
// subir o AP - "Regiao Wi-Fi: ST; banda: 5GHz; canal: 44." (ou
// "banda: auto (5GHz); canal: auto (44)." quando WIFI_FREQ_BAND/
// WIFI_CHANNEL estao como "auto" - o "auto (" opcional so anota a
// origem do valor no log, o numero capturado e sempre o resolvido) -
// unica linha de log com banda/canal que existe independente de como
// eles foram resolvidos (fixo pelo admin, inferido de um canal fixo,
// ou varredura automatica; ver resolve_wifi_band em channel.sh, que
// loga textos DIFERENTES em cada um desses casos - so essa linha e
// comum a todos). Os padroes antigos ("Canal automatico escolhido"/
// "Banda Wi-Fi automatica escolhida") nunca batiam com nenhum log real
// do script, deixando canal/banda sempre "?" no cabecalho do painel.
//
// internetInterfaceRegex casa com a linha que apply_bindnet_uplink_rules
// (services/worker/hotspot/interfaces.sh) sempre loga ao aplicar
// NAT/forward - "Fonte real de internet do hotspot: eth0; ..." (ou
// "... auto (eth0); ..." quando INTERNET_INTERFACE=auto).
var (
	bandRegex              = regexp.MustCompile(`banda: (?:auto \()?([\d.]+)GHz\)?`)
	channelRegex           = regexp.MustCompile(`canal: (?:auto \()?(\d+)\)?`)
	internetInterfaceRegex = regexp.MustCompile(`Fonte real de internet do hotspot: (?:auto \()?([\w.-]+)\)?;`)
)

// parseHotspotChannelBand extrai o canal/banda ativos agora a partir
// das ultimas linhas de log do container do hotspot - usado por
// GET /api/hotspot/status.
func parseHotspotChannelBand(logs string) (channel, band string) {
	return lastRegexMatch(channelRegex, logs), lastRegexMatch(bandRegex, logs)
}

// parseHotspotInternetInterface extrai a interface de internet
// realmente em uso agora (resolvida quando INTERNET_INTERFACE=auto) a
// partir das ultimas linhas de log do container do hotspot - usado por
// GET /api/hotspot/status.
func parseHotspotInternetInterface(logs string) string {
	return lastRegexMatch(internetInterfaceRegex, logs)
}

// lastRegexMatch devolve o ultimo match (nao o primeiro) do primeiro
// grupo de captura de re em s, ou "" se nao houver nenhum. Ultimo, nao
// primeiro: dentro da janela de log lida pode haver mais de uma
// tentativa (retry apos falha de beacon, ver watchdog.sh), cada uma
// logando sua propria linha "Regiao Wi-Fi: ...; banda: X; canal: Y." -
// so a mais recente reflete o que esta realmente ativo agora.
func lastRegexMatch(re *regexp.Regexp, s string) string {
	matches := re.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}
