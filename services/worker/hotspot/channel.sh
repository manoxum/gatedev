#!/usr/bin/env bash
# channel.sh - selecao de banda/canal Wi-Fi, extraido do entrypoint.sh
# principal pra manter cada arquivo focado num unico dominio (ver
# CLAUDE.md - limite de ~200 linhas por arquivo). Sourced pelo
# entrypoint.sh; usa "log" e as variaveis WIFI_* ja resolvidas por ele.

interface_phy() {
  iw dev "${WIFI_INTERFACE}" info 2>/dev/null | awk '/wiphy/{print $2}'
}

band_supported() {
  local band="$1"
  local phy
  local info

  phy="$(interface_phy)"
  [[ -n "${phy}" ]] || return 1

  info="$(iw "phy${phy}" info 2>/dev/null || true)"
  [[ -n "${info}" ]] || return 1

  if [[ "${band}" == "5" ]]; then
    grep -Eq '\* 5[0-9]{3}(\.[0-9])? MHz' <<< "${info}"
  else
    grep -Eq '\* 24[0-9]{2}(\.[0-9])? MHz' <<< "${info}"
  fi
}

resolve_wifi_band() {
  if [[ "${WIFI_FREQ_BAND}" != "auto" ]]; then
    if [[ "${WIFI_FREQ_BAND}" != "2.4" && "${WIFI_FREQ_BAND}" != "5" ]]; then
      log "ERRO: WIFI_FREQ_BAND deve ser 2.4, 5 ou auto."
      exit 1
    fi
    return
  fi

  if [[ "${WIFI_CHANNEL}" =~ ^[0-9]+$ ]]; then
    if (( WIFI_CHANNEL >= 1 && WIFI_CHANNEL <= 14 )); then
      WIFI_FREQ_BAND="2.4"
    else
      WIFI_FREQ_BAND="5"
    fi
    log "Banda Wi-Fi inferida a partir do canal ${WIFI_CHANNEL}: ${WIFI_FREQ_BAND}GHz."
    return
  fi

  ip link set "${WIFI_INTERFACE}" up >/dev/null 2>&1 || true

  # Wi-Fi para Wi-Fi nunca usa essa preferencia de verdade (o canal e
  # sempre travado pelo canal da estacao dentro de try_create_ap,
  # entrypoint.sh) - so consulta o historico quando a banda escolhida
  # aqui realmente importa (Ethernet para Wi-Fi/auto). Prefere a banda
  # com historico de sucesso real melhor em vez de sempre comecar pela
  # banda preferida por capacidade de hardware (5GHz): sem isso, uma
  # placa com trava regulatoria de firmware que rejeita 5GHz inteiro
  # (ver regulatory.sh) reaprende essa mesma licao do zero, testando
  # todos os candidatos de 5GHz de novo, toda vez que o hotspot sobe.
  if [[ "${WIFI_INTERFACE}" != "${REAL_INTERNET_INTERFACE}" ]]; then
    local score_5 score_24
    score_5="$(band_history_score "5")"
    score_24="$(band_history_score "2.4")"
    if (( score_24 > score_5 )); then
      WIFI_FREQ_BAND="2.4"
      log "Banda Wi-Fi automatica escolhida: 2.4GHz (historico de sucesso melhor: ${score_24} contra ${score_5} em 5GHz)."
      return
    fi
  fi

  if band_supported "5"; then
    WIFI_FREQ_BAND="5"
  elif band_supported "2.4"; then
    WIFI_FREQ_BAND="2.4"
  else
    WIFI_FREQ_BAND="2.4"
    log "AVISO: nao foi possivel detectar as bandas suportadas por ${WIFI_INTERFACE}; usando 2.4GHz."
  fi
  log "Banda Wi-Fi automatica escolhida: ${WIFI_FREQ_BAND}GHz."
}

freq_to_channel() {
  local freq="$1"
  # Versoes recentes do "iw" retornam a frequencia com casas decimais
  # (ex.: "2467.0" em vez de "2467") - bash nao faz aritmetica com
  # ponto flutuante em "(( ))", entao trunca antes de comparar.
  freq="${freq%%.*}"

  if (( freq == 2484 )); then
    printf '14\n'
  elif (( freq >= 2412 && freq <= 2472 )); then
    printf '%s\n' "$(((freq - 2407) / 5))"
  elif (( freq >= 5000 && freq <= 5900 )); then
    printf '%s\n' "$(((freq - 5000) / 5))"
  fi
}

# sta_link_connected confere se WIFI_INTERFACE esta associada como
# cliente Wi-Fi agora, tolerando blips momentaneos: uma unica leitura
# de "iw dev ... link" pode pegar a placa no meio de uma varredura de
# fundo que o proprio NetworkManager/wpa_supplicant dispara
# periodicamente numa interface ja associada (mantem a lista de APs
# proximos atualizada / avalia roaming, ex.: entre "WIFI2.4" e
# "WIFI2.4b") - confirmado comparando com polling externo ao container
# durante o blip: a associacao nunca cai de verdade (o usuario nunca
# percebe nada), so a resposta pontual de "iw dev link" fica ambigua
# por ate alguns segundos enquanto o unico radio fisico sai do canal
# pra escanear outros. Um debounce curto (poucos centissegundos) nao
# e suficiente pra cobrir um ciclo de varredura tipico (2.4GHz+5GHz
# inteiros) - a paciencia aqui e alta de proposito porque o custo de
# esperar mais um pouco aqui e muito menor que cair pro loop de retry
# la fora (backoff de 3s/6s/9s.../HOTSPOT_RESTART_BACKOFF_SECONDS por
# tentativa, ver entrypoint.sh) so pra repetir a mesma checagem de
# novo. STA_LINK_CHECK_ATTEMPTS/_INTERVAL_SECONDS (env do container,
# sem equivalente no painel - mesmo padrao de
# HOTSPOT_BEACON_FAILURE_THRESHOLD em watchdog.sh) controlam quanto
# tempo tolerar antes de considerar "nao associada" de verdade; o
# padrao (20 x 0.4s = 8s) cobre um ciclo tipico de varredura sem
# travar tempo demais numa queda genuina (essa ainda reprova todas as
# tentativas e retorna erro normalmente).
sta_link_connected() {
  # A paciencia abaixo so faz sentido quando WIFI_INTERFACE tambem e a
  # fonte de internet (Wi-Fi para Wi-Fi): so nesse modo existe uma
  # associacao STA que realmente importa preservar, e blips de
  # varredura de fundo sao esperados. Em qualquer outro modo (ex.:
  # Ethernet para Wi-Fi), a interface foi deliberadamente desconectada/
  # desgerenciada antes do hotspot subir (unmanageWifiInterfaceIfIdle,
  # services/worker/controller/compose.go) - ela nunca vai "reconectar"
  # a nenhum STA, entao esperar aqui so soma segundos de espera inutil
  # a cada candidato de canal testado (rank_channels_for_band chama
  # try_create_ap uma vez por candidato - 8+ candidatos x 8s cada
  # atrasava a subida do hotspot em mais de um minuto).
  if [[ "${WIFI_INTERFACE}" != "${REAL_INTERNET_INTERFACE}" ]]; then
    # "if ... | grep -q; then return 0; fi; return 1" em vez de rodar o
    # pipe solto e devolver "$?" direto: um pipe solto (nao condicao de
    # if/while) com pipefail conta como comando falho pro set -e do
    # topo do script quando o grep nao acha nada (caso normal aqui,
    # placa desconectada de proposito) - risco confirmado ao vivo em
    # ensure_wifi_radio_unblocked (regulatory.sh), que matava o script
    # inteiro em silencio pelo mesmo motivo.
    if iw dev "${WIFI_INTERFACE}" link 2>/dev/null | grep -q '^Connected to '; then
      return 0
    fi
    return 1
  fi

  local attempts="${STA_LINK_CHECK_ATTEMPTS:-20}"
  local interval="${STA_LINK_CHECK_INTERVAL_SECONDS:-0.4}"
  local attempt
  for attempt in $(seq 1 "${attempts}"); do
    if iw dev "${WIFI_INTERFACE}" link 2>/dev/null | grep -q '^Connected to '; then
      return 0
    fi
    [[ "${attempt}" -lt "${attempts}" ]] && sleep "${interval}"
  done
  return 1
}

# sta_current_band_channel imprime "banda canal" (ex.: "2.4 6") a
# partir da frequencia atual de associacao Wi-Fi cliente de
# WIFI_INTERFACE ("freq:" de "iw dev <if> link"), ou falha se a
# interface nao estiver associada agora. Usada por try_create_ap
# (entrypoint.sh) para travar o AP no mesmo canal da estacao quando
# WIFI_INTERFACE e INTERNET_INTERFACE sao a mesma placa (AP+STA
# concorrente, Wi-Fi para Wi-Fi): um radio fisico so transmite numa
# frequencia por vez, entao o AP nao pode ficar num canal/banda
# diferente do link STA ativo nele - ignorar isso e o motivo mais
# comum de "cliente associa mas nunca completa o DHCP"/instabilidade
# nesse modo, mesmo quando o create_ap sobe sem erro aparente.
sta_current_band_channel() {
  local link_info
  local freq
  local channel

  # sta_link_connected ja absorve o mesmo blip de varredura de fundo
  # descrito ali (paciente, ate STA_LINK_CHECK_ATTEMPTS tentativas) -
  # sem isso, uma leitura pontual aqui podia falhar por causa desse
  # blip e mandar attempt_hotspot_cycle (entrypoint.sh) cair pro
  # caminho lento de varredura completa de canais em vez do caminho
  # rapido "trava direto no canal da estacao", exatamente pelo mesmo
  # motivo que motivou o debounce em primeiro lugar.
  sta_link_connected || return 1

  link_info="$(iw dev "${WIFI_INTERFACE}" link 2>/dev/null || true)"
  [[ "${link_info}" == "Connected to"* ]] || return 1

  freq="$(awk '/freq:/ {print $2; exit}' <<< "${link_info}")"
  [[ -n "${freq}" ]] || return 1

  channel="$(freq_to_channel "${freq}")"
  [[ -n "${channel}" ]] || return 1

  if (( ${freq%%.*} >= 5000 )); then
    printf '5 %s\n' "${channel}"
  else
    printf '2.4 %s\n' "${channel}"
  fi
}

channel_abs() {
  local value="$1"
  if (( value < 0 )); then
    printf '%s\n' "$((-value))"
  else
    printf '%s\n' "${value}"
  fi
}

score_channel() {
  local candidate="$1"
  local observed="$2"
  local distance

  if [[ "${WIFI_FREQ_BAND}" == "2.4" ]]; then
    distance="$(channel_abs "$((candidate - observed))")"
    if (( distance == 0 )); then
      printf '8\n'
    elif (( distance == 1 )); then
      printf '6\n'
    elif (( distance == 2 )); then
      printf '4\n'
    elif (( distance <= 4 )); then
      printf '1\n'
    else
      printf '0\n'
    fi
    return
  fi

  if (( candidate == observed )); then
    printf '1\n'
  else
    printf '0\n'
  fi
}

candidate_channels() {
  if [[ -n "${WIFI_CHANNEL_CANDIDATES:-}" ]]; then
    tr ',;' '  ' <<< "${WIFI_CHANNEL_CANDIDATES}"
    return
  fi

  case "${WIFI_FREQ_BAND}" in
    5) printf '36 40 44 48 149 153 157 161\n' ;;
    2.4) printf '1 6 11\n' ;;
    *)
      log "ERRO: WIFI_FREQ_BAND deve ser 2.4 ou 5 para selecao automatica de canal."
      exit 1
      ;;
  esac
}

# rank_channels_for_band preenche RANKED_CHANNELS (array global) com os
# canais candidatos da banda informada, ordenados do menos para o mais
# interferido - usada por start_hotspot_auto para tentar canal por canal
# ate um que o create_ap realmente consiga usar (nem toda banda/canal
# reportado como "suportado" pelo "iw phy info" e transmissivel de fato,
# ver ERRO "adapter can not transmit" do create_ap).
rank_channels_for_band() {
  local band="$1"
  local channels
  local scan
  local freq
  local observed_channel
  local observed_channels=()
  local candidate
  local score
  local -a scored=()

  WIFI_FREQ_BAND="${band}"
  channels="$(candidate_channels)"
  ip link set "${WIFI_INTERFACE}" up >/dev/null 2>&1 || true
  scan="$(iw dev "${WIFI_INTERFACE}" scan 2>/dev/null || iw dev "${WIFI_INTERFACE}" scan ap-force 2>/dev/null || true)"
  load_channel_history_scores "${band}"

  while read -r freq; do
    [[ -n "${freq}" ]] || continue
    observed_channel="$(freq_to_channel "${freq}")"
    [[ -n "${observed_channel}" ]] || continue
    observed_channels+=("${observed_channel}")
  done < <(awk '/freq:/ {print $2}' <<< "${scan}")

  for candidate in ${channels}; do
    if ! [[ "${candidate}" =~ ^[0-9]+$ ]]; then
      log "ERRO: WIFI_CHANNEL_CANDIDATES contem canal invalido: ${candidate}"
      exit 1
    fi

    # Historico (channel_history_score, history.sh) pesa muito mais que
    # interferencia ao vivo de proposito - um canal que ja falhou
    # consistentemente antes (ex.: "adapter can not transmit") deve
    # sempre ir pro fim da fila, mesmo que o scan ao vivo o mostre como
    # o menos congestionado agora; interferencia so desempata entre
    # candidatos com o mesmo historico (incluindo nenhum ainda).
    score="$(channel_history_score "${candidate}")"
    for observed_channel in "${observed_channels[@]}"; do
      score=$((score + $(score_channel "${candidate}" "${observed_channel}")))
    done
    scored+=("${score} ${candidate}")
  done

  RANKED_CHANNELS=()
  if [[ "${#scored[@]}" -gt 0 ]]; then
    while read -r pair; do
      RANKED_CHANNELS+=("${pair#* }")
    done < <(printf '%s\n' "${scored[@]}" | sort -n)
  fi

  if [[ -z "${scan}" ]]; then
    log "AVISO: varredura Wi-Fi indisponivel; ordem de canais candidatos (${band}GHz) e arbitraria."
  else
    log "Canais candidatos ordenados por interferencia (${band}GHz): ${RANKED_CHANNELS[*]:-nenhum} (redes avaliadas ${#observed_channels[@]})."
  fi
}
