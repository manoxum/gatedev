#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[hotspot] %s\n' "$*"
}

obrigatoria() {
  local nome="$1"
  if [[ -z "${!nome:-}" ]]; then
    log "ERRO: variavel obrigatoria ausente: ${nome}"
    exit 1
  fi
}

obrigatoria WIFI_INTERFACE
obrigatoria INTERNET_INTERFACE
obrigatoria WIFI_SSID
obrigatoria WIFI_PASSWORD

HOTSPOT_GATEWAY="${HOTSPOT_GATEWAY:-192.168.12.1}"
WIFI_COUNTRY="${WIFI_COUNTRY:-ST}"
if [[ -z "${WIFI_CHANNEL:-}" && -n "${WIFI_CHANNE:-}" ]]; then
  WIFI_CHANNEL="${WIFI_CHANNE}"
  log "AVISO: usando WIFI_CHANNE como fallback; prefira WIFI_CHANNEL."
fi
WIFI_CHANNEL="${WIFI_CHANNEL:-auto}"
WIFI_FREQ_BAND="${WIFI_FREQ_BAND:-2.4}"

interface_phy() {
  iw dev "${WIFI_INTERFACE}" info 2>/dev/null | awk '/wiphy/{print $2}'
}

banda_suportada() {
  local banda="$1"
  local phy
  local info

  phy="$(interface_phy)"
  [[ -n "${phy}" ]] || return 1

  info="$(iw "phy${phy}" info 2>/dev/null || true)"
  [[ -n "${info}" ]] || return 1

  if [[ "${banda}" == "5" ]]; then
    grep -Eq '\* 5[0-9]{3}(\.[0-9])? MHz' <<< "${info}"
  else
    grep -Eq '\* 24[0-9]{2}(\.[0-9])? MHz' <<< "${info}"
  fi
}

resolver_banda_wifi() {
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

  if banda_suportada "5"; then
    WIFI_FREQ_BAND="5"
  elif banda_suportada "2.4"; then
    WIFI_FREQ_BAND="2.4"
  else
    WIFI_FREQ_BAND="2.4"
    log "AVISO: nao foi possivel detectar as bandas suportadas por ${WIFI_INTERFACE}; usando 2.4GHz."
  fi
  log "Banda Wi-Fi automatica escolhida: ${WIFI_FREQ_BAND}GHz."
}

freq_para_canal() {
  local freq="$1"

  if (( freq == 2484 )); then
    printf '14\n'
  elif (( freq >= 2412 && freq <= 2472 )); then
    printf '%s\n' "$(((freq - 2407) / 5))"
  elif (( freq >= 5000 && freq <= 5900 )); then
    printf '%s\n' "$(((freq - 5000) / 5))"
  fi
}

canal_abs() {
  local valor="$1"
  if (( valor < 0 )); then
    printf '%s\n' "$((-valor))"
  else
    printf '%s\n' "${valor}"
  fi
}

pontuar_canal() {
  local candidato="$1"
  local observado="$2"
  local distancia

  if [[ "${WIFI_FREQ_BAND}" == "2.4" ]]; then
    distancia="$(canal_abs "$((candidato - observado))")"
    if (( distancia == 0 )); then
      printf '8\n'
    elif (( distancia == 1 )); then
      printf '6\n'
    elif (( distancia == 2 )); then
      printf '4\n'
    elif (( distancia <= 4 )); then
      printf '1\n'
    else
      printf '0\n'
    fi
    return
  fi

  if (( candidato == observado )); then
    printf '1\n'
  else
    printf '0\n'
  fi
}

canais_candidatos() {
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

selecionar_canal_wifi() {
  local canais
  local scan
  local freq
  local canal_observado
  local canais_observados=()
  local candidato
  local pontuacao
  local melhor_canal=""
  local melhor_pontuacao=""

  if [[ "${WIFI_CHANNEL}" != "auto" ]]; then
    if ! [[ "${WIFI_CHANNEL}" =~ ^[0-9]+$ ]]; then
      log "ERRO: WIFI_CHANNEL deve ser numerico ou auto."
      exit 1
    fi
    return
  fi

  canais="$(canais_candidatos)"
  ip link set "${WIFI_INTERFACE}" up >/dev/null 2>&1 || true
  scan="$(iw dev "${WIFI_INTERFACE}" scan 2>/dev/null || iw dev "${WIFI_INTERFACE}" scan ap-force 2>/dev/null || true)"

  while read -r freq; do
    [[ -n "${freq}" ]] || continue
    canal_observado="$(freq_para_canal "${freq}")"
    [[ -n "${canal_observado}" ]] || continue
    canais_observados+=("${canal_observado}")
  done < <(awk '/freq:/ {print $2}' <<< "${scan}")

  for candidato in ${canais}; do
    if ! [[ "${candidato}" =~ ^[0-9]+$ ]]; then
      log "ERRO: WIFI_CHANNEL_CANDIDATES contem canal invalido: ${candidato}"
      exit 1
    fi

    pontuacao=0
    for canal_observado in "${canais_observados[@]}"; do
      pontuacao=$((pontuacao + $(pontuar_canal "${candidato}" "${canal_observado}")))
    done

    if [[ -z "${melhor_pontuacao}" || "${pontuacao}" -lt "${melhor_pontuacao}" ]]; then
      melhor_canal="${candidato}"
      melhor_pontuacao="${pontuacao}"
    fi
  done

  if [[ -z "${melhor_canal}" ]]; then
    log "ERRO: nenhum canal candidato disponivel para WIFI_FREQ_BAND=${WIFI_FREQ_BAND}."
    exit 1
  fi

  WIFI_CHANNEL="${melhor_canal}"
  if [[ -z "${scan}" ]]; then
    log "AVISO: varredura Wi-Fi indisponivel; usando canal ${WIFI_CHANNEL}."
  else
    log "Canal automatico escolhido: ${WIFI_CHANNEL} (pontuacao ${melhor_pontuacao}; redes avaliadas ${#canais_observados[@]})."
  fi
}

CREATE_AP_HELP="$(create_ap --help 2>&1 || true)"

if ! grep -q -- '--no-dns' <<< "${CREATE_AP_HELP}"; then
  log "ERRO: a versao baixada do create_ap nao suporta --no-dns."
  exit 1
fi

if ! grep -q -- '--dhcp-dns' <<< "${CREATE_AP_HELP}"; then
  log "ERRO: a versao baixada do create_ap nao suporta --dhcp-dns."
  exit 1
fi

resolver_banda_wifi
selecionar_canal_wifi

log "Preparando hotspot '${WIFI_SSID}' em ${WIFI_INTERFACE}, internet via ${INTERNET_INTERFACE}."
log "Regiao Wi-Fi: ${WIFI_COUNTRY}; banda: ${WIFI_FREQ_BAND}GHz; canal: ${WIFI_CHANNEL}."
log "Gateway do hotspot: ${HOTSPOT_GATEWAY}; DNS entregue por DHCP: ${HOTSPOT_GATEWAY}."

cleanup() {
  log "Encerrando hotspot em ${WIFI_INTERFACE}."
  create_ap --stop "${WIFI_INTERFACE}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

create_ap \
  --no-dns \
  --dhcp-dns "${HOTSPOT_GATEWAY}" \
  --country "${WIFI_COUNTRY}" \
  --freq-band "${WIFI_FREQ_BAND}" \
  -c "${WIFI_CHANNEL}" \
  -g "${HOTSPOT_GATEWAY}" \
  "${WIFI_INTERFACE}" \
  "${INTERNET_INTERFACE}" \
  "${WIFI_SSID}" \
  "${WIFI_PASSWORD}"
