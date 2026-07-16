#!/usr/bin/env bash
# uplink.sh - mecanica do uplink virtual estavel (bn-uplink) do
# hotspot: NAT/forward + roteamento por politica para a fonte real de
# internet. A decisao de QUANDO trocar a fonte (saude, failover,
# mudanca pelo painel) mora em uplink_monitor.sh. Sourced pelo
# entrypoint.sh depois de interfaces.sh (usa
# is_real_internet_interface e as variaveis
# BINDNET_UPLINK_INTERFACE/UPLINK_MONITOR_INTERVAL de la).

UPLINK_FILTER_CHAIN="BINDNET-HOTSPOT"
UPLINK_NAT_CHAIN="BINDNET-HOTSPOT"

# Roteamento por politica: o MASQUERADE "-o <iface>" sozinho NAO muda
# por onde o trafego sai - quem decide e a tabela de rotas do host, e
# com duas rotas default simultaneas (ex.: Ethernet metric 100 +
# Wi-Fi metric 600) o kernel continuaria saindo pela de menor metric,
# ignorando a fonte escolhida aqui. As duas ip rules abaixo fazem o
# trafego do HOTSPOT_CIDR resolver destinos locais/LAN pela tabela
# main normalmente (suppress_prefixlength 0 ignora SO as rotas default
# dela) e buscar a rota default na tabela dedicada - que aponta sempre
# pra fonte de internet escolhida. "91" ecoa o prefixo interno
# 10.91.x do stack; as prioridades ficam antes da main (32766).
UPLINK_ROUTE_TABLE=91
UPLINK_RULE_PRIORITY_LOCAL=30090
UPLINK_RULE_PRIORITY_DEFAULT=30091

ensure_bindnet_uplink_ip_rules() {
  if ! ip rule show | grep -q "^${UPLINK_RULE_PRIORITY_LOCAL}:"; then
    ip rule add priority "${UPLINK_RULE_PRIORITY_LOCAL}" from "${HOTSPOT_CIDR}" lookup main suppress_prefixlength 0
  fi
  if ! ip rule show | grep -q "^${UPLINK_RULE_PRIORITY_DEFAULT}:"; then
    ip rule add priority "${UPLINK_RULE_PRIORITY_DEFAULT}" from "${HOTSPOT_CIDR}" lookup "${UPLINK_ROUTE_TABLE}"
  fi
}

apply_bindnet_uplink_route() {
  local iface="$1"
  local gateway
  gateway="$(ip -4 route show default dev "${iface}" 2>/dev/null \
    | awk '{ for (i = 1; i < NF; i++) if ($i == "via") { print $(i + 1); exit } }')"
  if [[ -n "${gateway}" ]]; then
    ip route replace default via "${gateway}" dev "${iface}" table "${UPLINK_ROUTE_TABLE}"
  else
    # Sem gateway conhecido na main (ex.: link ponto-a-ponto ou ainda
    # sem rota default): rota direta pelo proprio device.
    ip route replace default dev "${iface}" table "${UPLINK_ROUTE_TABLE}"
  fi
  # Fluxos ja estabelecidos ficariam presos ao SNAT/rota antigos pelo
  # conntrack mesmo com a rota nova - derruba as entradas dos clientes
  # do hotspot pra que reconectem ja pela fonte nova. Best-effort: sem
  # o binario conntrack, os fluxos antigos so expiram sozinhos.
  conntrack -D -s "${HOTSPOT_CIDR}" >/dev/null 2>&1 || true
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

  ensure_bindnet_uplink_ip_rules
  apply_bindnet_uplink_route "${iface}"

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

# start_uplink_monitor (e toda a logica de saude/failover/troca pelo
# painel) mora em uplink_monitor.sh - ver o comentario no topo de la.

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

  ip rule del priority "${UPLINK_RULE_PRIORITY_LOCAL}" >/dev/null 2>&1 || true
  ip rule del priority "${UPLINK_RULE_PRIORITY_DEFAULT}" >/dev/null 2>&1 || true
  ip route flush table "${UPLINK_ROUTE_TABLE}" >/dev/null 2>&1 || true

  ip link delete "${BINDNET_UPLINK_INTERFACE}" >/dev/null 2>&1 || true
}
