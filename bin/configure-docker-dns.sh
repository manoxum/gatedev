#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[bindnet-docker-dns] %s\n' "$*"
}

docker_dns="${DOCKER_HOST_GATEWAY:-10.90.0.1}"
daemon_config="${DOCKER_DAEMON_CONFIG:-/etc/docker/daemon.json}"
backup="${daemon_config}.bak.$(date +%Y%m%d%H%M%S)"
tmp="$(mktemp)"

if [[ "${EUID}" -ne 0 && ( ! -f "${daemon_config}" || ! -w "${daemon_config}" ) ]]; then
  log "ERRO: execute com sudo para alterar ${daemon_config}."
  exit 1
fi

cleanup() {
  rm -f "${tmp}"
}
trap cleanup EXIT

mkdir -p "$(dirname "${daemon_config}")"
if [[ -f "${daemon_config}" ]]; then
  cp "${daemon_config}" "${backup}"
  log "Backup criado em ${backup}."
else
  printf '{}\n' > "${daemon_config}"
  log "Criado ${daemon_config}."
fi

python3 - "${daemon_config}" "${tmp}" "${docker_dns}" <<'PY'
import json
import sys

source, target, docker_dns = sys.argv[1:]
with open(source, "r", encoding="utf-8") as handle:
    raw = handle.read().strip() or "{}"

config = json.loads(raw)
config["dns"] = [docker_dns]

with open(target, "w", encoding="utf-8") as handle:
    json.dump(config, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
PY

python3 -m json.tool "${tmp}" >/dev/null
cp "${tmp}" "${daemon_config}"

log "Docker daemon configurado para usar DNS ${docker_dns}."
log "Reinicie o Docker para aplicar: sudo systemctl restart docker"
