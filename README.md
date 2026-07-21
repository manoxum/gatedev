# bindnet

Stack Docker Compose de infraestrutura de rede local: hotspot Wi-Fi
compartilhado, DNS split-horizon para domínios locais, uma UI para
administrar sites (`nginx-ui`), e um painel web próprio
(`services/frontend` + `services/backend` + `services/worker`) para
gerenciar hotspot/DNS e emitir/gerenciar certificados TLS assinados por
uma CA local, sem precisar mexer em shell/`.env` manualmente. O painel
também é a porta de entrada para o banco de dados principal
(PostgreSQL), a trilha de auditoria (MongoDB) e armazenamento de
arquivos provisionado para uso futuro (MinIO).

> Regras de negócio detalhadas por serviço: veja [RULE.md](RULE.md).
> Diretrizes para ferramentas de IA trabalhando neste repo:
> veja [CLAUDE.md](CLAUDE.md).

## Arquitetura

```
                 (clientes Wi-Fi: celular, notebook, etc.)
                                │
                          rede "xCosta" (WIFI_SSID)
                                │
                    ┌───────────────────────┐
                    │   hotspot (create_ap) │  privileged, network_mode: host
                    │   WIFI_INTERFACE → AP │
                    └───────────┬───────────┘
                                │ NAT para internet via uplink virtual
                                │
                    ┌───────────────────────┐
                    │  dns-provider (Go)    │  network_mode: host
                    │  split-horizon TLDs   │  3 sockets: host/
                    │  locais, 3 views      │  container/hotspot
                    └───────────────────────┘

                    ┌───────────────┐   painel de gestão web
                    │  nginx-ui     │◄──┐ (login, hotspot, DNS, certificados)
                    │  :9080 admin  │   │
                    └───────────────┘   │
                                        │
      ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌───────────────────────────┐
      │ frontend │──►│ backend  │──►│  worker  │   │ postgres/mongo/minio/redis│
      │  :9090   │   │  :8090   │   │ (socket) │   │  (rede interna)           │
      └──────────┘   └────┬─────┘   └──────────┘   └───────────────────────────┘
                          │              ▲
                          └──────────────┘
                     backend fala com postgres/mongo direto (rede "proxy");
                     dns-provider fala com postgres/redis por IP fixo (nao
                     enxerga a DNS do Docker em network_mode:host); nada
                     escuta em 80/443
```

- **hotspot** (`services/worker/hotspot/`) — cria o ponto de acesso
  Wi-Fi (via `create_ap`), compartilhando a internet de
  `INTERNET_INTERFACE`.
- **dns-provider** (`services/worker/dns/`) — servidor DNS Go próprio
  (não usa CoreDNS): resolve TLDs locais (`DNS_LOCAL_TLDS`) de forma
  diferente conforme quem pergunta (host/container/hotspot, ver
  [RULE.md](RULE.md)), usa `server_name` declarados no nginx-ui como
  anúncios locais, trata zonas concretas de discover (`DOMAINS`, ex.:
  `costa.bnet`) como domínios locais anunciados aos peers, e roteia
  domínios remotos conhecidos por próximo salto entre servidores
  Bindnet vizinhos cadastrados no Postgres pelo painel: domínio local resolve
  normalmente, domínio remoto conhecido é encaminhado (proxy real) para
  o próximo salto, e o resto é encaminhado para DNS público.
- **nginx-ui** — interface de administração para configurar sites,
  acessível só pela porta administrativa `:9080` (nada mais faz proxy
  público nas portas 80/443 neste stack). Não recebe
  `/var/run/docker.sock`; a checagem correspondente da instalação é
  desativada por variável de ambiente porque controle Docker fica só no
  `worker`.
- **worker** (`services/worker/controller/`) — único serviço com
  acesso privilegiado ao host (`docker.sock`, NetworkManager,
  `network_mode: host`); orquestra hotspot/dns-provider e expõe uma
  API interna restrita via socket Unix, usada só pelo `backend`.
- **backend** (`services/backend/`) — API pública do painel de gestão
  (login, regras de negócio, agrega status, emite/gerencia
  certificados assinados por uma CA local armazenada no Postgres);
  nunca toca o host diretamente, delega tudo privilegiado ao `worker`.
- **frontend** (`services/frontend/`) — interface web (React) do
  painel de gestão, consumida pelo navegador.
- **postgres** — banco de dados principal do stack (dados de
  certificados, ver [RULE.md](RULE.md)); imagem oficial, sem pasta
  própria em `services/`.
- **mongo** — trilha de auditoria do painel (login/logout, mudanças de
  config, emissão/revogação de certificado, start/stop de
  hotspot/dns); imagem oficial, sem pasta própria em `services/`.
- **minio** — provisionado para armazenamento de arquivos futuro;
  nenhum serviço usa isso hoje; imagem oficial, sem pasta própria em
  `services/`.
- **redis** — cache do `dns-provider` (hostname → IP de loopback),
  hidratado a partir do Postgres na inicialização; imagem oficial, sem
  pasta própria em `services/`.
- **migration** (`services/migration/`) — job Node.js/Prisma que roda
  `prisma migrate deploy` no Postgres e encerra; `backend` só sobe
  depois dele terminar com sucesso.

## Pré-requisitos

- Linux com Docker Engine + Docker Compose v2 (`docker compose ...`).
- Para o hotspot: placa Wi-Fi compatível com modo AP e NetworkManager
  instalado — o `worker` (que roda com `network_mode: host` e acesso a
  `/run/dbus`) reconfigura o NetworkManager por dentro do container via
  `nmcli`; não é mais necessário `sudo` no host nem scripts externos, o
  painel faz isso pelos botões "Iniciar"/"Parar" do hotspot.
- Opcional: Go 1.25+ e Node 22+ apenas se quiser compilar/rodar
  `backend`/`worker`/`migration` fora do Docker.

## Configuração inicial

1. Ajuste o arquivo de ambiente principal para a sua máquina
   (interfaces de rede, SSID, senha, etc.):

   ```bash
   cp .env.example .env.main
   $EDITOR .env.main
   ```

   Veja os comentários em [.env.example](.env.example) para o
   significado, obrigatoriedade e valor padrão de cada variável.
   O fluxo oficial usa `promote.yml` + `.env.main`; `.env` continua
   reservado como fallback local/legado e não deve ser commitado.

2. Os volumes Docker do stack (`nginx_config`,
   `nginx_ui_data`, `www_data`, `cert_proxy_data`,
   `admin_data`, `postgres_data`, `mongo_data`,
   `minio_data`) são volumes gerenciados pelo Docker Compose,
   sem `name:` fixo e sem `external: true` — o próprio
   `docker compose up` cria cada um automaticamente na primeira vez
   que sobe o stack, dentro do projeto Compose em uso. Não é preciso
   rodar `docker volume create` manualmente.

   `cert_proxy_data` é legado (do antigo `cert-proxy`, já
   removido): hoje só é usado, uma única vez, para o `backend` importar
   a CA existente para o Postgres. `postgres_data` passa a
   guardar a chave privada dessa CA — trate-o com o mesmo cuidado que o
   volume antigo (nunca apagar/recriar sem entender que isso invalida a
   confiança já estabelecida nos dispositivos do usuário). **Como esses
   volumes não são `external`, `docker compose down -v` os apaga —
   nunca rode `down -v` neste stack.**

   (`worker_ipc`, usado só para o socket Unix entre `backend` e
   `worker`, também não precisa ser criado manualmente — é recriado a
   cada subida.)

## Subindo os serviços

### UI de administração (sem mexer na rede/Wi-Fi do host)

```bash
docker compose up -d nginx-ui
```

### Hotspot Wi-Fi completo (hotspot + DNS split-horizon)

Ligar/desligar o hotspot é feito pelo painel de gestão web (botões
"Iniciar"/"Parar" na tela "Hotspot"), não por scripts externos — o
`worker` já reconfigura o NetworkManager (marca/desmarca
`WIFI_INTERFACE` como não-gerenciada) e sobe/derruba `hotspot` +
`dns-provider` internamente, exatamente como um script `sudo` faria,
mas por dentro do container privilegiado. Suba pelo menos
`worker`/`backend`/`frontend` (veja "Painel de gestão web" abaixo) e
use a UI; subir o painel sozinho não inicia `hotspot` nem
`dns-provider`.

⚠️ Iniciar o hotspot desconecta a placa Wi-Fi (`WIFI_INTERFACE`) do uso
normal como cliente enquanto estiver ativo. Use o botão "Parar" para
reverter isso de forma limpa. Se o host continuar mostrando "No Wi-Fi
Adapter Found" depois de uma queda/restart fora do painel, use
"Recuperar Wi-Fi" na tela "Hotspot Wi-Fi".

### Painel de gestão web

O painel depende de `postgres`/`mongo`/`migration` estarem prontos
antes do `backend` subir (o Compose já garante essa ordem via
`depends_on`):

```bash
docker compose up -d postgres mongo minio migration worker backend frontend
```

Depois de subir, acesse `http://<ip-do-host>:9090` (porta configurável
via `FRONTEND_PORT`) e entre com `ADMIN_USERNAME`/`ADMIN_PASSWORD`
definidos no `.env`. Se essas variáveis mudarem e o `backend` iniciar
novamente, o hash persistido do usuário administrador é atualizado
automaticamente. Pelo painel dá para ligar/desligar o
hotspot, editar SSID/senha/canal, ver clientes conectados, editar TLDs
locais do DNS e emitir/listar/revogar/baixar certificados TLS assinados
por uma CA local — sem precisar rodar `docker compose`/editar `.env`
manualmente no dia a dia.

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
| `INTERNET_INTERFACE` | sim | — | interface real com saída para a internet, ou `auto` (melhor interface real entre as rotas padrão, por velocidade reportada); pode ser igual a `WIFI_INTERFACE` se a placa suportar AP+STA concorrente |
| `BINDNET_UPLINK_INTERFACE` | não | `bn-uplink` | interface dummy estável entregue ao `create_ap`; o Bindnet alimenta essa interface com NAT/forward da interface real escolhida |
| `UPLINK_MONITOR_INTERVAL` | não | `10` | intervalo, em segundos, para reavaliar a melhor fonte quando `INTERNET_INTERFACE=auto` |
| `WIFI_SSID` | sim | — | nome da rede Wi-Fi criada |
| `WIFI_PASSWORD` | sim | — | senha WPA2 da rede |
| `WIFI_COUNTRY` | não | `ST` | código regulatório de país |
| `WIFI_CHANNEL` | não | `auto` | canal fixo ou seleção automática por varredura |
| `WIFI_CHANNEL_CANDIDATES` | não | por banda | candidatos avaliados na seleção automática |
| `WIFI_FREQ_BAND` | não | `auto` | `2.4`, `5` ou seleção automática por capacidade da placa |
| `HOTSPOT_GATEWAY` | não | `192.168.12.1` | IP do hotspot na rede local |
| `HOTSPOT_DNS_FALLBACKS` | não | `1.1.1.1,8.8.8.8` | DNS públicos entregues depois do gateway Bindnet no DHCP, para navegação continuar se o `dns-provider` reiniciar |
| `HOST_SOURCE_CIDR` | não | vazio | IP/CIDR da interface LAN onde o `dns-provider` escuta consultas encaminhadas por peers Bindnet |
| `DNS_LOCAL_TLDS` | não | `local,test,example` | TLDs resolvidos como locais pelo `dns-provider` |
| `DOMAINS` | não | vazio | TLDs/raízes amplas ou zonas concretas que participam do discover mode; zonas concretas como `costa.bnet` são anunciadas como locais |
| `DISCOVER_REMOTE_ROUTES` | não | `auto` | `auto` aprende vizinhos remotos anunciados pelo peer direto; `manual` exige adicionar esses vizinhos pelo painel |
| `DISCOVER_NODE_NAME` | não | hostname do container | nome deste servidor na malha de descoberta |
| `DISCOVER_PORT` | não | `8531` | porta HTTP onde este nó expõe sua tabela de rotas aos peers |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | sim (criação/atualização) | — | credenciais do painel web |
| `BACKEND_PORT` | não | `8090` | porta da API do painel (`backend`) |
| `FRONTEND_PORT` | não | `9090` | porta da interface web do painel (`frontend`) |
| `POSTGRES_PORT` | não | `9054` | porta local publicada para manutenção do Postgres |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | sim | — | credenciais do banco principal (dados de certificados) |
| `MONGO_USER` / `MONGO_PASSWORD` / `MONGO_DB` | sim | — | credenciais da trilha de auditoria do painel |
| `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` | sim | — | credenciais do MinIO (provisionado, sem uso ainda) |
| `CA_COMMON_NAME` | não | `Bindnet Local Development CA` | nome (CN) da CA local, só na primeira geração |
| `TZ` | não | `Africa/Sao_Tome` | timezone dos containers |

As regras de resolução automática de canal/banda do hotspot estão
detalhadas em [RULE.md](RULE.md#serviço-hotspot-hotspotentrypointsh).

## Confiando na CA local (HTTPS sem avisos)

A gestão de certificados agora é feita pelo painel de gestão web
(`services/backend`), não mais por um proxy público anônimo — nada
escuta mais nas portas 80/443. Para baixar a CA:

1. Acesse o painel (`http://<ip-do-host>:9090`) e faça login.
2. Vá em "Certificados" e clique em "Baixar CA" ou, no próprio servidor
   Linux onde o Bindnet roda, clique em "Instalar localmente".

Importe o arquivo baixado (`bindnet-local-ca.crt`) no armazém de
confiança do sistema operacional ou do navegador que for acessar
sites assinados por essa CA. A instalação local pelo painel é delegada
ao `worker`: ele grava a CA em
`/usr/local/share/ca-certificates/bindnet-local-ca.crt` e executa
`update-ca-certificates` no host Linux. Diferente do antigo `cert-proxy`,
certificados para domínios específicos não são mais emitidos
automaticamente por SNI — use o formulário "Emitir certificado" na
mesma tela para gerar um certificado para um domínio/IP específico.

Se você está migrando de uma instalação anterior com `cert-proxy`: a
CA já existente (e já confiada nos seus dispositivos) é importada
automaticamente para o Postgres no primeiro boot do `backend`, desde
que o volume `cert_proxy_data` ainda exista — nenhuma ação
manual é necessária, e os dispositivos que já confiam na CA antiga
continuam funcionando sem precisar reimportar nada (veja
[RULE.md](RULE.md#gestão-de-certificados-servicesbackendcertificadosgo)).

## Operação

O entrypoint operacional do repo é o mesmo padrão usado no FlowKey:
`Makefile` delega para o submódulo `bin/promote`, e `promote.yml`
define estágio atual, arquivos Compose, volumes/overlays de dev e
segredos declarados.

Comandos principais:

```bash
make dev        # bin/promote dev
make run-local  # service
make run-infra
make run-service
make stop-all
make dist       # resolve includes para deployment/
```

O compose é dividido em camadas:

- `docker-compose.infra.yml`: nginx-ui, Postgres, Mongo, MinIO e Redis.
- `docker-compose.services.yml`: migration, dns-provider, hotspot, worker, backend e frontend.
- `docker-compose.assets.build.yml`: contextos de build dos serviços próprios.
- `docker-compose.deploy.yml`: agregador de infra + services.
- `docker-compose.yml`: agregador principal usado por `bin/promote`.

## Estrutura do repositório

```
bin/                    # submodulo docker-cli; fornece bin/promote
promote.yml             # configuracao operacional do bin/promote
Makefile                # atalhos oficiais para bin/promote
docker-compose*.yml     # camadas Compose (infra/services/assets/deploy)
.env.example            # template documentado de variáveis (versionado)
.env.main               # ambiente principal usado por promote.yml/current_stage
scripts/                # utilitarios especificos do Bindnet
services/
  frontend/               # React (Vite) - UI do painel de gestão, container próprio
  backend/                # Go - API pública do painel (auth, hotspot/DNS, certificados)
  migration/              # Node/TS + Prisma - roda "prisma migrate deploy" no Postgres e encerra
  worker/
    controller/             # Go - único serviço com docker.sock/NetworkManager/host network
    hotspot/                # Dockerfile + entrypoint.sh do serviço "hotspot" (create_ap)
    dns/                    # Go - servidor DNS split-horizon proprio do "dns-provider" (sem CoreDNS)
RULE.md                   # regras de negócio de cada serviço
CLAUDE.md                   # diretrizes para ferramentas de IA neste repo
```

`postgres`, `mongo` e `minio` são imagens oficiais configuradas só no
`docker-compose.infra.yml`/`.env.main` — não têm pasta própria em
`services/`.

## Troubleshooting

- **Rede Wi-Fi não aparece depois de clicar em "Iniciar" no painel**:
  confira `docker compose logs hotspot`; verifique se `WIFI_INTERFACE`
  suporta modo AP (`iw list` deve listar `AP` em "Supported interface
  modes").
- **Ubuntu/GNOME mostra "No Wi-Fi Adapter Found" após usar o hotspot**:
  na tela "Hotspot Wi-Fi", clique em "Recuperar Wi-Fi". O painel para
  `hotspot`/`dns-provider`, recarrega o NetworkManager e devolve
  `WIFI_INTERFACE` ao controle normal do sistema.
- **`dns-provider` não sobe / fica esperando IPs**: ele detecta os
  gateways das bridges Docker existentes no host, lê os IPs de
  `HOST_SOURCE_CIDR`, espera esses IPs e `127.0.0.1` existirem como IPs
  na máquina antes de iniciar. O listener de `HOTSPOT_GATEWAY` é aberto
  depois, automaticamente, quando o hotspot criar esse IP.
- **Domínio local não resolve no host**: ao aplicar o DNS, o worker cria
  ou atualiza a conexão dummy `bindnet-dns` no NetworkManager, apontando
  `DNS_LOCAL_TLDS` e `DOMAINS` como rotas DNS (`~dominio`) para
  `127.0.0.1`. Confirme o estado efetivo com `resolvectl status bn-dns`.
- **Domínio local não resolve dentro de containers de outros projetos**:
  configure o Docker daemon para usar o DNS do `bindnet` como upstream:
  `sudo scripts/configure-docker-dns.sh` e depois reinicie o Docker. Sem
  isso, o DNS embutido do Docker (`127.0.0.11`) encaminha para o
  resolver do host e recebe a resposta da view host (`127.x.y.z`) em
  vez da view container (gateway Docker detectado no host).
- **`backend` não sobe / erro "ADMIN_USERNAME e ADMIN_PASSWORD são
  obrigatórios"**: defina essas variáveis no `.env.main` para criar o
  primeiro usuário administrador. Depois disso, manter ambas definidas
  faz o backend sincronizar o login com o env principal a cada boot; deixar
  ambas vazias reaproveita o `/data/admin.json` já persistido.
- **`backend` não sobe / erro ao conectar no Postgres ou Mongo**:
  confirme que `POSTGRES_PASSWORD`/`MONGO_PASSWORD` estão definidos no
  `.env.main` e que os containers `postgres`/`mongo` estão rodando e
  saudáveis (`docker compose ps`); o `backend` só sobe depois do
  `migration` terminar com sucesso, então também vale checar
  `docker compose logs migration`.

## Nota de segurança do painel de gestão

- `/var/run/docker.sock` (montado só no `worker`) dá acesso
  root-equivalente ao host — por isso `backend` e `frontend` nunca o
  montam, e a API interna do `worker` é uma lista fechada de ações
  (nunca "exec arbitrário").
- Como o DNS split-horizon responde `*.local`/`*.test`/`*.example` com
  o `HOTSPOT_GATEWAY` para qualquer dispositivo conectado ao próprio
  hotspot, evite expor `frontend`/`backend` via um hostname amigável
  `*.local` pelo `nginx-ui` por padrão — mantenha o acesso só por
  IP:porta direta (`FRONTEND_PORT`/`BACKEND_PORT`) a menos que você
  decida expor deliberadamente depois.
