#!/usr/bin/env bash
# history.sh - historico de sucesso/falha por banda/canal do hotspot,
# persistido em Postgres (tabela hotspot_channel_history) pra priorizar,
# na proxima subida, os candidatos que ja funcionaram antes em vez de
# sempre reavaliar do zero (varredura de interferencia + tentativa e
# erro em todos os candidatos, incluindo bandas/canais que ja se
# provaram sempre ruins nesta placa, ex.: trava regulatoria de firmware
# que rejeita 5GHz inteiro). Sourced pelo entrypoint.sh; usa
# "psql_hotspot" (definida ali) e as variaveis WIFI_INTERFACE/
# REAL_INTERNET_INTERFACE ja resolvidas. Nunca falha o hotspot por si
# so - historico e so uma otimizacao de velocidade, nunca uma
# dependencia funcional (toda consulta/escrita cai em silencio se o
# banco estiver indisponivel).

# channel_history_mode imprime 'same-interface' quando WIFI_INTERFACE e
# INTERNET_INTERFACE sao a mesma placa (Wi-Fi para Wi-Fi, canal travado
# pela estacao - sem escolha real de canal, historico so serve pra
# diagnostico ali) ou 'different-interface' caso contrario (canal
# escolhido livremente, ex. Ethernet para Wi-Fi - onde o historico
# realmente influencia a ordem de tentativas).
channel_history_mode() {
  if [[ "${WIFI_INTERFACE}" == "${REAL_INTERNET_INTERFACE}" ]]; then
    printf 'same-interface\n'
  else
    printf 'different-interface\n'
  fi
}

# record_channel_result registra o resultado de uma tentativa de subir
# o AP num banda/canal especifico - chamada por try_create_ap
# (entrypoint.sh) apos cada tentativa, com base no log do create_ap ter
# mostrado "AP-ENABLED" (sucesso de verdade, mesmo que o AP caia depois
# por outro motivo, ex. beacon) ou nao (rejeitado pelo adaptador/driver
# antes de sequer subir).
record_channel_result() {
  local band="$1"
  local channel="$2"
  local success="$3"
  local mode
  mode="$(channel_history_mode)"

  local success_delta=0
  local failure_delta=0
  local result='failure'
  if [[ "${success}" == "1" ]]; then
    success_delta=1
    result='success'
  else
    failure_delta=1
  fi

  psql_hotspot -Atqc "
    INSERT INTO hotspot_channel_history (wifi_interface, mode, band, channel, success_count, failure_count, last_result, last_attempt_at, updated_at)
    VALUES ('${WIFI_INTERFACE}', '${mode}', '${band}', ${channel}, ${success_delta}, ${failure_delta}, '${result}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
    ON CONFLICT (wifi_interface, mode, band, channel) DO UPDATE
    SET success_count = hotspot_channel_history.success_count + ${success_delta},
        failure_count = hotspot_channel_history.failure_count + ${failure_delta},
        last_result = '${result}',
        last_attempt_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP
  " >/dev/null 2>&1 || true
}

# CHANNEL_HISTORY_SCORE (array associativo global, canal -> pontuacao)
# e preenchido por load_channel_history_scores antes de
# rank_channels_for_band (channel.sh) ordenar os candidatos de uma
# banda. Pontuacao = (falhas - sucessos) * 1000 - multiplicador alto de
# proposito, pra historico sempre pesar mais que a pontuacao de
# interferencia ao vivo; interferencia so decide empate entre canais
# com o mesmo historico (incluindo nenhum historico ainda, pontuacao 0).
declare -gA CHANNEL_HISTORY_SCORE=()
load_channel_history_scores() {
  local band="$1"
  local mode
  mode="$(channel_history_mode)"
  CHANNEL_HISTORY_SCORE=()

  local rows
  rows="$(psql_hotspot -At -F $'\t' -c "
    SELECT channel, (failure_count - success_count) * 1000
    FROM hotspot_channel_history
    WHERE wifi_interface = '${WIFI_INTERFACE}' AND mode = '${mode}' AND band = '${band}'
  " 2>/dev/null || true)"

  local channel score
  while IFS=$'\t' read -r channel score; do
    [[ -n "${channel}" ]] || continue
    CHANNEL_HISTORY_SCORE["${channel}"]="${score}"
  done <<< "${rows}"
  # "return 0" explicito: sem historico ainda (rows vazio, caso comum
  # antes da primeira tentativa), o "while read" acima nunca entra no
  # corpo do loop e devolve 1 (status do proprio "read" no EOF) como
  # saida do compound "while" - essa e a ULTIMA instrucao da funcao, e
  # chamamos essa funcao como comando solto (nao dentro de if/&&/||)
  # em rank_channels_for_band (channel.sh), entao um retorno 1 aqui
  # derrubaria o script inteiro em silencio pelo set -e do topo do
  # script (mesma classe de bug ja confirmada em
  # ensure_wifi_radio_unblocked, regulatory.sh).
  return 0
}

# channel_history_score imprime a pontuacao de historico de um canal
# (0 se nunca tentado) - somada a pontuacao de interferencia ao vivo em
# rank_channels_for_band.
channel_history_score() {
  local channel="$1"
  printf '%s\n' "${CHANNEL_HISTORY_SCORE[${channel}]:-0}"
}

# band_history_score imprime (sucessos - falhas) somado de todos os
# canais ja tentados numa banda, nesta placa e neste modo - usada por
# resolve_wifi_band (channel.sh) pra preferir a banda com historico de
# verdade melhor em vez de sempre comecar pela banda preferida por
# capacidade de hardware (5GHz), mesmo quando essa banda inteira ja
# falhou consistentemente antes.
band_history_score() {
  local band="$1"
  local mode
  mode="$(channel_history_mode)"

  psql_hotspot -Atqc "
    SELECT COALESCE(SUM(success_count) - SUM(failure_count), 0)
    FROM hotspot_channel_history
    WHERE wifi_interface = '${WIFI_INTERFACE}' AND mode = '${mode}' AND band = '${band}'
  " 2>/dev/null || printf '0\n'
}
