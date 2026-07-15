#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[hotspot] %s\n' "$*"
}

CONTROL_DIR="${HOTSPOT_CONTROL_DIR:-/run/bindnet-admin/hotspot}"
SERVICE_LOG="${HOTSPOT_SERVICE_LOG:-/tmp/bindnet-hotspot-service.log}"
RUNNER_PID_FILE="${CONTROL_DIR}/runner.pid"
COMMAND="${1:-manager}"

db_host() {
  printf '%s\n' "${POSTGRES_HOST:-10.91.0.10}"
}

db_port() {
  printf '%s\n' "${POSTGRES_PORT:-5432}"
}

db_user() {
  printf '%s\n' "${POSTGRES_USER:-bindnet}"
}

db_name() {
  printf '%s\n' "${POSTGRES_DB:-bindnet}"
}

psql_hotspot() {
  PGPASSWORD="${POSTGRES_PASSWORD:-}" psql \
    -h "$(db_host)" \
    -p "$(db_port)" \
    -U "$(db_user)" \
    -d "$(db_name)" \
    "$@"
}

wait_for_hotspot_config_table() {
  local attempts="${HOTSPOT_DB_WAIT_ATTEMPTS:-90}"
  local ready
  for attempt in $(seq 1 "${attempts}"); do
    ready="$(psql_hotspot -Atqc "SELECT to_regclass('public.hotspot_config') IS NOT NULL" 2>/dev/null || true)"
    if [[ "${ready}" == "t" ]]; then
      return 0
    fi
    log "Aguardando banco de dados/migrations para carregar configuracao do hotspot (${attempt}/${attempts})."
    sleep 2
  done
  log "ERRO: banco de dados indisponivel ou tabela hotspot_config ausente."
  return 1
}

load_runtime_config_from_db() {
  wait_for_hotspot_config_table || return 1

  INTERNET_INTERFACE="auto"
  WIFI_OPEN="false"
  WIFI_COUNTRY="ST"
  WIFI_CHANNEL="auto"
  WIFI_FREQ_BAND="auto"
  HOTSPOT_GATEWAY="192.168.12.1"
  HOTSPOT_CIDR="192.168.12.0/24"
  HOTSPOT_DNS_FALLBACKS="1.1.1.1,8.8.8.8"
  BINDNET_UPLINK_INTERFACE="bn-uplink"
  UPLINK_MONITOR_INTERVAL="10"

  local rows
  rows="$(psql_hotspot -At -F $'\t' -c "
    SELECT key, value
    FROM hotspot_config
    WHERE key IN (
      'WIFI_INTERFACE',
      'INTERNET_INTERFACE',
      'WIFI_SSID',
      'WIFI_PASSWORD',
      'WIFI_OPEN',
      'WIFI_COUNTRY',
      'WIFI_CHANNEL',
      'WIFI_FREQ_BAND',
      'WIFI_CHANNEL_CANDIDATES',
      'HOTSPOT_GATEWAY',
      'HOTSPOT_CIDR',
      'HOTSPOT_DNS_FALLBACKS',
      'BINDNET_UPLINK_INTERFACE',
      'UPLINK_MONITOR_INTERVAL'
    )
  " 2>/tmp/bindnet-hotspot-db-error.log)" || {
    log "ERRO: falha ao ler hotspot_config: $(cat /tmp/bindnet-hotspot-db-error.log 2>/dev/null || true)"
    return 1
  }

  local key
  local value
  while IFS=$'\t' read -r key value; do
    [[ -n "${key}" ]] || continue
    case "${key}" in
      WIFI_INTERFACE|INTERNET_INTERFACE|WIFI_SSID|WIFI_PASSWORD|WIFI_OPEN|WIFI_COUNTRY|WIFI_CHANNEL|WIFI_FREQ_BAND|WIFI_CHANNEL_CANDIDATES|HOTSPOT_GATEWAY|HOTSPOT_CIDR|HOTSPOT_DNS_FALLBACKS|BINDNET_UPLINK_INTERFACE|UPLINK_MONITOR_INTERVAL)
        printf -v "${key}" '%s' "${value}"
        export "${key}"
        ;;
    esac
  done <<< "${rows}"

  log "Configuracao operacional do hotspot carregada diretamente do banco de dados."
}

load_runtime_config_if_available() {
  if ! wait_for_hotspot_config_table >/dev/null 2>&1; then
    return 1
  fi
  load_runtime_config_from_db >/dev/null 2>&1
}

runtime_pid() {
  [[ -f "${RUNNER_PID_FILE}" ]] || return 1
  local pid
  pid="$(cat "${RUNNER_PID_FILE}" 2>/dev/null || true)"
  [[ "${pid}" =~ ^[0-9]+$ ]] || return 1
  if kill -0 "${pid}" >/dev/null 2>&1; then
    printf '%s\n' "${pid}"
    return 0
  fi
  rm -f "${RUNNER_PID_FILE}" || true
  return 1
}

# force_stop_create_ap encerra de vez a instancia do create_ap numa
# interface, com timeout e escalonamento pra SIGKILL - usada tanto por
# stop_running_create_ap_instances (comando "stop", chamado pelo
# painel via POST /api/hotspot/stop e /api/hotspot/recover-wifi)
# quanto por cleanup() (saida normal do "run", mais abaixo). "create_ap
# --stop" sozinho pode nunca terminar: se o hostapd estiver preso
# (netlink/driver ja quebrado - mesma trava documentada em
# watchdog.sh), ele ignora o sinal de parada limpa e "create_ap --stop"
# fica esperando pra sempre, travando tambem quem o chamou (inclusive
# a propria requisicao HTTP do painel). Sem forcar a saida aqui, o
# hostapd/dnsmasq ficam orfaos segurando a interface fisica mesmo
# depois do "stop"/"recover-wifi" reportarem sucesso - devolver a
# placa pro NetworkManager em seguida (wifi-manage) so faria o
# NetworkManager brigar com um hostapd zumbi ainda vivo pela mesma
# placa. "pid" e opcional (pode nao ser conhecido pelo chamador); o
# pkill final varre por qualquer hostapd/dnsmasq que ainda referencie o
# diretorio de config dessa interface, cobrindo tambem processos ja
# orfaos/reparentados.
force_stop_create_ap() {
  local iface="$1"
  local pid="${2:-}"
  timeout -k 5 10 create_ap --stop "${iface}" >/dev/null 2>&1 || true
  if [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
    kill -KILL "${pid}" >/dev/null 2>&1 || true
  fi
  pkill -KILL -f "create_ap[.]${iface}[.]conf" >/dev/null 2>&1 || true
}

stop_running_create_ap_instances() {
  local line
  local pid
  local iface
  if [[ -n "${WIFI_INTERFACE:-}" ]]; then
    force_stop_create_ap "${WIFI_INTERFACE}"
  fi
  while read -r line; do
    pid="$(awk '{print $1}' <<< "${line}")"
    iface="$(awk '{print $2}' <<< "${line}")"
    [[ -n "${iface}" ]] || continue
    force_stop_create_ap "${iface}" "${pid}"
  done < <(create_ap --list-running 2>/dev/null || true)
}

stop_service() {
  mkdir -p "${CONTROL_DIR}"
  load_runtime_config_if_available || true

  local pid
  if pid="$(runtime_pid)"; then
    log "Desligando servico do hotspot sem parar o container."
    kill -TERM "${pid}" >/dev/null 2>&1 || true
    for _ in $(seq 1 30); do
      kill -0 "${pid}" >/dev/null 2>&1 || break
      sleep 1
    done
    if kill -0 "${pid}" >/dev/null 2>&1; then
      log "AVISO: servico do hotspot nao encerrou a tempo; forçando parada."
      kill -KILL "${pid}" >/dev/null 2>&1 || true
    fi
  fi

  stop_running_create_ap_instances
  rm -f "${RUNNER_PID_FILE}" || true
}

start_service() {
  mkdir -p "${CONTROL_DIR}"
  touch "${SERVICE_LOG}"
  chmod 0666 "${SERVICE_LOG}" || true

  if runtime_pid >/dev/null; then
    log "Servico do hotspot ja esta em execucao."
    return 0
  fi

  if ! load_runtime_config_from_db; then
    log "ERRO: configure o hotspot pelo painel antes de iniciar o servico."
    return 1
  fi

  log "Iniciando servico do hotspot dentro do container ja existente."
  nohup "$0" run >> "${SERVICE_LOG}" 2>&1 < /dev/null &
  echo "$!" > "${RUNNER_PID_FILE}"
}

status_service() {
  local running=false
  local running_instances=""
  local status="stopped"
  running_instances="$(create_ap --list-running 2>/dev/null || true)"
  if runtime_pid >/dev/null && grep -Eq '^[0-9]+[[:space:]]+' <<< "${running_instances}"; then
    running=true
    status="running"
  elif runtime_pid >/dev/null; then
    running=true
    status="starting"
  fi
  printf '{"running":%s,"status":"%s","startedAt":""}\n' "${running}" "${status}"
}

manager() {
  mkdir -p "${CONTROL_DIR}"
  touch "${SERVICE_LOG}"
  chmod 0666 "${SERVICE_LOG}" || true
  log "Container do hotspot pronto; aguardando configuracao/start pelo painel administrativo."
  tail -n 0 -F "${SERVICE_LOG}" &
  local tail_pid=$!
  trap 'kill "${tail_pid}" >/dev/null 2>&1 || true; stop_service >/dev/null 2>&1 || true' EXIT INT TERM
  while true; do
    sleep 3600 &
    wait $! || true
  done
}

case "${COMMAND}" in
  manager)
    manager
    exit 0
    ;;
  start)
    start_service
    exit $?
    ;;
  restart)
    stop_service
    start_service
    exit $?
    ;;
  stop)
    stop_service
    exit 0
    ;;
  status)
    status_service
    exit 0
    ;;
  run)
    load_runtime_config_from_db || exit 1
    ;;
  *)
    log "ERRO: comando desconhecido: ${COMMAND}"
    exit 2
    ;;
esac

required() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    log "ERRO: variavel obrigatoria ausente: ${name} - configure isso pelo painel (Hotspot -> \"Alterar configuracao\") antes de iniciar o hotspot."
    exit 1
  fi
}

required WIFI_INTERFACE
required INTERNET_INTERFACE
required WIFI_SSID

# WIFI_OPEN=true cria um hotspot livre (sem autenticacao): create_ap so
# gera as diretivas wpa_* do hostapd quando recebe uma passphrase (ver
# try_create_ap mais abaixo) - nesse modo WIFI_PASSWORD nunca e
# obrigatoria nem repassada ao create_ap, mesmo que ja exista um valor
# salvo de uma configuracao anterior com senha.
WIFI_OPEN="${WIFI_OPEN:-false}"
if [[ "${WIFI_OPEN}" != "true" ]]; then
  required WIFI_PASSWORD
fi

HOTSPOT_GATEWAY="${HOTSPOT_GATEWAY:-192.168.12.1}"
HOTSPOT_CIDR="${HOTSPOT_CIDR:-${HOTSPOT_GATEWAY%.*}.0/24}"
WIFI_COUNTRY="${WIFI_COUNTRY:-ST}"
if [[ -z "${WIFI_CHANNEL:-}" && -n "${WIFI_CHANNE:-}" ]]; then
  WIFI_CHANNEL="${WIFI_CHANNE}"
  log "AVISO: usando WIFI_CHANNE como fallback; prefira WIFI_CHANNEL."
fi
WIFI_CHANNEL="${WIFI_CHANNEL:-auto}"
WIFI_FREQ_BAND="${WIFI_FREQ_BAND:-auto}"

# channel.sh: selecao de banda/canal Wi-Fi. interfaces.sh: resolucao/
# avisos sobre a interface de internet. regulatory.sh: diagnostico do
# dominio regulatorio Wi-Fi. watchdog.sh: deteccao em tempo real de
# falha de beacon do create_ap. history.sh: historico de sucesso/falha
# por banda/canal, usado por channel.sh pra priorizar candidatos.
# Todos sourced do mesmo diretorio deste script (ver Dockerfile - os
# seis arquivos vao para /usr/local/bin/). history.sh precisa vir antes
# de channel.sh, que chama suas funcoes.
source "$(dirname "$0")/history.sh"
source "$(dirname "$0")/channel.sh"
source "$(dirname "$0")/interfaces.sh"
source "$(dirname "$0")/regulatory.sh"
source "$(dirname "$0")/watchdog.sh"

# ensure_wifi_radio_unblocked primeiro: um radio bloqueado por rfkill
# faz "iw reg get"/"iw phy info" logo abaixo se comportarem de forma
# estranha tambem, alem de rejeitar todos os canais igualmente. So
# informativo dai em diante - log_wifi_regulatory_info roda uma vez no
# inicio pra deixar visivel no log qualquer trava regulatoria de
# firmware (phy self-managed) antes de qualquer tentativa de canal, ja
# que isso e indistinguivel do lado de fora de "adapter can not
# transmit" em canal especifico.
ensure_wifi_radio_unblocked
log_wifi_regulatory_info

normalize_search_domains() {
  local raw="${DNS_SEARCH_DOMAINS:-${DNS_LOCAL_TLDS:-local,test,example}}"
  local domain
  local -a domains=()

  raw="${raw//;/,}"
  raw="${raw// /,}"
  IFS=',' read -r -a domains <<< "${raw}"

  for i in "${!domains[@]}"; do
    domain="${domains[$i],,}"
    domain="${domain#.}"
    [[ -n "${domain}" ]] || continue
    if ! [[ "${domain}" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]]; then
      log "ERRO: DNS_SEARCH_DOMAINS/DNS_LOCAL_TLDS contem dominio invalido para DHCP: ${domain}"
      exit 1
    fi
    printf '%s\n' "${domain}"
  done | awk '!seen[$0]++' | paste -sd, -
}

normalize_dns_fallbacks() {
  local raw
  local server
  local -a servers=()

  if [[ -v HOTSPOT_DNS_FALLBACKS ]]; then
    raw="${HOTSPOT_DNS_FALLBACKS}"
  else
    raw="1.1.1.1,8.8.8.8"
  fi
  raw="${raw//;/,}"
  raw="${raw// /,}"
  IFS=',' read -r -a servers <<< "${raw}"

  for i in "${!servers[@]}"; do
    server="${servers[$i]}"
    [[ -n "${server}" ]] || continue
    if ! [[ "${server}" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]]; then
      log "ERRO: HOTSPOT_DNS_FALLBACKS contem DNS IPv4 invalido para DHCP: ${server}"
      exit 1
    fi
    printf '%s\n' "${server}"
  done | awk '!seen[$0]++' | paste -sd, -
}

DHCP_SEARCH_DOMAINS="$(normalize_search_domains)"
DHCP_DOMAIN="${DHCP_SEARCH_DOMAINS%%,*}"
DHCP_DNS_FALLBACKS="$(normalize_dns_fallbacks)"
DHCP_DNS_SERVERS="${HOTSPOT_GATEWAY}"
if [[ -n "${DHCP_DNS_FALLBACKS}" ]]; then
  DHCP_DNS_SERVERS="${DHCP_DNS_SERVERS},${DHCP_DNS_FALLBACKS}"
fi
export DHCP_SEARCH_DOMAINS DHCP_DOMAIN DHCP_DNS_SERVERS

# Resolve valores "auto" antes de qualquer tentativa de create_ap:
# banda/canal (resolve_wifi_band, chamada mais abaixo) e a interface de
# internet (resolve_internet_interface). A internet real fica por tras
# de um uplink virtual estavel para que o hotspot nao precise reiniciar
# quando a fonte muda.
resolve_internet_interface
warn_if_concurrent_ap_sta_risky

# try_create_ap tenta subir o hotspot num canal/banda especificos.
# Devolve 0 em sucesso (create_ap encerrado com exit code 0 - parada
# limpa via sinal, ver cleanup/trap mais abaixo), 2 para uma falha que
# trocar de canal/banda nunca resolve (conflito AP+estacao ou EBUSY ao
# configurar uma interface virtual), ou 1 para qualquer outra falha
# (ex.: "adapter can not transmit" nesse canal especifico).
CREATE_AP_LOG="/tmp/bindnet-hotspot-create_ap.log"
DNSMASQ_DHCP_LOG="/tmp/bindnet-dnsmasq-dhcp.log"

prepare_dnsmasq_dhcp_log() {
  # dnsmasq pode abrir o log depois de baixar privilegios. Se uma
  # execucao anterior deixou o arquivo root:root/0644 em /tmp, ele falha
  # com "Permission denied" antes mesmo de servir DHCP.
  rm -f "${DNSMASQ_DHCP_LOG}" || true
  touch "${DNSMASQ_DHCP_LOG}"
  chmod 0666 "${DNSMASQ_DHCP_LOG}"
}

prepare_dnsmasq_dhcp_log

try_create_ap() {
  local band="$1"
  local channel="$2"
  local status=0
  local country="${WIFI_COUNTRY}"
  local -a virtual_interface_args=()

  # Algumas falhas de create_ap executam limpeza propria antes de
  # devolver erro. Como tentamos varios canais/bandas no mesmo processo,
  # garante novamente o uplink virtual antes de cada tentativa para que
  # o fallback seguinte nao quebre com "bn-uplink is not an interface".
  setup_bindnet_virtual_uplink

  # Uma interface AP virtual so e util quando ha uma associacao Wi-Fi
  # cliente que precisa ser preservada. Em alguns kernels/drivers (observado
  # com iwlwifi/AX211 no kernel 7.0), o driver aceita criar ap0 mas devolve
  # EBUSY quando o create_ap tenta trocar o MAC duplicado dessa interface.
  # Com a placa desconectada, usar diretamente a interface fisica evita essa
  # operacao sem perder funcionalidade; o uplink continua sendo bn-uplink.
  if sta_link_connected; then
    log "Wi-Fi cliente ativo em ${WIFI_INTERFACE}; preservando-o com uma interface AP virtual."
    # Um unico radio so transmite numa frequencia por vez: se
    # WIFI_INTERFACE (AP) e INTERNET_INTERFACE (STA) sao a mesma placa,
    # o AP fisicamente so pode subir no mesmo canal/banda que a
    # associacao Wi-Fi cliente ja esta usando agora - sobrescreve
    # band/channel desta tentativa (que podem vir de WIFI_CHANNEL fixo,
    # de rank_channels_for_band ou do ultimo canal bom) por esse
    # motivo, e nao por preferencia do usuario.
    local sta_band_channel
    if sta_band_channel="$(sta_current_band_channel)"; then
      local sta_band="${sta_band_channel% *}"
      local sta_channel="${sta_band_channel#* }"
      if [[ "${sta_band}" != "${band}" || "${sta_channel}" != "${channel}" ]]; then
        log "AVISO: ${WIFI_INTERFACE} esta associado em ${sta_band}GHz canal ${sta_channel} agora; um unico radio nao pode manter o AP em ${band}GHz canal ${channel} ao mesmo tempo - travando o hotspot no canal da estacao."
        band="${sta_band}"
        channel="${sta_channel}"
      fi
    else
      # Nunca sobe o AP com banda/canal nao confirmados contra a
      # estacao: um radio unico so transmite numa frequencia por vez,
      # entao banda/canal errados aqui (ex.: vindos de um candidato de
      # rank_channels_for_band) produzem um hostapd inconsistente com
      # a associacao real - confirmado ao vivo (create_ap forca o
      # canal certo nos bastidores, mas a banda/capacidades do hostapd
      # ficam erradas) resultando em clientes reais que nunca
      # completam a autenticacao ("did not acknowledge authentication
      # response"), mesmo com o AP aparecendo "ENABLED" no log.
      log "ERRO: nao foi possivel confirmar o canal atual da associacao Wi-Fi cliente de ${WIFI_INTERFACE} mesmo apos aguardar - subir o AP em ${band}GHz/canal ${channel} sem confirmar contra a estacao arrisca um hostapd com banda/canal incompativeis (clientes reais nao completam a autenticacao). Aguardando e tentando de novo."
      return 1
    fi
    # O canal acima ja e, por definicao, um canal que o firmware aceitou
    # transmitir para a associacao STA em curso - inclusive quando o phy
    # e self-managed e ignora WIFI_COUNTRY (ver regulatory.sh). Repassar
    # WIFI_COUNTRY pro hostapd nesse caso so arrisca a tabela de
    # canais/potencia propria do hostapd (independente do firmware)
    # rejeitar um canal que na pratica ja funciona - usa o pais que o
    # firmware self-managed realmente aplica, quando existir.
    local self_managed_country
    self_managed_country="$(self_managed_regulatory_country)"
    if [[ -n "${self_managed_country}" && "${self_managed_country}" != "${country}" ]]; then
      log "AVISO: usando pais regulatorio '${self_managed_country}' (imposto pelo firmware self-managed) no hostapd em vez de WIFI_COUNTRY='${WIFI_COUNTRY}', para nao rejeitar o canal ${channel} que a associacao Wi-Fi cliente ja usa de fato."
      country="${self_managed_country}"
    fi
  else
    # Wi-Fi para Wi-Fi (WIFI_INTERFACE == INTERNET_INTERFACE) sem
    # associacao agora: --no-virt sobe o AP usando a placa inteira,
    # sem nenhuma conexao cliente sobrando - o hotspot ficaria
    # "rodando" pro painel, mas sem internet nenhuma pra compartilhar,
    # ja que a mesma placa nao pode ser AP puro e estacao ao mesmo
    # tempo. Falha aqui (retentavel, ver loop de retry no fim do
    # script) em vez de aceitar um AP sem internet como sucesso.
    if [[ "${WIFI_INTERFACE}" == "${REAL_INTERNET_INTERFACE}" ]]; then
      log "ERRO: ${WIFI_INTERFACE} e INTERNET_INTERFACE sao a mesma placa (Wi-Fi para Wi-Fi), mas ela nao esta associada como cliente Wi-Fi agora - subir em --no-virt deixaria o hotspot 'rodando' sem internet nenhuma pra compartilhar. Aguardando a placa reconectar como cliente antes de tentar de novo."
      return 1
    fi
    virtual_interface_args=(--no-virt)
    log "${WIFI_INTERFACE} sem associacao Wi-Fi cliente; usando a interface fisica diretamente em modo AP (--no-virt)."
  fi

  local band_display="${band}GHz"
  [[ "${ORIGINAL_WIFI_FREQ_BAND}" == "auto" ]] && band_display="auto (${band}GHz)"
  local channel_display="${channel}"
  [[ "${WIFI_CHANNEL}" == "auto" ]] && channel_display="auto (${channel})"

  log "Preparando hotspot '${WIFI_SSID}' em ${WIFI_INTERFACE}, internet via ${CREATE_AP_INTERNET_INTERFACE} (alimentado por ${REAL_INTERNET_INTERFACE})."
  log "Regiao Wi-Fi: ${country}; banda: ${band_display}; canal: ${channel_display}."
  log "Gateway do hotspot: ${HOTSPOT_GATEWAY}; DNS entregues por DHCP: ${DHCP_DNS_SERVERS}."
  log "Dominios de busca entregues por DHCP: ${DHCP_SEARCH_DOMAINS}."

  # ap_credential_args monta o SSID (+ passphrase, quando nao livre) como
  # os ultimos argumentos posicionais do create_ap. Passphrase OMITIDA
  # (nao vazia - ausente mesmo) e a forma suportada pelo proprio
  # create_ap de pedir rede aberta: ele so escreve as diretivas wpa_* no
  # hostapd.conf quando recebe uma passphrase (`if [[ -n "$PASSPHRASE"
  # ]]`, oblique/create_ap) - sem isso, hostapd sobe sem autenticacao
  # nenhuma. WIFI_OPEN=true nunca reusa uma WIFI_PASSWORD ja salva de uma
  # configuracao anterior com senha.
  local -a ap_credential_args=("${WIFI_SSID}")
  local security_display="WPA2 (com senha)"
  if [[ "${WIFI_OPEN}" == "true" ]]; then
    security_display="livre (sem senha, rede aberta)"
  else
    ap_credential_args+=("${WIFI_PASSWORD}")
  fi
  log "Seguranca: ${security_display}."

  # Roda em segundo plano e usa "wait $PID" (em vez de um pipe em
  # primeiro plano) de proposito: o bash so garante que um trap (ver
  # "trap cleanup EXIT" / "trap ... INT TERM" mais abaixo) interrompe e
  # roda durante um "wait" explicito - bloqueado dentro de um pipe em
  # primeiro plano, o SIGTERM do "docker compose stop"/painel fica pendente ate
  # o create_ap terminar sozinho (nunca termina, ele serve o hotspot
  # indefinidamente), e o Docker acaba forcando SIGKILL apos o prazo,
  # sem o cleanup rodar - deixando a interface virtual orfa (ver
  # remove_stale_virtual_interfaces acima).
  create_ap \
    "${virtual_interface_args[@]}" \
    --no-dns \
    --dhcp-dns "${DHCP_DNS_SERVERS}" \
    --country "${country}" \
    --freq-band "${band}" \
    -c "${channel}" \
    -g "${HOTSPOT_GATEWAY}" \
    "${WIFI_INTERFACE}" \
    "${CREATE_AP_INTERNET_INTERFACE}" \
    "${ap_credential_args[@]}" > >(tee "${CREATE_AP_LOG}") 2>&1 &
  CREATE_AP_PID=$!
  start_beacon_failure_watcher "${CREATE_AP_LOG}" "${CREATE_AP_PID}" "${band}" "${channel}"
  wait "${CREATE_AP_PID}" || status=$?
  stop_beacon_failure_watcher
  CREATE_AP_PID=

  # Registra falha no historico persistente (history.sh) se o AP nunca
  # chegou a subir nesse banda/canal - "AP-ENABLED" ausente do log e o
  # sinal confiavel de que o adaptador rejeitou mesmo, independente do
  # "status" (que so reflete o resultado final de "wait", incluindo
  # quedas por outro motivo bem depois de ter subido). O SUCESSO ja foi
  # registrado ao vivo por start_beacon_failure_watcher (watchdog.sh)
  # no instante em que "AP-ENABLED" apareceu - nao registrar de novo
  # aqui: "wait" so retorna quando o create_ap eventualmente sai
  # (parada pedida, possivelmente dias depois, ou uma queda por outro
  # motivo), nunca no momento do sucesso de verdade, entao nunca
  # sobrescreve/duplica o que ja foi contado.
  if ! grep -q 'AP-ENABLED' "${CREATE_AP_LOG}" 2>/dev/null; then
    record_channel_result "${band}" "${channel}" 0
  fi

  # LAST_GOOD_BAND/LAST_GOOD_CHANNEL (globais, declaradas mais abaixo)
  # guardam o ultimo canal que realmente conseguiu subir o AP nesta
  # execucao - o loop de retry no fim do script tenta esse canal
  # direto antes de repetir a varredura completa.
  if [[ "${status}" -eq 0 ]]; then
    LAST_GOOD_BAND="${band}"
    LAST_GOOD_CHANNEL="${channel}"
  fi

  if [[ "${status}" -ne 0 ]] && grep -qi 'can not be a station.*and an AP at the same time' "${CREATE_AP_LOG}"; then
    log "ERRO: ${WIFI_INTERFACE} ainda esta associado como estacao (cliente Wi-Fi) - o driver desta placa nao suporta AP+estacao simultaneos, e trocar de canal/banda nao muda isso. Use outra placa para o hotspot ou outra interface Wi-Fi compatível com AP+STA."
    return 2
  fi

  if [[ "${status}" -ne 0 ]] && grep -qi 'RTNETLINK answers: Resource busy' "${CREATE_AP_LOG}"; then
    log "ERRO: o driver deixou criar a interface AP virtual, mas recusou configura-la (RTNETLINK: Resource busy). Desconecte ${WIFI_INTERFACE} da rede Wi-Fi cliente para o Bindnet usar automaticamente --no-virt, ou atualize/reverta o kernel/firmware do adaptador. Trocar de canal nao resolve esta causa."
    return 2
  fi

  return "${status}"
}

# start_hotspot_auto tenta, em ordem de menor interferencia, cada canal
# candidato da banda informada. Devolve 0 assim que um canal funcionar,
# 2 se try_create_ap detectar uma falha independente do canal (aborta a
# varredura imediatamente - continuar testando canais so repetiria o
# mesmo erro) ou 1 se todos os candidatos da banda forem rejeitados por
# outro motivo.
start_hotspot_auto() {
  local band="$1"
  local channel
  local status

  rank_channels_for_band "${band}"
  if [[ "${#RANKED_CHANNELS[@]}" -eq 0 ]]; then
    log "AVISO: nenhum canal candidato disponivel para ${band}GHz."
    return 1
  fi

  for channel in "${RANKED_CHANNELS[@]}"; do
    status=0
    try_create_ap "${band}" "${channel}" || status=$?
    if [[ "${status}" -eq 0 ]]; then
      return 0
    fi
    if [[ "${status}" -eq 2 ]]; then
      return 2
    fi
    log "AVISO: canal ${channel} (${band}GHz) rejeitado pelo adaptador; tentando proximo candidato."
  done
  return 1
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

CREATE_AP_PID=
UPLINK_MONITOR_PID=""
STOPPING=0

# LAST_GOOD_BAND/LAST_GOOD_CHANNEL: ver comentario em try_create_ap.
LAST_GOOD_BAND=""
LAST_GOOD_CHANNEL=""

cleanup() {
  log "Encerrando hotspot em ${WIFI_INTERFACE}."
  stop_beacon_failure_watcher
  # force_stop_create_ap (definida no topo do script, perto de
  # stop_running_create_ap_instances) ja cobre o timeout de
  # "create_ap --stop" e o escalonamento pra SIGKILL - reusa aqui em
  # vez de duplicar essa logica. Garante que hostapd/dnsmasq realmente
  # morrem antes de devolver a placa pro NetworkManager (ver
  # recoverWifiAdapter em services/backend/hotspot_network.go): sem
  # isso, uma instancia presa (netlink/driver ja quebrado, ver
  # watchdog.sh) ficaria orfa segurando a interface fisica mesmo depois
  # do cleanup reportar concluido.
  force_stop_create_ap "${WIFI_INTERFACE}" "${CREATE_AP_PID}"
  cleanup_bindnet_uplink
}
trap cleanup EXIT
# STOPPING=1 aqui (alem de cleanup) e o que diferencia, no loop de
# retry mais abaixo, uma parada pedida de verdade (stop_service manda
# SIGTERM para o PID deste script) de uma queda que deve ser
# retentada no lugar (watchdog.sh mata so o CREATE_AP_PID diretamente,
# nunca este processo - STOPPING continua 0 nesse caso).
trap 'STOPPING=1; cleanup' INT TERM

# Guarda a escolha original do usuario antes de resolve_wifi_band
# sobrescrever WIFI_FREQ_BAND - so tenta a banda alternativa (fallback)
# quando o proprio usuario deixou banda E canal em "auto" (modo
# totalmente automatico); um canal ou banda fixados explicitamente sao
# respeitados como estao, sem fallback.
ORIGINAL_WIFI_FREQ_BAND="${WIFI_FREQ_BAND}"
resolve_wifi_band

# remove_stale_virtual_interfaces apaga interfaces "apN" orfas de uma
# execucao anterior deste mesmo script. O create_ap sempre nomeia as
# interfaces virtuais que cria como "ap" + numero incremental
# (alloc_new_iface no create_ap) - nunca outro nome - entao o filtro
# "^ap[0-9]+$" so pega interfaces que o proprio create_ap criou, nunca
# WIFI_INTERFACE nem nada gerenciado pelo usuario/NetworkManager. Isso
# e necessario porque "network_mode: host" faz a interface virtual
# existir no host de verdade, nao só dentro do container - se o
# container anterior morrer por SIGKILL (ex.: timeout do "docker stop"),
# o trap acima nunca roda e a interface fica presa, fazendo a proxima
# tentativa falhar com "RTNETLINK answers: Resource busy" (a combinacao
# suportada pela placa so permite uma interface AP por vez).
remove_stale_virtual_interfaces() {
  local iface
  for iface in $(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | grep -E '^ap[0-9]+$' || true); do
    log "AVISO: interface virtual '${iface}' orfa de uma execucao anterior encontrada; removendo antes de tentar de novo."
    iw dev "${iface}" del >/dev/null 2>&1 || true
  done
}
remove_stale_virtual_interfaces

setup_bindnet_virtual_uplink
start_uplink_monitor

# attempt_hotspot_cycle faz uma rodada completa de tentativa de subir
# o hotspot: primeiro tenta direto o ultimo canal/banda que funcionou
# nesta mesma execucao (LAST_GOOD_BAND/LAST_GOOD_CHANNEL - evita
# repetir toda a varredura, incluindo bandas/canais ja sabidamente
# ruins nesta placa, ex.: trava regulatoria de firmware que rejeita
# 5GHz inteiro); se isso falhar ou for a primeira tentativa, cai para
# o fluxo normal (canal fixo do usuario, ou varredura automatica com
# fallback de banda, tambem comecando pela banda ja conhecida como boa
# quando existir). Mesmo contrato de retorno de
# try_create_ap/start_hotspot_auto (0 sucesso/parada limpa, 1 falha
# retentavel, 2 falha definitiva).
attempt_hotspot_cycle() {
  local status=0

  if [[ -n "${LAST_GOOD_BAND}" && -n "${LAST_GOOD_CHANNEL}" ]]; then
    log "Tentando primeiro o canal ${LAST_GOOD_CHANNEL} (${LAST_GOOD_BAND}GHz), que funcionou da ultima vez nesta execucao."
    try_create_ap "${LAST_GOOD_BAND}" "${LAST_GOOD_CHANNEL}" || status=$?
    if [[ "${status}" -eq 0 || "${status}" -eq 2 ]]; then
      return "${status}"
    fi
    log "AVISO: canal ${LAST_GOOD_CHANNEL} (${LAST_GOOD_BAND}GHz) nao funcionou desta vez; voltando a varredura completa."
  fi

  if [[ "${WIFI_CHANNEL}" != "auto" ]]; then
    if ! [[ "${WIFI_CHANNEL}" =~ ^[0-9]+$ ]]; then
      log "ERRO: WIFI_CHANNEL deve ser numerico ou auto."
      return 1
    fi
    status=0
    try_create_ap "${WIFI_FREQ_BAND}" "${WIFI_CHANNEL}" || status=$?
    return "${status}"
  fi

  # Wi-Fi para Wi-Fi (WIFI_INTERFACE == INTERNET_INTERFACE) com
  # WIFI_CHANNEL=auto: se a placa ja esta associada como cliente agora,
  # trava direto no canal/banda dessa associacao e pula a varredura de
  # candidatos inteira. try_create_ap ja sobrescreve banda/canal pra
  # bater com a estacao de qualquer forma (ver comentario ali) - rankear/
  # escanear canais (rank_channels_for_band, via start_hotspot_auto)
  # so arrisca derrubar essa mesma conexao Wi-Fi cliente (que alimenta o
  # hotspot) com "iw dev ... scan" antes da primeira tentativa real, sem
  # nenhum ganho, ja que o resultado do ranking seria descartado mesmo.
  # Wi-Fi para Wi-Fi NUNCA cai pro caminho de varredura ativa abaixo
  # (start_hotspot_auto/rank_channels_for_band): "iw dev ... scan"
  # ativo e bem mais perturbador pra estacao associada do que a
  # varredura passiva de fundo do NetworkManager (confirmado - foi
  # exatamente essa varredura ativa, disparada apos sta_current_band_channel
  # falhar aqui uma vez, que manteve a placa instavel nas tentativas
  # seguintes) - sem ganho nenhum de qualquer forma, ja que o canal do
  # AP e sempre forcado pro canal da estacao neste modo (try_create_ap
  # sobrescreve), nunca escolhido pelo ranking de interferencia. Se
  # sta_current_band_channel falhar mesmo com toda a paciencia de
  # sta_link_connected, retorna falha retentavel e deixa o loop de
  # retry externo (backoff mais longo, sem nenhum scan no meio)
  # esperar a estacao estabilizar sozinha.
  if [[ "${WIFI_INTERFACE}" == "${REAL_INTERNET_INTERFACE}" ]]; then
    local sta_band_channel
    if sta_band_channel="$(sta_current_band_channel)"; then
      local sta_band="${sta_band_channel% *}"
      local sta_channel="${sta_band_channel#* }"
      log "Wi-Fi para Wi-Fi: ${WIFI_INTERFACE} ja associado em ${sta_band}GHz canal ${sta_channel}; travando nesse canal direto, sem varredura, pra nao arriscar derrubar a propria conexao cliente."
      status=0
      try_create_ap "${sta_band}" "${sta_channel}" || status=$?
    else
      log "ERRO: nao foi possivel confirmar o canal atual da associacao Wi-Fi cliente de ${WIFI_INTERFACE} mesmo apos aguardar - Wi-Fi para Wi-Fi nao varre canais nessa placa (a varredura ativa so pioraria a instabilidade da propria conexao cliente). Aguardando reconectar antes de tentar de novo."
      status=1
    fi
    return "${status}"
  fi

  local first_band="${LAST_GOOD_BAND:-${WIFI_FREQ_BAND}}"
  status=0
  start_hotspot_auto "${first_band}" || status=$?
  if [[ "${status}" -eq 0 || "${status}" -eq 2 ]]; then
    return "${status}"
  fi

  if [[ "${ORIGINAL_WIFI_FREQ_BAND}" == "auto" ]]; then
    local fallback_band="2.4"
    [[ "${first_band}" == "2.4" ]] && fallback_band="5"
    log "AVISO: nenhum canal funcionou em ${first_band}GHz; tentando banda alternativa ${fallback_band}GHz."
    status=0
    start_hotspot_auto "${fallback_band}" || status=$?
  fi
  return "${status}"
}

# Loop de retry: qualquer queda do create_ap que nao seja uma parada
# pedida de verdade (STOPPING=1, ver trap acima) e retentada no lugar,
# com backoff crescente, ate HOTSPOT_MAX_RESTART_ATTEMPTS. So depois
# de esgotar essas tentativas locais o script sai de fato (STATUS=1),
# deixando a reconciliacao do backend religar via novo "start" (ver
# recoverHotspotIfDesired em services/backend/hotspot_reconcile.go) -
# rede de seguranca para quando o problema nao e mais o create_ap
# sozinho (ex.: o proprio container/host caiu). status 2 (falha
# definitiva, ver contrato de try_create_ap) sempre sai na hora, sem
# retentar - trocar de canal/banda ou tentar de novo nunca resolve
# esses casos.
HOTSPOT_MAX_RESTART_ATTEMPTS="${HOTSPOT_MAX_RESTART_ATTEMPTS:-5}"
HOTSPOT_RESTART_BACKOFF_SECONDS="${HOTSPOT_RESTART_BACKOFF_SECONDS:-3}"

attempt=0
while true; do
  # Confere STOPPING logo no topo tambem: se um SIGTERM/SIGINT chegou
  # durante o "sleep" do backoff abaixo, ele interrompe o sleep e volta
  # pro topo do loop - sem essa checagem aqui, uma parada pedida bem
  # nessa janela ainda dispararia mais uma tentativa completa de subir
  # o hotspot antes de sair.
  if [[ "${STOPPING}" -eq 1 ]]; then
    exit 0
  fi

  attempt=$((attempt + 1))
  STATUS=0
  attempt_hotspot_cycle || STATUS=$?

  if [[ "${STATUS}" -eq 2 ]]; then
    exit 1
  fi

  if [[ "${STOPPING}" -eq 1 ]]; then
    exit "${STATUS}"
  fi

  if [[ "${attempt}" -ge "${HOTSPOT_MAX_RESTART_ATTEMPTS}" ]]; then
    log "ERRO: hotspot caiu/falhou ${attempt} vezes seguidas nesta execucao; desistindo e deixando a reconciliacao do backend religar (em ate 15s)."
    exit 1
  fi

  backoff=$((HOTSPOT_RESTART_BACKOFF_SECONDS * attempt))
  log "Hotspot caiu (tentativa ${attempt}/${HOTSPOT_MAX_RESTART_ATTEMPTS}); tentando de novo em ${backoff}s."
  sleep "${backoff}"
done
