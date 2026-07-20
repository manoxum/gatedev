FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/controller

FROM alpine:3.22
# networkmanager-cli: no Alpine 3.22 o pacote "networkmanager" nao traz
# o "nmcli" (fica num sub-pacote separado) - sem isso,
# /network/wifi-unmanage e /network/wifi-manage (network.go) falhavam
# com "nmcli: not found", devolvendo 500 sem corpo (exec.Command nunca
# roda nada, "output" fica vazio).
# iptables: usado por shaping_iptables.go (chain "BINDNET-SHAPING" na
# tabela mangle) para marcar/contar trafego por dispositivo antes do
# shaping via tc (iproute2, ja instalado acima) - o worker roda esses
# comandos direto no host porque compartilha o mesmo network_mode: host
# do container "hotspot", sem precisar de "docker exec".
RUN apk add --no-cache docker-cli docker-cli-compose networkmanager networkmanager-cli iproute2 iptables iw ca-certificates nss-tools
COPY --from=build /out/worker /usr/local/bin/worker
ENTRYPOINT ["/usr/local/bin/worker"]
