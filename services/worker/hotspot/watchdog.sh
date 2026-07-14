# Sourced por entrypoint.sh - sem shebang proprio, mesmo padrao de
# channel.sh/interfaces.sh/regulatory.sh (ver Dockerfile, que copia os
# quatro arquivos para /usr/local/bin/).

WATCHDOG_PID=""
HOTSPOT_BEACON_FAILURE_WINDOW_SECONDS="${HOTSPOT_BEACON_FAILURE_WINDOW_SECONDS:-20}"
HOTSPOT_BEACON_FAILURE_THRESHOLD="${HOTSPOT_BEACON_FAILURE_THRESHOLD:-2}"

# start_beacon_failure_watcher acompanha o log do create_ap em tempo
# real (tail -F) procurando "Failed to set beacon parameters". Essa
# assinatura tem pelo menos duas causas ja confirmadas: a regressao de
# 802.11be/MLO do hostapd 2.11 (ver Dockerfile - mitigada fixando
# 2.10-r6, mas nao eliminada por completo) e, na pratica mais comum
# neste stack, um rfkill de verdade na placa fisica em uso pelo hotspot
# (--no-virt) enquanto o NetworkManager ainda a considerava sua (ver
# unmanagePhysicalInterfaceIfIdle em
# services/backend/hotspot_network.go) - o
# adaptador recusa novos beacons e fica derrubando/recusando clientes
# indefinidamente, sem o processo create_ap sair sozinho, em ambos os
# casos. threshold>1 evita reagir a uma falha isolada/transitoria; ao
# cruzar o limiar dentro da janela, derruba o create_ap (mesmo sinal
# que cleanup() usa pra parada limpa) - so isso, nao reinicia nada
# sozinho aqui: quem re-tenta e o loop de retry no fim do
# entrypoint.sh (que ve o create_ap cair sem STOPPING=1 e tenta de
# novo, comecando pelo mesmo canal), e a reconciliacao do backend
# entra como rede de seguranca se o script inteiro tambem cair (ver
# recoverHotspotIfDesired em services/backend/hotspot_reconcile.go).
start_beacon_failure_watcher() {
  local log_file="$1"
  local target_pid="$2"

  (
    local count=0
    local window_start
    window_start="$(date +%s)"
    tail -n0 -F "${log_file}" 2>/dev/null | while IFS= read -r line; do
      case "${line}" in
        *"Failed to set beacon parameters"*)
          local now
          now="$(date +%s)"
          if (( now - window_start > HOTSPOT_BEACON_FAILURE_WINDOW_SECONDS )); then
            count=0
            window_start="${now}"
          fi
          count=$((count + 1))
          if (( count >= HOTSPOT_BEACON_FAILURE_THRESHOLD )); then
            log "AVISO: falha recorrente de beacon detectada (${count}x em ${HOTSPOT_BEACON_FAILURE_WINDOW_SECONDS}s; regressao do hostapd 2.11 ou rfkill externo na placa fisica, ver Dockerfile/watchdog.sh); derrubando o create_ap para nova tentativa."
            kill -INT "${target_pid}" >/dev/null 2>&1 || true
            exit 0
          fi
          ;;
      esac
    done
  ) &
  WATCHDOG_PID=$!
}

stop_beacon_failure_watcher() {
  [[ -n "${WATCHDOG_PID}" ]] || return 0
  kill "${WATCHDOG_PID}" >/dev/null 2>&1 || true
  wait "${WATCHDOG_PID}" 2>/dev/null || true
  WATCHDOG_PID=""
  # O "tail -F" roda como ultimo estagio de um pipe, num subshell
  # separado do subshell acima (WATCHDOG_PID) - matar WATCHDOG_PID no
  # meio de um "exit 0" disparado pelo proprio watcher (deteccao de
  # falha) as vezes nao chega a encerrar o "tail" a tempo. pkill -f
  # pelo caminho exato do log garante que nenhuma instancia orfa
  # continue rodando entre tentativas.
  pkill -f "tail -n0 -F ${CREATE_AP_LOG}" >/dev/null 2>&1 || true
}
