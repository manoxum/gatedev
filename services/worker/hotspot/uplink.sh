#!/usr/bin/env bash
# uplink.sh - uplink virtual estavel (bn-uplink) do hotspot: NAT/
# forward para a fonte real de internet e o monitor que alterna essa
# fonte AO VIVO, sem reiniciar o create_ap. Extraido de interfaces.sh
# (que ficou so com resolucao/validacao de interfaces) pra manter cada
# arquivo focado num unico dominio (ver CLAUDE.md - limite de ~200
# linhas por arquivo). Sourced pelo entrypoint.sh depois de
# interfaces.sh (usa is_real_internet_interface/best_internet_interface
# e as variaveis BINDNET_UPLINK_INTERFACE/UPLINK_MONITOR_INTERVAL de
# la) e de sta_link.sh (usa sta_link_probe).

UPLINK_FILTER_CHAIN="BINDNET-HOTSPOT"
UPLINK_NAT_CHAIN="BINDNET-HOTSPOT"

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

# db_internet_strategy le o INTERNET_INTERFACE atual direto do banco -
# e o que permite o painel trocar a fonte de uplink com o hotspot no
# ar: como o create_ap so conhece o uplink virtual bn-uplink, mudar a
# fonte real e so retrocear o NAT, nunca reiniciar o AP. Vazio em
# falha de banco (o chamador mantem o valor atual).
db_internet_strategy() {
  psql_hotspot -Atqc "SELECT value FROM hotspot_config WHERE key = 'INTERNET_INTERFACE'" 2>/dev/null || true
}

# uplink_target_for_strategy resolve uma estrategia (auto/nome fixo)
# para a interface real utilizavel AGORA, ou nada se nao houver:
# - auto: melhor candidata (exclui WIFI_INTERFACE, ver
#   best_internet_interface);
# - a propria placa Wi-Fi (Wi-Fi para Wi-Fi): so vale com a associacao
#   STA de pe (sta_link_probe, que ja rejeita a placa em modo AP) - em
#   --no-virt nao ha estacao nenhuma pra servir de uplink, e ai a troca
#   ao vivo e impossivel (exige reiniciar o hotspot pra reassociar);
# - qualquer outra interface: precisa existir e ser elegivel.
uplink_target_for_strategy() {
  local strategy="$1"
  if [[ "${strategy}" == "auto" ]]; then
    best_internet_interface
    return 0
  fi
  if [[ "${strategy}" == "${WIFI_INTERFACE}" ]] && ! sta_link_probe; then
    return 0
  fi
  if ! is_real_internet_interface "${strategy}"; then
    return 0
  fi
  printf '%s\n' "${strategy}"
}

# refresh_internet_strategy_from_db realinha INTERNET_INTERFACE/
# INTERNET_STRATEGY/REAL_INTERNET_INTERFACE deste processo com o banco
# - chamada por attempt_hotspot_cycle (entrypoint.sh) antes de cada
# rodada: um retry pos-queda deve raciocinar (modo W2W ou nao, uplink
# de qual placa) com a fonte que o painel escolheu por ultimo, nao com
# a que estava valendo quando o processo nasceu.
refresh_internet_strategy_from_db() {
  local desired
  desired="$(db_internet_strategy)"
  [[ -n "${desired}" && "${desired}" != "${INTERNET_STRATEGY}" ]] || return 0
  log "INTERNET_INTERFACE mudou no painel: ${INTERNET_STRATEGY} -> ${desired}; adotando o novo valor nesta tentativa."
  INTERNET_INTERFACE="${desired}"
  INTERNET_STRATEGY="${desired}"
  resolve_internet_interface
}

# start_uplink_monitor roda SEMPRE (nao so em INTERNET_INTERFACE=auto,
# como era antes): alem de seguir a melhor interface no modo auto, ele
# detecta mudanca de INTERNET_INTERFACE feita pelo painel (via banco) e
# alterna o NAT ao vivo - o create_ap/hostapd e os clientes associados
# nem percebem, so a rota de saida muda. "warned" evita repetir o mesmo
# aviso a cada tick enquanto a situacao nao muda.
start_uplink_monitor() {
  (
    local strategy="${INTERNET_STRATEGY}"
    local current="${REAL_INTERNET_INTERFACE}"
    local desired detected warned=""

    while true; do
      sleep "${UPLINK_MONITOR_INTERVAL}"
      desired="$(db_internet_strategy)"
      [[ -n "${desired}" ]] || desired="${strategy}"
      if [[ "${desired}" != "${strategy}" ]]; then
        log "Fonte de internet alterada pelo painel: ${strategy} -> ${desired}; alternando ao vivo, sem reiniciar o hotspot."
        strategy="${desired}"
        warned=""
      fi
      detected="$(uplink_target_for_strategy "${strategy}")"
      if [[ -z "${detected}" ]]; then
        if [[ -z "${warned}" ]]; then
          log "AVISO: fonte de internet '${strategy}' sem interface utilizavel agora; mantendo ${current}."
          warned=1
        fi
        continue
      fi
      if [[ "${detected}" == "${current}" ]]; then
        warned=""
        continue
      fi
      if apply_bindnet_uplink_rules "${detected}"; then
        log "Uplink alternado de ${current} para ${detected} sem reiniciar o hotspot."
        current="${detected}"
        warned=""
      elif [[ -z "${warned}" ]]; then
        log "AVISO: nao foi possivel aplicar NAT/forward para ${detected}; mantendo ${current}."
        warned=1
      fi
    done
  ) &
  UPLINK_MONITOR_PID=$!
  log "Monitor de uplink ativo a cada ${UPLINK_MONITOR_INTERVAL}s (estrategia: ${INTERNET_STRATEGY}; fonte atual: ${REAL_INTERNET_INTERFACE})."
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
