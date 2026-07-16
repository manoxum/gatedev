#!/usr/bin/env bash
# uplink_monitor.sh - saude da fonte de internet e failover automatico
# do uplink, separado de uplink.sh (que tem so a mecanica de NAT/rotas)
# pra manter cada arquivo focado num unico dominio (ver CLAUDE.md).
# Sourced pelo entrypoint.sh depois de uplink.sh (usa
# apply_bindnet_uplink_rules/db_internet_strategy de la,
# ranked_internet_candidates/best_internet_interface de interfaces.sh e
# sta_link_probe de sta_link.sh).
#
# No modo INTERNET_INTERFACE=auto o monitor verifica a SAUDE da fonte
# atual a cada UPLINK_HEALTH_CHECK_INTERVAL segundos (link fisico via
# carrier + internet de verdade via ping pela propria interface): se a
# fonte cair ou perder internet - mesmo mantendo rota default e link -
# alterna imediatamente para a proxima candidata COM internet
# comprovada, sem reiniciar o hotspot. A reavaliacao de "apareceu uma
# candidata melhor?" e a leitura de mudanca pelo painel continuam na
# cadencia mais lenta de UPLINK_MONITOR_INTERVAL.

# UPLINK_HEALTH_CHECK_INTERVAL/_FAILURES_THRESHOLD: env do container,
# sem equivalente no painel (mesmo padrao de STA_LINK_CHECK_ATTEMPTS em
# sta_link.sh). O threshold exige N checagens de internet ruins
# SEGUIDAS antes de declarar a fonte morta - confirmado ao vivo que um
# unico ping perdido e normal na STA Wi-Fi (o radio e compartilhado
# com o proprio AP) e que failover em 1 falha causava ping-pong entre
# as fontes. Link fisico caido (carrier) nunca espera o threshold.
UPLINK_HEALTH_CHECK_INTERVAL="${UPLINK_HEALTH_CHECK_INTERVAL:-3}"
UPLINK_HEALTH_FAILURES_THRESHOLD="${UPLINK_HEALTH_FAILURES_THRESHOLD:-2}"

# uplink_probe_targets: alvos do teste de internet - reusa
# HOTSPOT_DNS_FALLBACKS (ja configuravel pelo painel e garantidamente
# IPs publicos que respondem ICMP, ex.: 1.1.1.1/8.8.8.8).
uplink_probe_targets() {
  local raw="${HOTSPOT_DNS_FALLBACKS:-1.1.1.1,8.8.8.8}"
  raw="${raw//;/,}"
  raw="${raw// /,}"
  tr ',' ' ' <<< "${raw}"
}

interface_link_up() {
  [[ "$(cat "/sys/class/net/$1/carrier" 2>/dev/null)" == "1" ]]
}

# interface_has_internet pinga os alvos PELA interface informada
# (ping -I, SO_BINDTODEVICE) - e o que detecta "tem link mas nao tem
# internet" (ex.: cabo ligado num roteador sem upstream). Timeout de
# 2s por alvo (1s dava falso-negativo na STA Wi-Fi, cujo radio e
# compartilhado com o AP e responde com latencia); primeiro alvo que
# responder aprova.
interface_has_internet() {
  local iface="$1"
  local target
  for target in $(uplink_probe_targets); do
    if ping -c 1 -W 2 -I "${iface}" "${target}" >/dev/null 2>&1; then
      return 0
    fi
  done
  return 1
}

uplink_healthy() {
  local iface="$1"
  interface_link_up "${iface}" && interface_has_internet "${iface}"
}

# best_uplink_with_internet percorre as candidatas do modo auto em
# ordem de preferencia e devolve a primeira com internet COMPROVADA -
# nada alem disso, de proposito: quando nenhuma comprova, trocar "no
# escuro" pra melhor por rota/velocidade causava ping-pong entre duas
# fontes igualmente sem internet (confirmado ao vivo); nesse caso o
# monitor mantem a fonte atual.
best_uplink_with_internet() {
  local candidate
  for candidate in $(ranked_internet_candidates); do
    if uplink_healthy "${candidate}"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  return 0
}

# first_carrier_candidate_except: fallback duro pro caso "link fisico
# da fonte atual morreu e nenhuma candidata comprova internet" -
# qualquer interface com carrier e melhor que uma sem link nenhum.
first_carrier_candidate_except() {
  local skip="$1"
  local candidate
  for candidate in $(ranked_internet_candidates); do
    [[ "${candidate}" != "${skip}" ]] || continue
    if interface_link_up "${candidate}"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  return 0
}

# uplink_target_for_strategy resolve uma estrategia FIXA (nome de
# interface) para a interface utilizavel agora, ou nada se nao houver:
# a propria placa Wi-Fi (Wi-Fi para Wi-Fi) so vale com a associacao
# STA de pe (sta_link_probe, que ja rejeita a placa em modo AP) - em
# --no-virt nao ha estacao pra servir de uplink e a troca ao vivo e
# impossivel (exige reiniciar o hotspot pra reassociar). O modo auto
# nao passa por aqui (ver best_uplink_with_internet).
uplink_target_for_strategy() {
  local strategy="$1"
  if [[ "${strategy}" == "${WIFI_INTERFACE}" ]] && ! sta_link_probe; then
    return 0
  fi
  if ! is_real_internet_interface "${strategy}"; then
    return 0
  fi
  printf '%s\n' "${strategy}"
}

# start_uplink_monitor roda SEMPRE (qualquer estrategia): detecta
# mudanca de INTERNET_INTERFACE feita pelo painel (via banco) e, no
# modo auto, faz a checagem de saude/failover descrita no topo. Toda
# troca passa por apply_bindnet_uplink_rules (NAT + rota por politica
# + derrubada de conntrack) e loga "Fonte real de internet do
# hotspot: ..." - e dessa linha que GET /api/hotspot/status extrai a
# interface em uso pro painel (parseHotspotInternetInterface,
# services/backend/hotspot_status.go), entao a troca fisica e a visual
# andam sempre juntas. "warned" evita repetir o mesmo aviso a cada
# tick enquanto a situacao nao muda.
start_uplink_monitor() {
  (
    # Mata o "ip monitor link" (e qualquer outro filho) quando o
    # proprio monitor morre (cleanup_bindnet_uplink mata este subshell).
    trap 'kill $(jobs -p) >/dev/null 2>&1 || true' EXIT

    local strategy="${INTERNET_STRATEGY}"
    local current="${REAL_INTERNET_INTERFACE}"
    local desired detected interval warned=""
    local since_recheck="${UPLINK_MONITOR_INTERVAL}"
    local health_failures=0
    local event last_tick now remaining

    # Eventos de link do kernel (netlink, via "ip monitor link")
    # acordam o loop NO INSTANTE em que qualquer interface muda de
    # estado - sem isso, desligar a placa a mao so era percebido no
    # tick de polling seguinte (ate ~10s somando intervalo + sondas,
    # medido ao vivo). O "read -t" abaixo dorme o intervalo normal OU
    # retorna na hora quando um evento chega; eventos de interfaces
    # que nao sao a fonte atual (veth do Docker etc. mudam toda hora)
    # sao ignorados sem antecipar o tick, e "last_tick" garante que o
    # tick periodico nunca passa fome mesmo sob rajada de eventos.
    exec 3< <(ip monitor link 2>/dev/null || true)
    last_tick="$(date +%s)"

    while true; do
      interval="${UPLINK_MONITOR_INTERVAL}"
      [[ "${strategy}" == "auto" ]] && interval="${UPLINK_HEALTH_CHECK_INTERVAL}"

      now="$(date +%s)"
      remaining=$((interval - (now - last_tick)))
      if (( remaining > 0 )); then
        event=""
        if read -t "${remaining}" -r -u 3 event; then
          case "${event}" in
            *"${current}"*) ;;
            *) continue ;;
          esac
        fi
      fi
      now="$(date +%s)"
      since_recheck=$((since_recheck + now - last_tick))
      last_tick="${now}"

      desired="$(db_internet_strategy)"
      [[ -n "${desired}" ]] || desired="${strategy}"
      if [[ "${desired}" != "${strategy}" ]]; then
        log "Fonte de internet alterada pelo painel: ${strategy} -> ${desired}; alternando ao vivo, sem reiniciar o hotspot."
        strategy="${desired}"
        warned=""
        health_failures=0
        since_recheck="${UPLINK_MONITOR_INTERVAL}"
      fi

      if [[ "${strategy}" == "auto" ]]; then
        if ! interface_link_up "${current}"; then
          # Link fisico morto e definitivo - failover na hora, sem
          # esperar o threshold de falhas de internet.
          detected="$(best_uplink_with_internet)"
          [[ -n "${detected}" ]] || detected="$(first_carrier_candidate_except "${current}")"
          since_recheck=0
          health_failures=0
          if [[ -n "${detected}" && "${detected}" != "${current}" ]]; then
            log "AVISO: fonte de internet atual ${current} perdeu o link fisico; alternando imediatamente para ${detected}."
          elif [[ -z "${warned}" ]]; then
            log "AVISO: fonte de internet atual ${current} sem link fisico e nenhuma outra candidata utilizavel; mantendo ${current} e reavaliando a cada ${UPLINK_HEALTH_CHECK_INTERVAL}s."
            warned=1
          fi
        elif ! interface_has_internet "${current}"; then
          health_failures=$((health_failures + 1))
          if (( health_failures < UPLINK_HEALTH_FAILURES_THRESHOLD )); then
            continue
          fi
          detected="$(best_uplink_with_internet)"
          since_recheck=0
          if [[ -n "${detected}" && "${detected}" != "${current}" ]]; then
            log "AVISO: fonte de internet atual ${current} perdeu a internet (${health_failures} checagens seguidas); alternando imediatamente para ${detected}."
            health_failures=0
          elif [[ -z "${warned}" ]]; then
            log "AVISO: fonte de internet atual ${current} sem internet e nenhuma outra candidata com internet comprovada; mantendo ${current} e reavaliando a cada ${UPLINK_HEALTH_CHECK_INTERVAL}s."
            warned=1
          fi
        elif (( since_recheck >= UPLINK_MONITOR_INTERVAL )); then
          # Fonte atual saudavel: reavalia com calma se apareceu uma
          # candidata melhor (ex.: Ethernet mais rapida religada).
          detected="$(best_uplink_with_internet)"
          since_recheck=0
          health_failures=0
          warned=""
        else
          health_failures=0
          warned=""
          continue
        fi
      else
        detected="$(uplink_target_for_strategy "${strategy}")"
      fi

      if [[ -z "${detected}" ]]; then
        if [[ -z "${warned}" ]]; then
          log "AVISO: fonte de internet '${strategy}' sem interface utilizavel agora; mantendo ${current}."
          warned=1
        fi
        continue
      fi
      if [[ "${detected}" == "${current}" ]]; then
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
  log "Monitor de uplink ativo (estrategia: ${INTERNET_STRATEGY}; fonte atual: ${REAL_INTERNET_INTERFACE}; queda de link detectada por evento netlink na hora, saude a cada ${UPLINK_HEALTH_CHECK_INTERVAL}s no modo auto, painel/reavaliacao a cada ${UPLINK_MONITOR_INTERVAL}s)."
}
