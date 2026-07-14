#!/usr/bin/env bash
# interfaces.sh - resolucao/avisos sobre a interface de internet,
# sourced pelo entrypoint.sh. Usa "log" e "interface_phy" (definida em
# channel.sh) e as variaveis WIFI_INTERFACE/INTERNET_INTERFACE ja
# resolvidas pelo entrypoint.sh.

INTERNET_STRATEGY="${INTERNET_INTERFACE}"
REAL_INTERNET_INTERFACE=""
BINDNET_UPLINK_INTERFACE="${BINDNET_UPLINK_INTERFACE:-bn-uplink}"
CREATE_AP_INTERNET_INTERFACE="${BINDNET_UPLINK_INTERFACE}"
UPLINK_MONITOR_INTERVAL="${UPLINK_MONITOR_INTERVAL:-10}"
UPLINK_FILTER_CHAIN="BINDNET-HOTSPOT"
UPLINK_NAT_CHAIN="BINDNET-HOTSPOT"

# resolve_internet_interface so age quando INTERNET_INTERFACE=auto -
# mesma filosofia de WIFI_CHANNEL/WIFI_FREQ_BAND: "auto" e um valor
# explicito, nunca um comportamento implicito de variavel vazia (essa
# continua sendo barrada por "required" antes desta funcao rodar).
# Escolhe a melhor interface real entre as rotas padrao IPv4: maior
# velocidade reportada em /sys/class/net/<iface>/speed, e menor metric
# como desempate. Se nao houver rota padrao, tenta interfaces reais UP.
resolve_internet_interface() {
  if [[ "${INTERNET_STRATEGY}" != "auto" ]]; then
    REAL_INTERNET_INTERFACE="${INTERNET_STRATEGY}"
    return
  fi

  local detected
  detected="$(best_internet_interface)"
  if [[ -z "${detected}" ]]; then
    log "ERRO: INTERNET_INTERFACE=auto mas nenhuma interface de internet candidata foi encontrada."
    exit 1
  fi
  REAL_INTERNET_INTERFACE="${detected}"
  INTERNET_INTERFACE="${detected}"
  log "Interface de internet automatica escolhida: ${REAL_INTERNET_INTERFACE} ($(interface_speed_mbps "${REAL_INTERNET_INTERFACE}")Mbps reportados)."
}

# is_real_internet_interface so valida existencia/elegibilidade
# estrutural (nao e uma interface virtual/dummy/loopback) - de
# proposito NAO exclui WIFI_INTERFACE aqui: quando o usuario configura
# INTERNET_INTERFACE=WIFI_INTERFACE explicitamente (Wi-Fi para Wi-Fi,
# AP+STA concorrente no mesmo radio - ver aviso de
# warn_if_concurrent_ap_sta_risky), essa e a interface real que o
# create_ap vai mesmo usar como uplink, e validate_real_internet_interface/
# apply_bindnet_uplink_rules precisam aceita-la. A exclusao de
# WIFI_INTERFACE fica isolada em best_internet_interface (abaixo), que
# so roda no modo INTERNET_INTERFACE=auto.
is_real_internet_interface() {
  local iface="$1"
  [[ -d "/sys/class/net/${iface}" ]] || return 1
  [[ "${iface}" != "${BINDNET_UPLINK_INTERFACE}" ]] || return 1
  case "${iface}" in
    lo|ap0|ap[0-9]*|bn-*|docker*|br-*|veth*|virbr*|tun*|tap*|wg*) return 1 ;;
  esac
  return 0
}

interface_speed_mbps() {
  local iface="$1"
  local speed="0"
  if [[ -r "/sys/class/net/${iface}/speed" ]]; then
    speed="$(cat "/sys/class/net/${iface}/speed" 2>/dev/null || printf '0')"
  fi
  if [[ "${speed}" =~ ^[0-9]+$ ]] && [[ "${speed}" -gt 0 ]]; then
    printf '%s\n' "${speed}"
  else
    printf '0\n'
  fi
}

pick_better_interface() {
  local candidate="$1"
  local metric="$2"
  local speed
  speed="$(interface_speed_mbps "${candidate}")"

  if [[ -z "${BEST_IFACE:-}" ]] ||
     [[ "${speed}" -gt "${BEST_SPEED:-0}" ]] ||
     { [[ "${speed}" -eq "${BEST_SPEED:-0}" ]] && [[ "${metric}" -lt "${BEST_METRIC:-999999}" ]]; }; then
    BEST_IFACE="${candidate}"
    BEST_SPEED="${speed}"
    BEST_METRIC="${metric}"
  fi
}

# best_internet_interface so roda no modo INTERNET_INTERFACE=auto
# (resolve_internet_interface/start_uplink_monitor). Exclui
# WIFI_INTERFACE explicitamente aqui (e so aqui - ver comentario em
# is_real_internet_interface acima): sem isso, a deteccao automatica
# pode escolher o proprio radio do hotspot como "fonte de internet"
# quando ele aparece UP, alternando NAT/forward para ele a cada ciclo
# (UPLINK_MONITOR_INTERVAL) e derrubando o beacon/clientes associados.
# Isso nao se aplica quando o usuario escolhe INTERNET_INTERFACE
# explicitamente igual a WIFI_INTERFACE (Wi-Fi para Wi-Fi) - nesse
# caso resolve_internet_interface nem chama esta funcao.
best_internet_interface() {
  local line iface metric token index
  BEST_IFACE=""
  BEST_SPEED=0
  BEST_METRIC=999999

  while IFS= read -r line; do
    iface=""
    metric=0
    read -r -a tokens <<< "${line}"
    for index in "${!tokens[@]}"; do
      token="${tokens[$index]}"
      if [[ "${token}" == "dev" ]]; then
        iface="${tokens[$((index + 1))]:-}"
      elif [[ "${token}" == "metric" ]]; then
        metric="${tokens[$((index + 1))]:-0}"
      fi
    done
    [[ -n "${iface}" ]] || continue
    is_real_internet_interface "${iface}" || continue
    [[ "${iface}" != "${WIFI_INTERFACE}" ]] || continue
    pick_better_interface "${iface}" "${metric}"
  done < <(ip -o route show default 2>/dev/null || true)

  if [[ -n "${BEST_IFACE}" ]]; then
    printf '%s\n' "${BEST_IFACE}"
    return
  fi

  for path in /sys/class/net/*; do
    iface="${path##*/}"
    is_real_internet_interface "${iface}" || continue
    [[ "${iface}" != "${WIFI_INTERFACE}" ]] || continue
    [[ "$(cat "/sys/class/net/${iface}/operstate" 2>/dev/null || true)" == "up" ]] || continue
    pick_better_interface "${iface}" 999999
  done

  printf '%s\n' "${BEST_IFACE}"
}

# warn_if_concurrent_ap_sta_risky avisa cedo quando WIFI_INTERFACE e
# INTERNET_INTERFACE sao a mesma placa fisica (hotspot + internet no
# mesmo radio, modo AP+STA concorrente). Nunca bloqueia: quem decide se
# funciona de fato e o proprio create_ap, mesma filosofia ja usada para
# canal/banda ("o hotspot nunca trava por falta de varredura").
warn_if_concurrent_ap_sta_risky() {
  if [[ "${WIFI_INTERFACE}" != "${REAL_INTERNET_INTERFACE}" ]]; then
    return
  fi

  log "AVISO: WIFI_INTERFACE e INTERNET_INTERFACE sao a mesma placa (${WIFI_INTERFACE}) - modo AP+STA concorrente no mesmo radio, requer suporte do driver/chipset; o create_ap decide se funciona de fato."

  local phy
  phy="$(interface_phy)"
  if [[ -z "${phy}" ]]; then
    log "AVISO: nao foi possivel identificar o phy de ${WIFI_INTERFACE} para checar combinacoes suportadas."
    return
  fi

  if iw "phy${phy}" info 2>/dev/null | grep -A5 'valid interface combinations' | grep -qi 'AP.*managed\|managed.*AP'; then
    log "Placa ${WIFI_INTERFACE} (phy${phy}) reporta suporte a AP+managed simultaneos."
  else
    log "AVISO: 'iw phy${phy} info' nao reporta uma combinacao AP+managed simultanea; o create_ap pode falhar ao tentar hotspot+internet na mesma placa."
  fi
}

validate_real_internet_interface() {
  if [[ -z "${REAL_INTERNET_INTERFACE}" ]]; then
    log "ERRO: nenhuma interface real de internet foi resolvida."
    exit 1
  fi
  if ! is_real_internet_interface "${REAL_INTERNET_INTERFACE}"; then
    log "ERRO: interface de internet '${REAL_INTERNET_INTERFACE}' nao existe ou nao e uma interface real elegivel."
    exit 1
  fi
}

ensure_iptables_chain() {
  local table="$1"
  local chain="$2"
  if [[ -n "${table}" ]]; then
    iptables -w -t "${table}" -N "${chain}" >/dev/null 2>&1 || true
  else
    iptables -w -N "${chain}" >/dev/null 2>&1 || true
  fi
}

ensure_iptables_jump() {
  local table="$1"
  local parent="$2"
  local chain="$3"
  if [[ -n "${table}" ]]; then
    if ! iptables -w -t "${table}" -C "${parent}" -j "${chain}" >/dev/null 2>&1; then
      iptables -w -t "${table}" -I "${parent}" 1 -j "${chain}"
    fi
  elif ! iptables -w -C "${parent}" -j "${chain}" >/dev/null 2>&1; then
    iptables -w -I "${parent}" 1 -j "${chain}"
  fi
}

remove_iptables_jump() {
  local table="$1"
  local parent="$2"
  local chain="$3"
  if [[ -n "${table}" ]]; then
    while iptables -w -t "${table}" -C "${parent}" -j "${chain}" >/dev/null 2>&1; do
      iptables -w -t "${table}" -D "${parent}" -j "${chain}" >/dev/null 2>&1 || break
    done
  else
    while iptables -w -C "${parent}" -j "${chain}" >/dev/null 2>&1; do
      iptables -w -D "${parent}" -j "${chain}" >/dev/null 2>&1 || break
    done
  fi
}

ensure_bindnet_uplink_chains() {
  ensure_iptables_chain "" "${UPLINK_FILTER_CHAIN}"
  ensure_iptables_chain "nat" "${UPLINK_NAT_CHAIN}"
  ensure_iptables_jump "" "FORWARD" "${UPLINK_FILTER_CHAIN}"
  ensure_iptables_jump "nat" "POSTROUTING" "${UPLINK_NAT_CHAIN}"
}

apply_bindnet_uplink_rules() {
  local iface="$1"

  is_real_internet_interface "${iface}" || return 1
  ensure_bindnet_uplink_chains

  iptables -w -F "${UPLINK_FILTER_CHAIN}"
  iptables -w -t nat -F "${UPLINK_NAT_CHAIN}"
  iptables -w -A "${UPLINK_FILTER_CHAIN}" -s "${HOTSPOT_CIDR}" -o "${iface}" -j ACCEPT
  iptables -w -A "${UPLINK_FILTER_CHAIN}" -i "${iface}" -d "${HOTSPOT_CIDR}" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  iptables -w -t nat -A "${UPLINK_NAT_CHAIN}" -s "${HOTSPOT_CIDR}" -o "${iface}" -j MASQUERADE

  REAL_INTERNET_INTERFACE="${iface}"
  INTERNET_INTERFACE="${iface}"
  local interface_display="${REAL_INTERNET_INTERFACE}"
  [[ "${INTERNET_STRATEGY}" == "auto" ]] && interface_display="auto (${REAL_INTERNET_INTERFACE})"
  log "Fonte real de internet do hotspot: ${interface_display}; create_ap recebe uplink virtual estavel ${CREATE_AP_INTERNET_INTERFACE}."
}

setup_bindnet_virtual_uplink() {
  validate_real_internet_interface

  if ! ip link show "${BINDNET_UPLINK_INTERFACE}" >/dev/null 2>&1; then
    ip link add "${BINDNET_UPLINK_INTERFACE}" type dummy
  fi
  ip link set "${BINDNET_UPLINK_INTERFACE}" up
  sysctl -w net.ipv4.ip_forward=1 >/dev/null

  if ! apply_bindnet_uplink_rules "${REAL_INTERNET_INTERFACE}"; then
    log "ERRO: nao foi possivel aplicar NAT/forward para ${REAL_INTERNET_INTERFACE}."
    exit 1
  fi
}

start_uplink_monitor() {
  if [[ "${INTERNET_STRATEGY}" != "auto" ]]; then
    return
  fi

  (
    local current="${REAL_INTERNET_INTERFACE}"
    local detected=""

    while true; do
      sleep "${UPLINK_MONITOR_INTERVAL}"
      detected="$(best_internet_interface)"
      if [[ -z "${detected}" ]]; then
        log "AVISO: monitor auto nao encontrou interface de internet candidata; mantendo ${current}."
        continue
      fi
      if [[ "${detected}" == "${current}" ]]; then
        continue
      fi
      if apply_bindnet_uplink_rules "${detected}"; then
        log "Monitor auto alternou a fonte de internet de ${current} para ${detected} sem reiniciar o hotspot."
        current="${detected}"
      else
        log "AVISO: monitor auto detectou ${detected}, mas nao conseguiu aplicar regras; mantendo ${current}."
      fi
    done
  ) &
  UPLINK_MONITOR_PID=$!
  log "Monitor automatico de internet ativo a cada ${UPLINK_MONITOR_INTERVAL}s."
}

cleanup_bindnet_uplink() {
  if [[ -n "${UPLINK_MONITOR_PID:-}" ]]; then
    kill "${UPLINK_MONITOR_PID}" >/dev/null 2>&1 || true
    wait "${UPLINK_MONITOR_PID}" >/dev/null 2>&1 || true
    UPLINK_MONITOR_PID=""
  fi

  remove_iptables_jump "" "FORWARD" "${UPLINK_FILTER_CHAIN}"
  remove_iptables_jump "nat" "POSTROUTING" "${UPLINK_NAT_CHAIN}"
  iptables -w -F "${UPLINK_FILTER_CHAIN}" >/dev/null 2>&1 || true
  iptables -w -X "${UPLINK_FILTER_CHAIN}" >/dev/null 2>&1 || true
  iptables -w -t nat -F "${UPLINK_NAT_CHAIN}" >/dev/null 2>&1 || true
  iptables -w -t nat -X "${UPLINK_NAT_CHAIN}" >/dev/null 2>&1 || true

  ip link delete "${BINDNET_UPLINK_INTERFACE}" >/dev/null 2>&1 || true
}
