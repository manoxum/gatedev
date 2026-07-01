# central

Stack Docker Compose de infraestrutura de rede local: hotspot Wi-Fi
compartilhado, DNS split-horizon para domínios locais, proxy HTTPS com
CA própria (autoassinada) para desenvolvimento sem avisos de
certificado, e uma UI para administrar os sites atrás do proxy.

> Regras de negócio detalhadas por serviço: veja [RULE.md](RULE.md).
> Diretrizes para agentes de IA (Claude/Codex) trabalhando neste repo:
> veja [CLAUDE.md](CLAUDE.md) e [AGENTS.md](AGENTS.md).

## Arquitetura

```
                 (clientes Wi-Fi: celular, notebook, etc.)
                                │
                          rede "xCosta" (WIFI_SSID)
                                │
                    ┌───────────────────────┐
                    │   hotspot (create_ap)  │  privileged, network_mode: host
                    │   WIFI_INTERFACE → AP   │
                    └───────────┬───────────┘
                                │ NAT para internet via INTERNET_INTERFACE
                                │
                    ┌───────────────────────┐
                    │  dns-provider (CoreDNS) │  network_mode: host
                    │  split-horizon TLDs      │
                    │  locais → HOTSPOT_GATEWAY │
                    └───────────┬───────────┘
                                │
                    ┌───────────────────────┐        ┌──────────────┐
                    │  cert-proxy (Go)        │◄──────►│  nginx-ui     │
                    │  :80 / :443              │        │  :9080 admin  │
                    │  CA local + certs sob     │        └──────────────┘
                    │  demanda                   │
                    └───────────────────────┘
```

- **hotspot** — cria o ponto de acesso Wi-Fi (via `create_ap`),
  compartilhando a internet de `INTERNET_INTERFACE`.
- **dns-provider** — CoreDNS: resolve TLDs locais (`DNS_LOCAL_TLDS`)
  para `HOTSPOT_GATEWAY` e encaminha o resto para DNS público.
- **cert-proxy** — reverse proxy Go que termina TLS com uma CA local
  autoassinada, emitindo certificados sob demanda por domínio, e
  encaminha tudo (exceto rotas da própria CA) para o `nginx-ui`.
- **nginx-ui** — interface de administração para configurar os sites
  servidos atrás do `cert-proxy`.

## Pré-requisitos

- Linux com Docker Engine + Docker Compose v2 (`docker compose ...`).
- Para o hotspot: placa Wi-Fi compatível com modo AP, NetworkManager
  instalado, e `sudo` (os scripts `scripts/hotspot-on.sh` /
  `hotspot-off.sh` reconfiguram o NetworkManager).
- Opcional: Go 1.23+ apenas se quiser compilar/rodar `cert-proxy` fora
  do Docker.

## Configuração inicial

1. Copie o template de variáveis de ambiente e ajuste para a sua
   máquina (interfaces de rede, SSID, senha, etc.):

   ```bash
   cp .env.example .env
   $EDITOR .env
   ```

   Veja os comentários em [.env.example](.env.example) para o
   significado, obrigatoriedade e valor padrão de cada variável.
   **`.env` nunca deve ser commitado** (já está no `.gitignore`).

2. Crie os volumes Docker externos usados pelo stack (são `external:
   true` no `docker-compose.yml`, ou seja, não são criados
   automaticamente):

   ```bash
   docker volume create central_nginx_config
   docker volume create central_nginx_ui_data
   docker volume create central_www_data
   docker volume create central_cert_proxy_data
   ```

## Subindo os serviços

### Proxy + UI (sem mexer na rede/Wi-Fi do host)

```bash
docker compose up -d nginx-ui cert-proxy
```

### Hotspot Wi-Fi completo (hotspot + DNS split-horizon)

Use os scripts dedicados em vez de `docker compose up` direto: eles
também preparam o NetworkManager para liberar a placa Wi-Fi para o
`hostapd` (dentro do `create_ap`).

```bash
./scripts/hotspot-on.sh   # sobe hotspot + dns-provider, tira a placa do NetworkManager
./scripts/hotspot-off.sh  # para os serviços e devolve a placa ao NetworkManager
```

⚠️ `hotspot-on.sh` desconecta a placa Wi-Fi (`WIFI_INTERFACE`) do uso
normal como cliente enquanto o hotspot estiver ativo. Use
`hotspot-off.sh` para reverter isso de forma limpa.

### Tudo junto

```bash
docker compose up -d
```

## Variáveis de ambiente

Referência completa e comentada em [.env.example](.env.example).
Resumo das mais relevantes:

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `WIFI_INTERFACE` | sim | — | interface Wi-Fi física que vira o AP |
| `INTERNET_INTERFACE` | sim | — | interface com saída para a internet |
| `WIFI_SSID` | sim | — | nome da rede Wi-Fi criada |
| `WIFI_PASSWORD` | sim | — | senha WPA2 da rede |
| `WIFI_COUNTRY` | não | `ST` | código regulatório de país |
| `WIFI_CHANNEL` | não | `auto` | canal fixo ou seleção automática por varredura |
| `WIFI_CHANNEL_CANDIDATES` | não | por banda | candidatos avaliados na seleção automática |
| `WIFI_FREQ_BAND` | não | `auto` | `2.4`, `5` ou seleção automática por capacidade da placa |
| `HOTSPOT_GATEWAY` | não | `192.168.12.1` | IP do hotspot na rede local |
| `DOCKER_HOST_GATEWAY` | não | — | IP do host visto pelos containers (bind extra do DNS) |
| `DNS_LOCAL_TLDS` | não | `local,test,example` | TLDs resolvidos como locais pelo CoreDNS |
| `TZ` | não | `Africa/Sao_Tome` | timezone dos containers |

As regras de resolução automática de canal/banda do hotspot estão
detalhadas em [RULE.md](RULE.md#serviço-hotspot-hotspotentrypointsh).

## Confiando na CA local (HTTPS sem avisos)

O `cert-proxy` expõe o certificado da sua CA local em qualquer domínio
servido pelo stack:

```bash
curl -o central-local-ca.crt http://<host-ou-dominio-local>/ca.crt
# ou, se o DNS local estiver ativo:
curl -o central-local-ca.crt http://ca.local/
```

Importe `central-local-ca.crt` no armazém de confiança do sistema
operacional ou do navegador que for acessar os sites locais via
HTTPS. Depois disso, qualquer domínio novo recebe um certificado
válido automaticamente na primeira requisição (emitido sob demanda,
veja [RULE.md](RULE.md#serviço-cert-proxy-cert-proxymaingo)).

## Estrutura do repositório

```
docker-compose.yml   # orquestra todos os serviços
.env.example          # template documentado de variáveis (versionado)
.env                   # valores reais da máquina (git-ignored, não versionar)
hotspot/                # Dockerfile + entrypoint.sh do serviço "hotspot" (create_ap)
coredns/                 # Dockerfile + Corefile + entrypoint do "dns-provider"
cert-proxy/                # Dockerfile + go.mod + main.go do proxy TLS com CA local
scripts/                    # hotspot-on.sh / hotspot-off.sh (mexem no host, pedem sudo)
RULE.md                      # regras de negócio de cada serviço
CLAUDE.md                     # diretrizes para o Claude Code neste repo
AGENTS.md                      # diretrizes para agentes de codificação (Codex etc.) neste repo
```

## Troubleshooting

- **Rede Wi-Fi não aparece após `hotspot-on.sh`**: confira
  `docker compose logs hotspot`; verifique se `WIFI_INTERFACE` suporta
  modo AP (`iw list` deve listar `AP` em "Supported interface modes").
- **`dns-provider` não sobe / fica esperando IPs**: ele espera
  `HOTSPOT_GATEWAY` (e `DOCKER_HOST_GATEWAY`, se definido) existirem
  como IPs na máquina antes de iniciar — normalmente indica que o
  `hotspot` ainda não subiu ou falhou.
- **Domínio local não resolve no host**: confirme que o resolver do
  sistema está apontando `127.0.0.1` para os TLDs de `DNS_LOCAL_TLDS`
  (ex: `systemd-resolved` com drop-in dedicado).
- **Erro "external volume not found"**: os volumes do `nginx-ui`/
  `cert-proxy` precisam ser criados manualmente antes do primeiro
  `docker compose up` (veja "Configuração inicial" acima).
