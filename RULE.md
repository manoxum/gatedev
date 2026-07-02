# Regras de negócio — bindnet

Este documento descreve o comportamento funcional esperado de cada
serviço do stack `bindnet`. É a referência de "o que o sistema deve
fazer"; para "como rodar" veja o [README.md](README.md).

## Visão geral do domínio

O `bindnet` provê, para uma rede local doméstica/pequeno escritório:

1. Um ponto de acesso Wi-Fi (hotspot) compartilhando a internet de uma
   interface cabeada/outra Wi-Fi.
2. Resolução DNS *split-horizon*, com resposta diferente conforme
   quem pergunta: domínios internos (TLDs locais, ex. `.local`)
   resolvem para um IP de loopback próprio quando a consulta vem do
   próprio host, para o gateway Docker quando vem de um container, ou
   para o gateway do hotspot quando vem de um cliente Wi-Fi; todo o
   resto é encaminhado para DNS público.
3. Uma CA local própria, com emissão explícita de certificados TLS sob
   demanda (via painel de gestão), para permitir HTTPS sem avisos de
   certificado durante desenvolvimento. Não há mais proxy público nas
   portas 80/443 — a emissão é sempre uma ação explícita do usuário.
4. Uma UI (`nginx-ui`) para administrar sites, acessível só pela porta
   administrativa `9080`.
5. Um painel web (`services/frontend` + `services/backend` +
   `services/worker`) para gerenciar hotspot/DNS e emitir/gerenciar
   certificados TLS, sem depender de shell/`.env` manual.
6. Um banco de dados PostgreSQL como armazenamento principal do
   stack, um MongoDB para trilha de auditoria, um Redis como cache do
   `dns-provider`, um MinIO provisionado para uso futuro, e um job
   Node.js/Prisma (`services/migration`) que aplica o schema do
   Postgres antes de `backend`/`dns-provider` subirem.

## Serviço `hotspot` (`services/worker/hotspot/entrypoint.sh`)

Regras:

- Variáveis obrigatórias: `WIFI_INTERFACE`, `INTERNET_INTERFACE`,
  `WIFI_SSID`, `WIFI_PASSWORD`. Ausência de qualquer uma delas é erro
  fatal (o container não sobe).
- `WIFI_CHANNE` (sem "L") é aceito como alias legado de
  `WIFI_CHANNEL`, com aviso de depreciação no log. Não usar em
  configurações novas.
- **`WIFI_CHANNEL`** aceita:
  - um número de canal fixo (validado como inteiro) — usado como está,
    sem retry nem fallback de banda: se `create_ap` rejeitar esse canal
    específico, o hotspot falha (respeita a escolha explícita do
    usuário); ou
  - `auto` (padrão): o hotspot escaneia o ambiente Wi-Fi
    (`iw dev ... scan`) e ordena os canais candidatos da banda
    resolvida do **menos para o mais interferido**.
    - Em 2.4GHz a pontuação penaliza canais sobrepostos (distância 0 a
      4 do canal observado); em 5GHz os canais são considerados
      ortogonais (pontuação binária, mesmo canal = interferência).
    - Se a varredura falhar ou não retornar redes, a ordem dos
      candidatos é arbitrária (com aviso no log) — o hotspot nunca
      trava por falta de varredura.
    - **Retry por canal**: o hotspot tenta `create_ap` em cada canal
      candidato, na ordem de menor interferência, até um que o
      adaptador realmente aceite — a pontuação de interferência é só
      um critério de desempate/ordenação, não uma garantia de que o
      canal é utilizável (o adaptador pode reportar suporte a uma
      banda via `iw phy info` e ainda assim rejeitar canais específicos
      dela, ex.: `create_ap` retornando "adapter can not transmit to
      channel X").
    - **Fallback de banda**: se `WIFI_CHANNEL` **e** `WIFI_FREQ_BAND`
      estiverem ambos em `auto` e nenhum canal da banda escolhida
      funcionar, o hotspot tenta automaticamente a outra banda (todos
      os seus canais candidatos, também em ordem de interferência)
      antes de desistir. Se `WIFI_FREQ_BAND` foi fixado explicitamente
      (`2.4` ou `5`), não há fallback de banda — só retry de canal
      dentro da banda escolhida.
  - Candidatos podem ser sobrescritos via `WIFI_CHANNEL_CANDIDATES`
    (lista separada por vírgula/espaço); caso contrário os candidatos
    padrão são `1 6 11` (2.4GHz) ou `36 40 44 48 149 153 157 161`
    (5GHz).
- **`WIFI_FREQ_BAND`** aceita `2.4`, `5` ou `auto` (padrão). Resolvida
  **antes** da seleção de canal, porque a lista de candidatos e a
  pontuação de interferência dependem da banda:
  - Se `WIFI_CHANNEL` já for um número fixo, a banda é **inferida do
    canal** (1–14 → 2.4GHz; caso contrário → 5GHz) — não faz sentido
    escanear banda quando o canal já decide isso.
  - Caso contrário, o hotspot inspeciona as capacidades da placa via
    `iw phy<N> info` e **prefere 5GHz quando suportado** (menos
    congestionamento, mais throughput); cai para 2.4GHz se a placa não
    suportar 5GHz ou se a detecção falhar (com aviso no log). Essa é só
    a banda **preferida** para a primeira tentativa — ver fallback de
    banda acima para o que acontece se nenhum canal dela funcionar.
- O hotspot exige que o binário `create_ap` baixado suporte
  `--no-dns` e `--dhcp-dns`; sem isso, falha explicitamente em vez de
  criar um AP com comportamento de DNS inesperado.
- DNS entregue via DHCP aos clientes do hotspot é sempre o próprio
  `HOTSPOT_GATEWAY` (o `dns-provider` responde por trás dele) — o
  hotspot nunca delega DNS para o `create_ap` (`--no-dns`).
- O DHCP do hotspot também anuncia `domain-search` para os TLDs locais
  (`DNS_SEARCH_DOMAINS`, ou `DNS_LOCAL_TLDS` quando ausente). Isso é
  necessário para clientes como Ubuntu/systemd-resolved rotearem
  `*.local`, `*.test` e `*.example` para o DNS do link Wi-Fi em vez de
  mDNS/outros links.
- Ao encerrar (`SIGTERM`/`SIGINT`/saída normal), o hotspot **sempre**
  chama `create_ap --stop` antes de sair, para não deixar a placa em
  modo AP "preso".
- O container precisa rodar `privileged: true` e `network_mode: host`
  porque manipula diretamente a interface Wi-Fi física do host.

## Ligar/desligar o hotspot pelo painel (`POST /api/hotspot/start` / `/stop`)

Não existem mais scripts de shell separados (`scripts/hotspot-on.sh` /
`hotspot-off.sh`, removidos) — ligar/desligar o hotspot é feito
inteiramente pelo painel de gestão, via `services/backend/hotspot.go`
delegando ao `services/worker/controller` (único serviço com acesso a
NetworkManager/`docker.sock`). O comportamento é o mesmo que os
scripts tinham, só que dentro do container privilegiado em vez de
`sudo` no host:

- `POST /api/hotspot/start`:
  1. Marca `WIFI_INTERFACE` (e `ap0`) como **não gerenciada** pelo
     NetworkManager, via drop-in em
     `/etc/NetworkManager/conf.d/90-bindnet-hotspot-unmanaged.conf`
     (`worker`: `POST /network/wifi-unmanage`), para o `hostapd`
     (dentro do `create_ap`) poder assumir a placa.
  2. Sobe `hotspot` + `dns-provider` via `docker compose up -d
     --no-build` (`worker`: `POST /hotspot/apply`) — cria os
     containers se ainda não existirem (1ª subida) e também os recria
     se o `.env` mudou, não apenas `docker start`.
- `POST /api/hotspot/stop` desfaz exatamente o inverso, na ordem
  inversa:
  1. Para `hotspot` + `dns-provider` (`docker stop`, via `worker`).
  2. Remove o drop-in do NetworkManager e devolve `WIFI_INTERFACE` ao
     controle do NetworkManager (`worker`: `POST /network/wifi-manage`,
     que roda `nmcli device set ... managed yes`).
- `WIFI_INTERFACE` vem da seção `hotspot` do `.env` (via `worker`); se
  não estiver definida, a requisição falha explicitamente (sem
  fallback silencioso para uma interface adivinhada).
- Essas operações mexem em configuração real do host (NetworkManager,
  placa Wi-Fi física) — o mesmo cuidado que se aplicava aos scripts
  antigos se aplica aos botões "Iniciar"/"Parar" do painel: iniciar o
  hotspot desconecta a placa Wi-Fi do uso normal como cliente; "Parar"
  é o único caminho suportado para reverter isso de forma limpa.

## Serviço `dns-provider` (servidor DNS split-horizon próprio — `services/worker/dns/`)

Não usa mais CoreDNS/Corefile — é um binário Go próprio (`miekg/dns`),
pelo mesmo motivo que a gestão de certificados saiu do cert-proxy: o
CoreDNS/`template` plugin não tem como implementar split-horizon **por
IP de bind** nem alocação persistente de IP por hostname sem plugins
customizados fora da imagem oficial.

Regras:

- `DNS_LOCAL_TLDS` define os TLDs tratados como locais (padrão
  `local,test,example`). Cada TLD é validado (`[a-z0-9-]`, sem começar
  ou terminar com `-`); TLD inválido é erro fatal. Duplicatas são
  ignoradas silenciosamente.
- `DOMAINS` define os TLDs do **discover mode** (ex.: `discover`).
  Para esses TLDs, qualquer consulta `A` (ex.: `painel.discover`,
  `qualquer-nome.discover`) responde com o IP LAN desta instância,
  igual em todas as views. O IP vem de `HOST_SOURCE_CIDR` sem o CIDR
  (ex.: `10.234.2.102/32` → `10.234.2.102`); se
  `HOST_SOURCE_CIDR` estiver vazio, o dns-provider detecta o IP pela
  rota padrão de saída. `DOMAINS` vazio desliga o discover mode. Use
  TLDs diferentes dos de `DNS_LOCAL_TLDS`, porque discover tem
  precedência quando houver sobreposição.
- **Split-horizon por três "views", uma por IP de bind** — o processo
  abre um socket UDP:53 separado para cada IP abaixo; como cada view é
  um socket próprio, o dns-provider sabe de onde a consulta veio só
  pelo bind que a recebeu, sem precisar inspecionar o IP remoto do
  cliente:
  - `127.0.0.1` (**view host**): consultas do próprio host (o
    `resolved`/resolver do sistema aponta pra cá). Registros `A` para
    os TLDs locais resolvem para um **IP de loopback (127.x.y.z)
    exclusivo daquele hostname**, alocado na primeira consulta e
    persistido no Postgres (tabela `local_dns_records`, coluna
    `loopback_offset` vinda de uma sequence que começa em 2 — o offset
    1/`127.0.0.1` fica reservado) — o mesmo hostname sempre resolve
    para o mesmo IP entre reinícios. Um cache Redis (hash
    `local_dns_records`, hostname → offset) fica na frente do Postgres
    e é **hidratado com todos os registros existentes na
    inicialização do serviço** (não espera cada hostname ser
    consultado de novo para voltar a estar em cache); cache miss vai
    ao Postgres, aloca se necessário, e grava de volta no Redis.
  - `DOCKER_HOST_GATEWAY` (**view container**): consultas vindas de
    containers Docker (que só alcançam o host via esse gateway).
    Registros `A` para os TLDs locais sempre respondem com o próprio
    `DOCKER_HOST_GATEWAY`.
  - `HOTSPOT_GATEWAY` (**view hotspot**): consultas vindas de clientes
    conectados ao hotspot Wi-Fi. Registros `A` para os TLDs locais
    sempre respondem com o próprio `HOTSPOT_GATEWAY`.
  - Em qualquer view, registros `AAAA`/`ANY` para os TLDs locais
    sempre respondem `NXDOMAIN` (sem IPv6, sem split-horizon
    multi-tipo) e `SOA` responde um registro sintético mínimo — sem
    isso, resolvers que validam autoridade de zona antes de aceitar a
    resposta (ex.: `systemd-resolved`) tratam a zona inteira como
    quebrada, mesmo a resposta `A` estando correta.
- Para qualquer outro domínio (em qualquer view), a consulta é
  encaminhada para DNS público (`8.8.8.8`, `1.1.1.1`).
- Antes de abrir os sockets principais, o processo **espera** (até
  `COREDNS_WAIT_TIMEOUT`, padrão 90s) `127.0.0.1` e
  `DOCKER_HOST_GATEWAY` existirem na máquina. O socket de
  `HOTSPOT_GATEWAY` é tratado em loop separado: se o hotspot ainda não
  criou o IP, o dns-provider continua vivo e tenta abrir esse listener
  novamente a cada poucos segundos.
- `dns-provider` roda com `network_mode: host` (precisa bindar IPs
  reais do host) e por isso **não enxerga a DNS interna do Docker**
  para resolver `postgres`/`redis` pelo nome do serviço — fala com
  eles pelos IPs fixos atribuídos na rede `proxy`
  (`POSTGRES_HOST`/`REDIS_HOST` apontam para esses IPs fixos no
  `docker-compose.yml`, não para os nomes dos serviços).
- Para que **qualquer container de qualquer projeto** resolva os TLDs
  locais pela view container, o Docker daemon do host deve usar
  `DOCKER_HOST_GATEWAY` como DNS upstream. O caminho versionado é
  `sudo bin/configure-docker-dns.sh`, que grava `"dns":
  ["10.90.0.1"]` (ou o valor de `DOCKER_HOST_GATEWAY`) em
  `/etc/docker/daemon.json`; aplicar de fato exige restart explícito do
  Docker pelo operador.

## Gestão de certificados (`services/backend/certificates.go`)

Substitui o antigo `services/worker/cert-proxy` (removido). A emissão
de certificados agora é sempre uma ação explícita, disparada por
request HTTP autenticado no painel — **não existe mais emissão
automática por SNI, nem nada escutando nas portas 80/443**. Isso é uma
mudança de comportamento intencional, não um efeito colateral.

Regras:

- `loadOrImportCA` roda uma vez, no boot do `backend`, com
  precedência de 3 níveis:
  1. Já existe uma linha na tabela `ca` do Postgres → usa essa, nunca
     regenera.
  2. Senão, se o volume legado `cert_proxy_data` (montado
     somente leitura em `/certproxy-data`) tiver `ca.crt`+`ca.key` →
     importa para o Postgres (`source='imported'`). Isso preserva a
     confiança já estabelecida nos dispositivos que já importaram essa
     CA quando o stack ainda usava `cert-proxy`.
  3. Senão → gera uma CA nova (`source='generated'`): RSA 4096, válida 10
     anos, CN de `CA_COMMON_NAME` (padrão "Bindnet Local Development
     CA"). Mesmos parâmetros criptográficos do antigo `cert-proxy`.
- `POST /api/certificates` emite um certificado *leaf* (RSA 2048,
  válido 2 anos) para o domínio informado, sempre como uma linha nova
  na tabela `certificates` — sem cache/reuso por domínio, já que
  emitir é agora uma ação explícita do usuário, não um lookup
  implícito por SNI.
- O nome do domínio é normalizado antes de emitir: minúsculas, sem
  porta, sem `.` final; se não passar numa validação básica de
  caracteres (`[a-z0-9-]` por rótulo), cai para `localhost.local` em
  vez de emitir certificado para um valor não sanitizado — mesma regra
  do antigo `cert-proxy`.
- `DELETE /api/certificates/{id}` revoga: seta `revoked_at`, **nunca
  deleta a linha** — o certificado revogado continua aparecendo na
  listagem, com status "revogado".
- `GET /api/certificates/ca` e `GET /api/certificates/{id}/download`
  servem os PEMs para download. **Todas** as rotas de
  `/api/certificates/*` exigem sessão autenticada — diferente do
  antigo `cert-proxy`, que servia `/ca.crt` anonimamente na porta 80.
- A chave privada da CA (`private_key_pem` na tabela `ca`) fica no
  `postgres_data` — apagar/recriar esse volume tem o mesmo
  efeito que apagar o antigo `cert_proxy_data`: invalida a CA e todos
  os certificados já confiados nos dispositivos do usuário.

## Serviço `nginx-ui`

- Interface administrativa para configurar sites, exposta só em `9080`
  (porta administrativa) — não há mais proxy público nas portas 80/443
  na frente dela.
- Não monta `/var/run/docker.sock`: essa capacidade fica exclusivamente
  no `worker`. O container define `NGINX_UI_IGNORE_DOCKER_SOCKET=true`
  para a instalação do Nginx UI aceitar esse desenho de segurança.
- Estado (configuração de sites, dados da UI, arquivos servidos) é
  persistido nos volumes externos `nginx_config`, `nginx_ui_data` e
  `www_data` — esses volumes **não** são gerenciados pelo
  `docker-compose.yml` (são `external: true`) e precisam existir antes
  do primeiro `docker compose up` (ver README).

## Banco de dados e armazenamento (`postgres`, `mongo`, `minio`, `redis`, `migration`)

- **Postgres é o banco de dados principal** do stack. Hoje guarda a CA
  local e os certificados emitidos (`services/backend/certificates.go`)
  e o mapeamento hostname → IP de loopback do `dns-provider`
  (`services/worker/dns/db.go`); disponível para uso por futuras
  funcionalidades. Tanto `backend` quanto `dns-provider` acessam via
  `database/sql` + driver `pgx` puro — nunca via Prisma Client (que é
  Node-only). `postgres` e `redis` têm **IP fixo** na rede `proxy`
  (`docker-compose.yml`, bloco `networks`) porque `dns-provider` roda
  em `network_mode: host` e não enxerga a DNS interna do Docker para
  resolvê-los pelo nome do serviço.
- O schema do Postgres é criado/atualizado só pelo serviço
  `migration` (`services/migration/`, Node.js + Prisma), que roda
  `prisma migrate deploy` uma vez no boot e encerra (`restart: "no"`).
  O `backend` só sobe depois dele terminar com sucesso
  (`depends_on: migration: condition: service_completed_successfully`
  no `docker-compose.yml`) — nunca cria/altera tabelas sozinho.
- **Mongo guarda só a trilha de auditoria do painel** — eventos
  discretos de ações administrativas: `login`, `logout`,
  `config_changed` (hotspot/DNS), `certificate_issued`,
  `certificate_revoked`, `hotspot_started`, `hotspot_stopped`,
  `dns_applied`. **Não** persiste logs de containers — isso continua
  sendo só streaming ao vivo via `worker` (`LogsPanel.tsx`,
  `worker.streamLogs`). Falha ao gravar auditoria nunca derruba a ação
  principal do usuário (só loga o erro).
- **MinIO está provisionado mas sem uso ainda** — nenhum endpoint do
  `backend` usa isso hoje; existe só para necessidade futura de
  armazenamento de arquivos.
- **Redis é cache, nunca fonte de verdade** — hoje só o `dns-provider`
  usa (mapeamento hostname → offset de IP de loopback, hidratado do
  Postgres na inicialização; ver seção do `dns-provider` acima).
  Perder o volume do Redis não perde dado nenhum, só custa uma
  rehidratação no próximo boot do serviço que o usa.
- Nenhum dos serviços de dados publica porta no host por padrão —
  ficam só na rede interna `proxy`, mesma postura de exposição mínima
  do resto do stack.

## Painel de gestão (`services/frontend` + `services/backend` + `services/worker/controller`)

Regras:

- **`worker`** é o único serviço com acesso privilegiado ao host
  (`/var/run/docker.sock`, `/run/dbus`, `/etc/NetworkManager/conf.d`,
  `network_mode: host`). Sua API interna (servida só via socket Unix
  em `worker_ipc`, sem porta TCP) opera com uma lista fechada de
  serviços permitidos (`hotspot`, `dns-provider`, `nginx-ui`,
  `postgres`, `mongo`, `minio`) e ações específicas via
  `docker compose --project-name bindnet` — nunca executa comando
  arbitrário vindo do backend. O `worker` não depende de `hotspot` nem
  `dns-provider` no Compose, para que o painel suba sem acionar a rede
  física; esses serviços só sobem quando o usuário aplica/inicia o
  hotspot pelo painel. `migration` (job one-shot) propositalmente não
  está nessa lista.
- Editar `.env` a partir do painel (`GET/PATCH /env` no worker) é
  restrito por "seção": a seção `hotspot` só pode tocar
  `WIFI_*`/`HOTSPOT_GATEWAY`/`HOTSPOT_CIDR`; a seção `dns` só pode
  tocar `DNS_LOCAL_TLDS`. O editor preserva comentários, ordem e
  chaves não mencionadas — nunca regenera o arquivo do zero.
- "Salvar" configuração (grava no `.env`) e "aplicar" (recria o
  container via `docker compose up -d --no-build`) são ações
  separadas — o painel nunca reinicia o hotspot/DNS sozinho ao salvar,
  só quando o usuário confirma explicitamente "aplicar".
- **`backend`** nunca monta `docker.sock` nem roda em
  `network_mode: host`; toda operação privilegiada (controlar
  containers, NetworkManager, listar interfaces/clientes, testar DNS)
  é delegada ao `worker`. Exceção: `backend` pode montar volumes de
  dados somente leitura (ex.: `cert_proxy_data:/certproxy-data:ro`)
  para ler arquivos diretamente, já que isso não é controle de
  host/Docker.
- Login é obrigatório: `backend` guarda um único usuário administrador
  em `/data/admin.json` (senha derivada por HMAC-SHA256 iterado +
  segredo de sessão). No primeiro boot, `ADMIN_USERNAME` e
  `ADMIN_PASSWORD` criam esse arquivo; em boots seguintes, se ambas
  estiverem definidas, o backend sincroniza o usuário persistido com o
  `.env` e troca o segredo de sessão quando as credenciais mudarem.
  Se ambas ficarem vazias e o arquivo já existir, o administrador
  persistido continua valendo.
- Go em `services/worker/controller` segue a convenção original: sem
  dependências externas no `go.mod` (inclusive autenticação usa
  HMAC-SHA256 da biblioteca padrão em vez de bcrypt/JWT de terceiros).
  `services/backend` é a exceção deliberada: tem `github.com/jackc/pgx/v5`
  (Postgres) e `go.mongodb.org/mongo-driver/v2` (Mongo) como
  dependências, porque não há alternativa razoável de biblioteca
  padrão para os protocolos de rede desses bancos — diferente do caso
  HMAC-SHA256, que a stdlib já resolve bem.

## Regras transversais

- Nenhum segredo (senha de Wi-Fi, etc.) deve ir para o Git: apenas
  `.env.example` (com placeholders) é versionado; `.env` real é
  ignorado (`.gitignore`).
- Todo `entrypoint.sh`/script novo deve seguir `set -euo pipefail` (ou
  equivalente em `sh`), falhar alto (erro explícito + `exit 1`) em vez
  de continuar em estado inconsistente, e logar em português com
  prefixo `[nome-do-servico]`, no padrão já usado pelos scripts
  existentes.
- Serviços que dependem de estado de rede do host (`hotspot`,
  `dns-provider`, `worker`) usam `network_mode: host`; os demais
  (`nginx-ui`, `cert-proxy`, `backend`, `frontend`) ficam isolados na
  rede Docker `proxy`.
