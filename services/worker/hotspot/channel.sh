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

# sta_link_connected/sta_current_band_channel moraram aqui ate serem
# extraidas para sta_link.sh (estado da associacao STA e um dominio
# proprio, e a leitura via "iw dev link" era falso-negativa - ver o
# comentario no topo de sta_link.sh).

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
