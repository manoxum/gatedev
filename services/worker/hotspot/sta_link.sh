#!/usr/bin/env bash
# sta_link.sh - estado da associacao Wi-Fi cliente (STA) de
# WIFI_INTERFACE, extraido de channel.sh pra manter cada arquivo focado
# num unico dominio (ver CLAUDE.md - limite de ~200 linhas por
# arquivo): channel.sh cuida de selecao/ranking de canais, este arquivo
# cuida de "a placa esta associada como cliente agora, e em que
# canal?". Sourced pelo entrypoint.sh depois de channel.sh (usa
# freq_to_channel de la).
#
# POR QUE NAO "iw dev ... link": esse comando responde a partir do
# cache de resultados de varredura do cfg80211 (procura o BSS marcado
# como associado nos scan results), nao do estado vivo do driver.
# Confirmado ao vivo em 2026-07-16 nesta maquina (iwlwifi/AX211):
# cruzando journalctl (NetworkManager + wpa_supplicant, associacao
# continua sem nenhuma queda por >5 minutos) com o log do container no
# mesmo periodo, "iw dev link" respondeu "Not connected" em TODAS as
# leituras de janelas de 8s+ seguidas (20 tentativas x 0.4s cada,
# varias vezes em sequencia) enquanto a associacao real estava de pe o
# tempo todo - falso-negativo dominante por minutos, nao um "blip" de
# varredura coberto por paciencia. Foi exatamente isso que travava o
# modo Wi-Fi para Wi-Fi no loop "nao foi possivel confirmar o canal
# atual da associacao" ate desistir, indefinidamente.
#
# Sinais confiaveis usados no lugar (ambos leem estado vivo do driver
# via nl80211, sem passar pelo cache de varredura):
# - "iw dev ... station dump": tabela de estacoes do mac80211; em modo
#   managed, a unica entrada possivel e o proprio AP da associacao
#   atual (removida na hora em caso de desassociacao real).
# - "iw dev ... info": linha "channel N (FREQ MHz)" reflete o chanctx
#   que o radio esta realmente sintonizado pra associacao.
# "iw dev link" continua como sinal alternativo (se qualquer um dos
# dois enxergar a associacao, ela existe) - nunca mais como unico.

# sta_link_probe: leitura unica "esta associada agora?" - ver
# comentario no topo sobre por que station dump vem primeiro.
# Estilo "if ...; then return 0; fi" em vez de pipe solto de proposito:
# um pipe solto com pipefail conta como comando falho pro set -e do
# entrypoint quando o grep nao acha nada (caso normal com a placa
# desconectada) - mesmo risco ja confirmado ao vivo em
# ensure_wifi_radio_unblocked (regulatory.sh).
sta_link_probe() {
  # So significa "associada como estacao" se a interface esta em modo
  # managed: em modo AP (--no-virt, ou apos um crash que deixou a placa
  # presa nesse tipo), "station dump" lista os CLIENTES do hotspot - um
  # falso-positivo aqui faria o monitor de uplink (uplink.sh) aceitar a
  # propria placa do AP como fonte de internet.
  iw dev "${WIFI_INTERFACE}" info 2>/dev/null | grep -q 'type managed' || return 1
  if iw dev "${WIFI_INTERFACE}" station dump 2>/dev/null | grep -q '^Station '; then
    return 0
  fi
  if iw dev "${WIFI_INTERFACE}" link 2>/dev/null | grep -q '^Connected to '; then
    return 0
  fi
  return 1
}

# sta_link_connected confere se WIFI_INTERFACE esta associada como
# cliente Wi-Fi agora. A paciencia (STA_LINK_CHECK_ATTEMPTS x
# _INTERVAL_SECONDS, env do container sem equivalente no painel) so
# vale no modo Wi-Fi para Wi-Fi: so nesse modo existe uma associacao
# STA que importa preservar, e uma reconexao em andamento (ex.: o
# NetworkManager reassociando apos o AP fonte trocar de canal) merece
# alguns segundos antes de reprovar. Em qualquer outro modo (ex.:
# Ethernet para Wi-Fi) a interface foi desconectada de proposito antes
# do hotspot subir (unmanageWifiInterfaceIfIdle,
# services/worker/controller/compose.go) - ela nunca vai "reconectar",
# entao esperar aqui so atrasaria cada candidato de canal testado por
# rank_channels_for_band (8+ candidatos x 8s cada ja atrasou a subida
# em mais de um minuto numa sessao real).
sta_link_connected() {
  if [[ "${WIFI_INTERFACE}" != "${REAL_INTERNET_INTERFACE}" ]]; then
    sta_link_probe
    return $?
  fi

  local attempts="${STA_LINK_CHECK_ATTEMPTS:-20}"
  local interval="${STA_LINK_CHECK_INTERVAL_SECONDS:-0.4}"
  local attempt
  for attempt in $(seq 1 "${attempts}"); do
    if sta_link_probe; then
      return 0
    fi
    [[ "${attempt}" -lt "${attempts}" ]] && sleep "${interval}"
  done
  return 1
}

# sta_current_freq imprime a frequencia (MHz) da associacao atual, ou
# nada se indisponivel. Prefere o chanctx de "iw dev info" (estado vivo
# do driver, ver topo); cai pro "freq:" de "iw dev link" so como
# alternativa. Formato da linha: "channel 11 (2462 MHz), width: ..." -
# $3 e "(2462", limpo com gsub (awk do BusyBox, sem extensoes GNU).
sta_current_freq() {
  local freq
  freq="$(iw dev "${WIFI_INTERFACE}" info 2>/dev/null \
    | awk '$1 == "channel" { gsub(/[()]/, "", $3); print $3; exit }')"
  if [[ -z "${freq}" ]]; then
    freq="$(iw dev "${WIFI_INTERFACE}" link 2>/dev/null \
      | awk '/freq:/ { print $2; exit }')"
  fi
  [[ -n "${freq}" ]] || return 1
  printf '%s\n' "${freq}"
}

# sta_current_band_channel imprime "banda canal" (ex.: "2.4 6") da
# associacao Wi-Fi cliente atual de WIFI_INTERFACE, ou falha se a
# interface nao estiver associada agora. Usada por try_create_ap e
# attempt_hotspot_cycle (entrypoint.sh) para travar o AP no mesmo canal
# da estacao quando WIFI_INTERFACE e INTERNET_INTERFACE sao a mesma
# placa (AP+STA concorrente, Wi-Fi para Wi-Fi): um radio fisico so
# transmite numa frequencia por vez, entao o AP nao pode ficar num
# canal/banda diferente do link STA ativo nele - ignorar isso e o
# motivo mais comum de "cliente associa mas nunca completa o
# DHCP"/instabilidade nesse modo, mesmo quando o create_ap sobe sem
# erro aparente.
sta_current_band_channel() {
  local freq
  local channel

  sta_link_connected || return 1

  freq="$(sta_current_freq)" || return 1

  channel="$(freq_to_channel "${freq}")"
  [[ -n "${channel}" ]] || return 1

  if (( ${freq%%.*} >= 5000 )); then
    printf '5 %s\n' "${channel}"
  else
    printf '2.4 %s\n' "${channel}"
  fi
}
