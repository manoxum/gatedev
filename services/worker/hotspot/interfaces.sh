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

# auto_candidate_allowed decide se uma interface pode entrar na
# selecao automatica (INTERNET_INTERFACE=auto). A placa do hotspot
# (WIFI_INTERFACE) so entra quando esta associada como ESTACAO agora
# (sta_link_probe, sta_link.sh - que ja rejeita a placa em modo AP):
# nesse estado ela e uma fonte de internet legitima e o unico fallback
# possivel numa maquina com uma unica porta Ethernet. Sem o probe, a
# deteccao automatica podia escolher o proprio radio do AP como "fonte
# de internet" quando ele aparece UP, alternando NAT/forward para ele
# e derrubando o beacon/clientes associados.
auto_candidate_allowed() {
  local iface="$1"
  is_real_internet_interface "${iface}" || return 1
  if [[ "${iface}" == "${WIFI_INTERFACE}" ]]; then
    sta_link_probe || return 1
  fi
  return 0
}

# ranked_internet_candidates imprime as candidatas do modo auto em
# ordem de preferencia: primeiro interfaces com rota padrao IPv4
# (maior velocidade reportada em /sys/class/net/<iface>/speed; metric
# menor desempata - Wi-Fi nao reporta speed e fica atras de qualquer
# Ethernet, de proposito), depois interfaces com link fisico ativo
# (carrier) mas sem rota padrao, como ultimo recurso. Usada por
# best_internet_interface e pelo monitor de uplink
# (uplink_monitor.sh), que percorre a lista testando internet de
# verdade em cada candidata. Campos de ordenacao com largura fixa
# (sort lexicografico do BusyBox funciona como numerico).
ranked_internet_candidates() {
  local line iface metric speed
  {
    while IFS= read -r line; do
      iface=""
      metric=0
      read -r -a tokens <<< "${line}"
      for index in "${!tokens[@]}"; do
        if [[ "${tokens[$index]}" == "dev" ]]; then
          iface="${tokens[$((index + 1))]:-}"
        elif [[ "${tokens[$index]}" == "metric" ]]; then
          metric="${tokens[$((index + 1))]:-0}"
        fi
      done
      [[ -n "${iface}" ]] || continue
      auto_candidate_allowed "${iface}" || continue
      speed="$(interface_speed_mbps "${iface}")"
      printf '0 %06d %06d %s\n' "$((999999 - speed))" "${metric}" "${iface}"
    done < <(ip -o route show default 2>/dev/null || true)

    for path in /sys/class/net/*; do
      iface="${path##*/}"
      auto_candidate_allowed "${iface}" || continue
      [[ "$(cat "/sys/class/net/${iface}/carrier" 2>/dev/null)" == "1" ]] || continue
      speed="$(interface_speed_mbps "${iface}")"
      printf '1 %06d 999999 %s\n' "$((999999 - speed))" "${iface}"
    done
  } | sort | awk '{ print $NF }' | awk '!seen[$0]++'
}

# best_internet_interface so roda no modo INTERNET_INTERFACE=auto
# (resolve_internet_interface e o fallback do monitor de uplink) - a
# melhor candidata por rota/velocidade, SEM testar internet de
# verdade; quem testa e best_uplink_with_internet (uplink_monitor.sh).
best_internet_interface() {
  ranked_internet_candidates | head -n 1
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

# As funcoes de NAT/uplink virtual e o monitor de troca ao vivo
# (apply_bindnet_uplink_rules, setup_bindnet_virtual_uplink,
# start_uplink_monitor, cleanup_bindnet_uplink etc.) moraram aqui ate
# serem extraidas para uplink.sh - ver o comentario no topo de la.
