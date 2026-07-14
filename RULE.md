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
      channel X"). **Exceção**: se o `create_ap` recusar com "adapter
      can not be a station ... and an AP at the same time" (
      `WIFI_INTERFACE` associado como cliente Wi-Fi num adaptador que
      não suporta AP+estação simultâneos — ver item abaixo), o hotspot
      **não** varre os demais canais nem tenta a banda alternativa:
      trocar canal/banda nunca resolve essa causa especificamente, só
      adiaria o mesmo erro por mais tentativas inúteis.
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
- **`INTERNET_INTERFACE`** aceita o nome de uma interface fixa ou
  `auto`: nesse caso o hotspot avalia as rotas padrão IPv4 do host
  (`ip -o route show default`), ignora interfaces virtuais/loopback, e
  escolhe a interface real com maior velocidade reportada em
  `/sys/class/net/<iface>/speed`; a métrica da rota desempata. Se não
  houver rota padrão, tenta interfaces reais `UP`; se ainda assim não
  encontrar nenhuma candidata, falha explicitamente — mesma filosofia de
  nunca adivinhar silenciosamente já usada para canal/banda (`auto` só
  age quando o valor é explicitamente esse; a variável continua
  obrigatória e sem fallback quando ausente). A interface real escolhida
  **não é passada diretamente ao `create_ap`**: o hotspot cria uma
  interface dummy estável (`BINDNET_UPLINK_INTERFACE`, padrão
  `bn-uplink`) e instala regras próprias em chains `BINDNET-HOTSPOT` de
  `FORWARD`/`POSTROUTING` para alimentar esse uplink virtual pela fonte
  real. No modo `auto`, um monitor (`UPLINK_MONITOR_INTERVAL`, padrão
  10s) reavalia a melhor interface em tempo real e troca somente essas
  regras, sem derrubar/recriar o AP. `INTERNET_INTERFACE` pode ser a
  **mesma** interface de `WIFI_INTERFACE` (hotspot e saída de internet
  pela mesma placa física, modo AP+STA concorrente no mesmo rádio) — o
  hotspot detecta esse caso e loga um aviso citando se `iw phy<N>
  info` reporta suporte a combinações `AP`+`managed` simultâneas, mas
  **nunca bloqueia**: quem decide se funciona de fato é o próprio
  `create_ap`, exatamente como no retry de canal. O mesmo vale mesmo
  quando `WIFI_INTERFACE` já está conectado como estação a **outra**
  rede (não à internet compartilhada por `INTERNET_INTERFACE`) no
  momento em que o hotspot sobe: se `iw phy<N> info` reportar a
  combinação `AP`+`managed`, o `create_ap` cria uma interface virtual
  (`ap0`) e mantém a conexão de estação existente. Quando a interface
  física não está associada como cliente Wi-Fi, o hotspot passa
  `--no-virt` e usa a própria interface física em modo AP: uma virtual
  não preservaria conexão alguma e alguns drivers/kernels (por exemplo,
  `iwlwifi`/AX211 no kernel 7.0) deixam criar `ap0`, mas recusam com
  `RTNETLINK ... Resource busy` a troca do MAC duplicado feita em
  seguida pelo `create_ap`. Com uma estação ativa, o `create_ap`
  **sobrescreve o canal/banda pedido pelo do rádio já associado**
  (a combinação normalmente vem com `#channels <= 1` — os dois modos
  têm que estar na mesma frequência; isso é decisão do próprio
  `create_ap`, não da seleção de canal do hotspot). A imagem
  `services/worker/hotspot/Dockerfile` instala o pacote `grep` (GNU
  grep) especificamente para essa detecção funcionar: o `grep` do
  BusyBox (padrão do Alpine) não processa a regex que o `create_ap`
  usa contra a saída de `iw phy info` (`{` sem bound válido) e sempre
  falha, fazendo o hotspot concluir erroneamente, em qualquer
  adaptador, que o modo concorrente não é suportado.
- O painel filtra do seletor de `INTERNET_INTERFACE` (e de
  `WIFI_INTERFACE`) qualquer interface virtual que nunca é uma saída de
  internet real (`docker*`, `br-*` gerada pelo Docker, `veth*`,
  `virbr*`, `tun*`, `tap*`, `wg*`, `bn-*` — uplink dummy Bindnet — e
  `ap0`, a virtual que o próprio `create_ap` cria) — `GET
  /network/interfaces` (worker) já devolve só interfaces
  físicas/relevantes.
- O hotspot exige que o binário `create_ap` baixado suporte
  `--no-dns` e `--dhcp-dns`; sem isso, falha explicitamente em vez de
  criar um AP com comportamento de DNS inesperado.
- DNS entregue via DHCP aos clientes do hotspot é sempre o próprio
  `HOTSPOT_GATEWAY` em primeiro lugar (o `dns-provider` responde por
  trás dele), seguido por `HOTSPOT_DNS_FALLBACKS` (padrão
  `1.1.1.1,8.8.8.8`) para manter navegação externa se o
  `dns-provider` reiniciar ou ainda não estiver escutando. O hotspot
  nunca delega DNS para o `create_ap` (`--no-dns`).
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
- **Detecção de falha de beacon e auto-recuperação em duas camadas**
  (`services/worker/hotspot/watchdog.sh` + laço de retry no fim de
  `entrypoint.sh` + `reconcileHotspotOnce` em
  `services/backend/hotspot_reconcile.go`): alguns drivers/kernels
  reproduzem uma regressão conhecida do hostapd 2.11 (802.11be/MLO —
  ver comentário no `Dockerfile`, que por isso fixa `hostapd=2.10-r6`)
  onde, após um cliente Wi-Fi associar/reassociar com o AP já de pé, o
  adaptador passa a recusar `Failed to set beacon parameters`
  indefinidamente sem o `create_ap` sair sozinho — o hotspot fica
  "vivo" mas inutilizável para novos clientes.
  - **Camada 1 (worker, imediata)**: o hotspot acompanha seu próprio
    log em tempo real e, se essa falha se repetir
    (`HOTSPOT_BEACON_FAILURE_THRESHOLD`, padrão 2, dentro de
    `HOTSPOT_BEACON_FAILURE_WINDOW_SECONDS`, padrão 20 — variáveis de
    ambiente do container, sem equivalente no painel), derruba o
    `create_ap` (mesmo sinal de parada limpa usado por
    `stop`/`SIGTERM`). O próprio `entrypoint.sh`, na mesma execução do
    script, detecta que o `create_ap` caiu sem que uma parada de
    verdade tenha sido pedida e tenta de novo no lugar, com backoff
    crescente (`HOTSPOT_RESTART_BACKOFF_SECONDS`, padrão 3s por
    tentativa) até `HOTSPOT_MAX_RESTART_ATTEMPTS` (padrão 5) — a nova
    tentativa começa direto pelo último canal/banda que funcionou
    nesta execução (evita repetir uma varredura completa, incluindo
    bandas travadas por regulamentação de firmware que nunca vão
    funcionar). `cleanup()` usa `timeout` ao chamar `create_ap --stop`
    para nunca travar indefinidamente esperando uma instância presa
    num loop de erro do driver.
  - **Camada 2 (backend, rede de segurança, ciclo de 15s)**: se as
    tentativas locais do worker se esgotarem (ou o processo cair por
    qualquer outro motivo, incluindo o container/host reiniciar),
    `reconcileHotspotOnce` detecta `"running": false"` e, se a última
    intenção do admin foi ligar (`POST /api/hotspot/start`, nunca
    desfeita por um `POST /api/hotspot/stop`), religa sozinho pelo
    mesmo caminho de `autoStartHotspotOnBoot`. Uma parada deliberada
    pelo painel nunca é desfeita por nenhuma das duas camadas.

## Ligar/desligar/recuperar o hotspot pelo painel (`POST /api/hotspot/start` / `/stop` / `/recover-wifi`)

Não existem mais scripts de shell separados (`scripts/hotspot-on.sh` /
`hotspot-off.sh`, removidos) — ligar/desligar o hotspot é feito
inteiramente pelo painel de gestão, via `services/backend/hotspot.go`
delegando ao `services/worker/controller` (único serviço com acesso a
NetworkManager/`docker.sock`). O comportamento é o mesmo que os
scripts tinham, só que dentro do container privilegiado em vez de
`sudo` no host:

- `POST /api/hotspot/start`:
  1. Sobe `hotspot` + `dns-provider` via `docker compose up -d
     --no-build --no-deps` usando diretamente
     `docker-compose.services.yml` (`worker`: `POST /hotspot/apply`) —
     cria os containers se ainda não existirem (1ª subida) e também os
     recria se o `.env` mudou, não apenas `docker start`, sem acionar o
     job `migration` nem o `docker-compose.yml` agregador.
  2. O container `hotspot` deixa a `WIFI_INTERFACE` física sob controle
     do NetworkManager e delega ao `create_ap` a criação da interface
     AP virtual (`ap0`) quando o adaptador suporta AP+STA. A fonte de
     internet entregue ao `create_ap` é sempre o uplink virtual
     `BINDNET_UPLINK_INTERFACE`, alimentado por regras Bindnet de
     NAT/forward a partir da interface real configurada.
- `POST /api/hotspot/stop` desfaz exatamente o inverso, na ordem
  inversa:
  1. Para `hotspot` + `dns-provider` (`docker stop`, via `worker`).
  2. Garante que qualquer drop-in antigo do NetworkManager seja removido
     e devolve `WIFI_INTERFACE` ao controle do NetworkManager
     (`worker`: `POST /network/wifi-manage`, que roda `nmcli device set
     ... managed yes`). O container também remove `bn-uplink` e as
     chains `BINDNET-HOTSPOT` ao sair.
- `POST /api/hotspot/recover-wifi` é a ação operacional do botão
  "Recuperar Wi-Fi" na tela "Hotspot Wi-Fi": repete a etapa segura de
  parada e devolução da placa ao NetworkManager quando a interface ficou
  presa como não gerenciada após queda/restart/interrupção fora do fluxo
  normal.
- `WIFI_INTERFACE` vem da seção `hotspot` do `.env` (via `worker`); se
  não estiver definida, a requisição falha explicitamente (sem
  fallback silencioso para uma interface adivinhada).
- Essas operações mexem em configuração real do host (NetworkManager,
  placa Wi-Fi física, `iptables` e interface dummy `bn-uplink`). O
  fluxo suportado é iniciar/parar pelo painel; "Recuperar Wi-Fi" fica
  como ação segura para limpar estados antigos ou interrupções fora do
  fluxo normal.
- `POST /api/hotspot/start`/`stop` também gravam a última intenção do
  admin (chave interna `_DESIRED_STATE` em `hotspot_config`, fora da
  allowlist de `GET`/`PATCH /api/hotspot/config` — ver
  `hotspotDesiredStateKey` em `services/backend/hotspot_config_store.go`).
  Ao subir, o backend chama `autoStartHotspotOnBoot`
  (`services/backend/hotspot_autostart.go`, goroutine iniciada em
  `main.go`): se a última intenção foi "ligado" e o hotspot não está
  rodando, ele religa sozinho (com retry curto, já que o
  worker/container do hotspot podem demorar a ficar prontos logo após
  o backend subir) — sem isso, o container do hotspot sempre volta em
  modo "manager" (ocioso) após qualquer restart do container/reboot da
  máquina, exigindo clique manual do admin mesmo que o hotspot
  estivesse ligado antes.

## Clientes do hotspot: identificação e bloqueio por MAC (`services/backend/hotspot_devices.go`)

- `GET /api/hotspot/clients` continua vindo de `create_ap --list-clients`
  (via `worker`), mas agora cada cliente é enriquecido com dados
  cacheados na tabela `hotspot_device_info` (fabricante/tipo/SO
  aproximados) e uma flag `blocked` cruzada com `hotspot_blocked_devices`.
  O enriquecimento nunca é automático a cada poll (a tela reconsulta a
  cada 5s) — só acontece sob demanda.
- `POST /api/hotspot/clients/{mac}/identify` dispara a identificação:
  1. Busca o fingerprint DHCP do dispositivo (opções pedidas + vendor
     class) via `worker`: `GET /hotspot/fingerprint` — lê
     `/tmp/bindnet-dnsmasq-dhcp.log`, caminho fixo que
     `services/worker/hotspot/patch-create-ap.sh` força no
     `dnsmasq.conf` gerado pelo `create_ap` (`log-dhcp` +
     `log-facility`), já que o `CONFDIR` original tem sufixo aleatório
     por execução.
  2. Descobre o fabricante via `api.macvendors.com` (MAC → nome do
     fabricante); se a chamada externa falhar ou não tiver rede, cai
     para uma base OUI local (`BINDNET_OUI_DB_PATH`, opcional) e por
     fim uma tabela mínima embutida no binário — nunca quebra a tela
     por falta de internet.
  3. Estima tipo de dispositivo/SO por heurística local (substring em
     hostname/fabricante/vendor class DHCP - ver
     `inferHotspotDeviceProfile`) — é uma aproximação com nível de
     confiança (`confidence`), não uma identificação garantida.
  4. Resultado fica em cache (`hotspot_device_info`, por MAC) até o
     operador pedir de novo.
- `PATCH /api/hotspot/devices/{mac}/identity`
  (`services/backend/hotspot_device_identity.go`) grava edição manual
  de `alias`/`vendor`/`deviceName`/`osName` na mesma tabela
  `hotspot_device_info` — campo ausente no corpo do PATCH preserva o
  valor atual (permite editar só o alias na aba de visão geral, ou os
  quatro campos juntos no modal "Identificar" da lista de clientes,
  sem um sobrescrever o outro). Editar manualmente marca
  `confidence = 100` (sinaliza "definido a mão", distinto da heurística
  de `POST .../identify`). `alias` é único (`UNIQUE` em
  `hotspot_device_info.alias`, nulo permitido e não contável para a
  constraint) — conflito devolve 409.
- `GET/POST /api/hotspot/blocklist` e `DELETE
  /api/hotspot/blocklist/{mac}` gerenciam a tabela
  `hotspot_blocked_devices`. Bloquear/desbloquear tem efeito imediato
  via `hostapd_cli deny_acl ADD_MAC`/`DEL_MAC` (+ `deauthenticate` no
  bloqueio) no `worker` (`services/worker/controller/hotspot_acl.go`)
  — não precisa reiniciar o hotspot. Essa ACL só existe na memória do
  `hostapd`: some a cada restart do container, por isso `POST
  /api/hotspot/start` e `POST /api/hotspot/apply` sempre reaplicam a
  blocklist inteira depois de subir o hotspot (com retry curto,
  já que o `hostapd` leva alguns segundos para ficar pronto).

## Perfis de dispositivo, vouchers de recarga e portal cativo (`services/backend/hotspot_profiles*.go`, `hotspot_vouchers.go`, `hotspot_portal.go`)

- **Perfil** (`hotspot_profiles`) é um bundle nomeado e reutilizável de
  limites de tráfego (mesmo shape de `hotspot_device_limits`) + política
  de recarga de crédito (subconjunto de `hotspot_device_credit`: apenas
  `rechargeAmountBytes`/`rechargePeriod`/`plafondBytes` — nunca
  saldo/estado). Existe um perfil "Padrão" com id fixo
  (`00000000-0000-0000-0000-000000000001`, protegido contra remoção),
  vinculado por padrão a todo dispositivo (`hotspot_device_info.profile_id`
  tem esse valor como `DEFAULT`, aplicado inclusive a linhas já
  existentes quando a coluna foi criada).
- **Tipo de limitação (`limitType`) é único e mutuamente exclusivo**,
  tanto em perfil quanto em override de dispositivo (`hotspot_device_limits.limit_type`):
  `unlimited` (nenhum teto), `credit` (precisa de saldo, política em
  `hotspot_device_credit`), `quota` (até 3 tetos simultâneos e
  independentes — diário/semanal/mensal, cada um com seu acumulador em
  `hotspot_device_quota_periods` e bloqueio rígido ao estourar) ou
  `custom` (só válido em **perfil** — ver abaixo). Taxa
  (download/upload, Mbps) é sempre independente do tipo, configurável
  nos 4 casos. Esse tipo único existe para impedir cota e crédito
  ficarem ativos ao mesmo tempo no mesmo perfil/dispositivo (causa
  histórica de bloqueio por crédito disfarçado de "cota parou de
  contabilizar").
- **`limitType = "custom"` delega a decisão para o dispositivo**: um
  perfil "customizado" não aplica limite nenhum por si só — o
  dispositivo vinculado a ele é quem escolhe sua própria estratégia
  (`unlimited`/`credit`/`quota`, nunca `custom` — um dispositivo é
  sempre o último nível, não há para quem delegar de novo) via
  `PATCH /api/hotspot/devices/{mac}/limits`. `PATCH` nessa rota é
  **rejeitado com 409** se o perfil vinculado não for `custom` no
  momento da chamada.
- **Ordem de resolução por dispositivo é sempre perfil, a menos que o
  perfil seja `custom`** (nunca override > perfil como um fallback
  genérico, e nunca cai para `hotspot_global_limits` — o limite global
  já é uma camada HTB separada e sempre ativa):
  - `effectiveDeviceLimits` (`hotspot_profiles_apply.go`): resolve o
    perfil vinculado ao MAC; se `limitType != "custom"`, os valores do
    perfil valem inteiros (um override antigo em `hotspot_device_limits`
    fica dormente/ignorado enquanto isso). Só quando o perfil é
    `custom` é que a linha do dispositivo em `hotspot_device_limits`
    passa a valer — na ausência dela, o padrão é `unlimited`.
  - `syncDeviceCreditFromProfile`: decide se crédito está ativo sempre
    pelo `LimitType` **efetivo** (`effectiveDeviceLimits`, já
    resolvido), nunca pelo `limitType` cru do perfil — um dispositivo
    com override `credit` sob perfil `custom` também conta como
    crédito ativo. A política de recarga (`rechargeAmountBytes`/
    `rechargePeriod`/`plafondBytes`), porém, só é herdada do perfil
    quando é o próprio perfil (não `custom`) que dá origem ao crédito;
    sob perfil `custom` o dispositivo configura sua própria política
    via `PATCH .../credit`. Em qualquer caso, só age quando
    `hotspot_device_credit.configured = false` — vira `true` assim que
    o admin configura crédito manualmente ou o dispositivo resgata um
    voucher (que também força `limitType = "credit"` no override do
    dispositivo, já que resgate é auto-serviço sem acesso à aba de
    limites) — a partir daí o perfil para de influenciar aquele MAC.
  - Editar um perfil (`PATCH /api/hotspot/profiles/{id}`) reaplica ao
    vivo (`applyProfileShapingLive`) em todo dispositivo conectado
    vinculado a ele, sem pular quem tem override próprio — desde que
    `effectiveDeviceLimits` já decide sozinho se esse override importa
    (só quando o perfil é `custom`).
- **Vouchers** (`hotspot_vouchers`) são códigos de recarga
  (`XXXX-XXXX-XXXX`, `crypto/rand`) com valor fixo em bytes, emitidos em
  lote pelo admin (`POST /api/hotspot/vouchers`, até 100 por vez) e
  resgatáveis uma única vez. O resgate (`redeemVoucher`,
  `hotspot_vouchers.go`) é uma transação que reivindica o código
  (`UPDATE ... WHERE status='active'`, atômico via lock de linha do
  Postgres) e credita o saldo exatamente como uma recarga manual —
  ganha um novo `entry_type` (`voucher_redemption`) no extrato
  (`hotspot_device_credit_history`). Código inexistente e código já
  usado devolvem o mesmo erro genérico de propósito (evita virar um
  oráculo para quem tenta códigos ao acaso).
- **Portal de autoatendimento** (`GET/POST /api/hotspot/portal/*`,
  `hotspot_portal.go`) são as únicas rotas do hotspot sem
  `requireSession` — mesmo precedente de `GET /api/mesh/ca`. O MAC do
  dispositivo chamador **nunca** vem do corpo/query: é sempre resolvido
  no servidor a partir do IP de origem (`X-Forwarded-For` ou
  `RemoteAddr`), cruzado contra `GET /hotspot/clients` (mesma função
  `liveHotspotClients` já usada pelo resto do hotspot). Falha em
  resolver o MAC devolve 409 — nunca aceita um MAC alternativo vindo do
  cliente. A página (`/portal` no frontend, fora de `RequireAuth`) é
  servida pelo mesmo SPA já publicado, sem porta/serviço novo.
- **Redirecionamento automático (portal cativo)** não usa DNS: a
  sondagem de conectividade que o próprio SO do dispositivo já dispara
  ao entrar numa rede Wi-Fi (HTTP simples, nunca HTTPS) é interceptada
  por uma regra `iptables -t nat -I PREROUTING ... REDIRECT` por MAC
  (`services/worker/controller/captive_portal.go`), avaliada antes do
  `filter/FORWARD` onde vive o DROP de `traffic_block.go` — o
  dispositivo continua associado ao Wi-Fi, só a porta 80 é desviada
  para um responder HTTP mínimo que sempre devolve um redirect para
  `/portal`. Só é ativado quando o dispositivo é bloqueado **por falta
  de crédito** (`blocked_by_credit`) — nunca pelo bloqueio manual do
  admin (blocklist modo "traffic"), que continua sem portal cativo de
  propósito. HTTPS não é interceptado (limitação universal de qualquer
  portal cativo).

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
- `DOMAINS` define as zonas que participam do **discover mode**. Valores
  com um label (ex.: `bnet`, `dev`, `discover`) são raízes amplas da
  malha: nomes dentro delas só resolvem se forem locais via
  `server_name`/rota própria ou aprendidos de outro Bindnet. Valores
  com mais de um label (ex.: `costa.bnet`, `*.costa.bnet`) são zonas
  concretas deste nó: a zona inteira é anunciada como local e
  subdomínios como `app.costa.bnet` resolvem localmente. `DOMAINS`
  vazio desliga o discover mode.
- `server_name` declarados no nginx-ui são tratados como anúncios de
  serviços locais deste nó. O `dns-provider` descobre esses nomes a
  partir do volume `nginx_config` montado somente leitura. Nomes exatos
  (ex.: `app.costa.dev`) anunciam só aquele host; wildcards (ex.:
  `*.costa.dev`) anunciam uma zona inteira. Entradas `_`, regex e
  variáveis do Nginx são ignoradas.
- Quando a consulta é para um nome de `DOMAINS` que este nó possui
  localmente (por zona concreta em `DOMAINS`, por `server_name` ou por
  outra fonte local equivalente),
  a resposta segue a mesma resolução local split-horizon dos TLDs de
  `DNS_LOCAL_TLDS`: host recebe loopback persistente, containers
  recebem o gateway Docker em que a consulta chegou e clientes do
  hotspot recebem `HOTSPOT_GATEWAY`.
- Quando a consulta é para um nome de `DOMAINS` pertencente a outro
  servidor descoberto, o Bindnet funciona como um **roteador de
  descoberta**: o nó local responde encaminhando (proxy real da
  consulta, não uma resposta fixa) para o próximo salto conhecido, não
  necessariamente para o servidor final. Exemplo: se a topologia é
  `A <-> B <-> C <-> D`, com `DOMAINS=a.dev`, `DOMAINS=b.dev`,
  `DOMAINS=c.dev` e `DOMAINS=d.dev`, então:
  - no servidor A, uma consulta para `b.dev` é encaminhada para B,
    porque B é o dono direto desse domínio;
  - no servidor A, consultas para `c.dev` ou `d.dev` também são
    encaminhadas para B, porque B é o próximo salto conhecido para
    alcançar C ou D;
  - no servidor B, `c.dev` segue para C e `d.dev` também segue para C,
    que então entrega a D.
- **Como os nós se conhecem**: não existe mais entrada automática por
  multicast/broadcast. O operador precisa clicar em "Fazer busca" no
  painel para varrer a LAN naquele momento, selecionar o peer
  encontrado e salvar. A busca manual grava apenas candidatos em
  `discover_peers`; a malha efetiva continua sendo exclusivamente os
  peers diretos salvos no Postgres (`discover_configured_peers`).
- Para peers fora da LAN, o operador adiciona manualmente endereços
  `host:porta` do `DISCOVER_PORT`.
- A troca de rotas acontece só com os peers efetivamente presentes em
  `discover_configured_peers`: o `dns-provider` sobe endpoints HTTP
  próprios (`GET /discover/info` e `GET /discover/routes`, porta
  `DISCOVER_PORT`, padrão `8531`) e uma goroutine que consulta cada peer
  a cada 15 segundos, implementando um vetor de distância simples
  (estilo RIP):
  - cada nó anuncia, com distância 0, suas zonas concretas de
    `DOMAINS` e os nomes que ele mesmo possui localmente
    (`server_name` do nginx-ui) que também caem dentro de algum
    `DOMAINS` seu (chamado de "dono": `DISCOVER_NODE_NAME`, padrão o
    hostname do container);
  - ao aprender uma rota de um peer, a distância local é
    `distância_anunciada + 1`; rotas com distância acima de 16 saltos
    são descartadas (limite de segurança contra contagem-ao-infinito);
  - `DISCOVER_REMOTE_ROUTES=auto` permite aprender vizinhos remotos
    anunciados por um peer direto; `DISCOVER_REMOTE_ROUTES=manual`
    aceita apenas rotas próprias do peer direto (`distância anunciada =
    0`), exigindo que vizinhos remotos sejam adicionados explicitamente
    pelo painel;
  - o endpoint de descoberta nunca devolve a um peer uma rota que só
    existe porque foi aprendida dele mesmo (split-horizon, baseado no
    IP de origem da requisição HTTP) e um nó nunca aprende de volta uma
    rota para um domínio que ele mesmo possui;
  - uma rota existente só é substituída se vier do mesmo peer (refresh)
    ou se a nova distância for menor; se um peer parar de responder,
    as rotas aprendidas dele viram `state = "stale"` depois de ~3
    ciclos sem confirmação, mas nunca são apagadas automaticamente —
    ficam visíveis no painel até o peer voltar ou um operador remover
    manualmente.
  - a tabela (domínio anunciado, dono, próximo salto, distância,
    origem, estado, última vez vista) é persistida no Postgres
    (`discover_routes`) só para sobreviver a reinicializações e para o
    painel ler — a resolução de DNS em si nunca consulta o Postgres por
    consulta, só um snapshot em memória atualizado a cada ciclo. O
    painel (`GET /api/dns/routes`) apresenta essa tabela e permite
    remover manualmente uma rota parada (`DELETE
    /api/dns/routes/{domínio}`).
- Nomes dentro de `DOMAINS` que não forem locais nem conhecidos pela
  tabela de descoberta respondem **NXDOMAIN** — nunca são encaminhados
  ao DNS público (evitaria vazar um namespace que é interno da malha)
  nem viram silenciosamente o IP desta máquina.
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
  - gateways Docker detectados automaticamente (**view container**):
    consultas vindas de containers Docker (que só alcançam o host por
    esses gateways). Registros `A` para os TLDs locais sempre respondem
    com o próprio gateway Docker em que a consulta chegou.
  - IPs declarados em `HOST_SOURCE_CIDR` (**view peer/LAN**):
    consultas encaminhadas por outros servidores Bindnet chegam pelo IP
    físico/LAN deste nó. Registros `A` para zonas locais respondem com
    esse próprio IP, para que o cliente remoto consiga alcançar o dono
    final do domínio.
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
- O processo **detecta** os gateways das bridges Docker existentes no
  host e lê `HOST_SOURCE_CIDR` (ou auto-detecta os IPs de LAN quando
  vazio), mas isso nunca bloqueia nem derruba o processo: o servidor
  HTTP de descoberta (`GET /discover/info`/`GET /discover/routes`,
  `DISCOVER_PORT`) e o poll de peers sobem incondicionalmente assim que
  Postgres/Redis/fingerprint estiverem prontos, sem depender de nenhuma
  interface de rede do host. Cada socket DNS UDP:53 que depende de um IP
  específico (gateway Docker, `HOST_SOURCE_CIDR`, `HOTSPOT_GATEWAY`) sobe
  em loop próprio e não-fatal: se o IP ainda não existir (até
  `COREDNS_WAIT_TIMEOUT`, padrão 90s por tentativa), só loga um aviso e
  tenta de novo a cada poucos segundos — nunca derruba o processo nem os
  demais sockets. Só o socket de `127.0.0.1:53` é síncrono/fatal (loopback
  sempre deveria existir; se falhar, é um problema diferente e
  genuinamente fatal). Isso evita que um `HOST_SOURCE_CIDR` desatualizado
  ou um gateway Docker ausente torne o nó inteiro "invisível" para busca
  de peers — antes, essa mesma espera derrubava o processo inteiro
  (`log.Fatalf`) antes do servidor de descoberta sequer subir.
- `dns-provider` não tem (nem deveria ter) `depends_on: hotspot` no
  `docker-compose.services.yml`: ele já sobe/funciona de forma
  independente do container `hotspot` existir ou estar rodando (ver
  espera assíncrona/não-fatal descrita acima) — a única leitura
  relacionada a hotspot é `HOTSPOT_GATEWAY` no Postgres (config
  estática, com fallback), nunca uma chamada ao container `hotspot`
  em si. `depends_on: hotspot: condition: service_started` (sem
  healthcheck) era só ordenação de criação de container no Compose,
  sem efeito funcional real, e foi removido para não sugerir um
  acoplamento que não existe no código.
- `dns-provider` roda com `network_mode: host` (precisa bindar IPs
  reais do host) e por isso **não enxerga a DNS interna do Docker**
  para resolver `postgres`/`redis` pelo nome do serviço — fala com
  eles pelos IPs fixos atribuídos na rede `proxy`
  (`POSTGRES_HOST`/`REDIS_HOST` apontam para esses IPs fixos no
  `docker-compose.services.yml`, não para os nomes dos serviços).
- Para que **qualquer container de qualquer projeto** resolva os TLDs
  locais pela view container, o Docker daemon do host deve usar o
  gateway Docker do Bindnet como DNS upstream. O caminho versionado é
  `sudo scripts/configure-docker-dns.sh`, que detecta esse gateway via
  `docker network inspect` e grava `"dns": ["<gateway>"]` em
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
- `POST /api/certificates` emite um certificado *leaf* (RSA 2048) a
  partir de `domains` (lista de domínios/IPs) — todos os itens viram
  SAN (Subject Alternative Name) de um único certificado; o primeiro
  item normalizado é o domínio/CN primário, salvo na coluna `domain` e
  usado como nome de referência nas listagens e na importação para o
  `nginx-ui`. Um item pode ser um domínio curinga (`*.mydomain`) —
  cobre o caso de emitir um certificado para `*.mydomain` junto com
  `app.mydomain`, `app2.mydomain` etc. em uma única emissão. Sempre
  cria uma linha nova em `certificates` — sem cache/reuso por domínio,
  já que emitir é agora uma ação explícita do usuário, não um lookup
  implícito por SNI. `validityQuantity` (inteiro) e `validityUnit`
  (`days`|`weeks`|`months`|`years`) definem `NotAfter`; ausentes ou
  inválidos caem para o padrão fixo anterior (2 anos), e o resultado
  nunca ultrapassa a validade da própria CA local.
- Após persistir no Postgres, o backend importa o certificado para o
  `nginx-ui`, para que ele apareça em
  `/#/certificates/list?search={}`. Se `NGINX_UI_USERNAME` e
  `NGINX_UI_PASSWORD` estiverem preenchidos, usa a API `/api/certs`;
  caso contrário, grava os PEMs em `/etc/nginx/ssl/<domínio primário>/`
  e registra a linha correspondente (com todos os SANs em `domains`) no
  `database.db` do `nginx-ui` via os volumes compartilhados.
- Cada domínio/IP é normalizado antes de emitir: minúsculas, sem
  porta, sem `.` final; o primeiro rótulo pode ser `*` (curinga); se
  algum rótulo restante não passar numa validação básica de caracteres
  (`[a-z0-9-]`), aquela entrada cai para `localhost.local` em vez de
  emitir certificado para um valor não sanitizado — mesma regra do
  antigo `cert-proxy`, agora aplicada item a item da lista.
- `DELETE /api/certificates/{id}` revoga: seta `revoked_at`, **nunca
  deleta a linha** — o certificado revogado continua aparecendo na
  listagem dedicada de revogados do Bindnet, com status "revogado", e
  sai da lista principal de certificados emitidos. A mesma chamada
  remove o certificado da lista do `nginx-ui` e limpa os PEMs importados
  em `/etc/nginx/ssl/<domínio>/`.
- `DELETE /api/certificates/{id}/permanent` elimina definitivamente
  uma linha **somente se ela já estiver revogada**; certificados ativos
  precisam passar primeiro pela revogação para sair do `nginx-ui`.
- `GET /api/certificates/ca` e `GET /api/certificates/{id}/download`
  servem os PEMs para download e exigem sessão autenticada, igual às
  demais rotas de `/api/certificates/*`. **Exceção deliberada**:
  `GET /api/mesh/ca` devolve o mesmo PEM da CA (só o certificado
  público, nunca a chave privada) **sem sessão**, com
  `Access-Control-Allow-Origin: *` — usada pela tela "Servidores
  Bindnet" do painel (`services/frontend/src/components/bindnets/`)
  para buscar a CA de outros nós da malha direto do navegador, sem
  autenticação entre backends. É seguro porque uma CA raiz é feita
  para ser distribuída publicamente (mesmo papel do antigo
  `cert-proxy`, que servia `/ca.crt` anonimamente na porta 80 — só que
  agora escopado só a essa rota, em vez de todo o cert-proxy).
- `POST /api/certificates/ca/install-local` instala uma CA no próprio
  host Linux onde o stack está rodando. Por padrão usa a CA deste
  backend; aceita um corpo opcional `{"certificatePem": "..."}` para
  instalar a CA de **outro** servidor Bindnet (buscada via
  `GET /api/mesh/ca` dele) — usado pelo card de CA de nós remotos em
  "Servidores Bindnet". Em qualquer um dos casos o backend só repassa
  o PEM para o `worker`; o `worker` valida que é uma CA, grava
  `/usr/local/share/ca-certificates/bindnet-local-ca.crt` e executa a
  ação fixa `update-ca-certificates` (mais a importação nas stores
  NSS do Chrome/Chromium e Firefox de cada usuário do host, ver
  `services/worker/controller/browser_trust.go`). Essa rota não aceita
  caminho nem comando vindo do frontend/backend, só o conteúdo do
  certificado.
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
  `www_data`. O backend também monta `nginx_config` e `nginx_ui_data`
  com escrita para importar os certificados emitidos no painel Bindnet
  para a lista do `nginx-ui`. Esses volumes **não** são gerenciados pelo
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
- **Mongo guarda a trilha de auditoria do painel** (coleção
  `audit_log`) — eventos discretos de ações administrativas: `login`,
  `logout`, `config_changed` (hotspot/DNS), `certificate_issued`,
  `certificate_revoked`, `hotspot_started`, `hotspot_stopped`,
  `dns_applied`. **Não** persiste logs de containers — isso continua
  sendo só streaming ao vivo via `worker` (`LogsPanel.tsx`,
  `worker.streamLogs`). Falha ao gravar auditoria nunca derruba a ação
  principal do usuário (só loga o erro). Mongo também guarda o **trace
  bruto de consumo do hotspot** (coleção `hotspot_credit_debits`,
  `services/backend/hotspot_credit_trace.go`) — uma linha por ciclo de
  reconciliação (a cada 15s) com o tráfego do dispositivo naquele
  ciclo, gravada pra **todo dispositivo conectado**, com ou sem crédito
  habilitado (só quando habilitado é que o mesmo valor também desconta
  `hotspot_device_credit.balance_bytes`, ver `reconcileDeviceCredit`)
  — é esse trace que alimenta o detalhe de consumo ao clicar numa
  sessão (`GET .../sessions/{id}/consumption`). É o item de maior
  volume da conta corrente, por isso não mora no Postgres: expira
  sozinho via índice TTL em `createdAt`, com a retenção controlada por
  `HOTSPOT_CREDIT_TRACE_RETENTION_DAYS` (padrão 180 dias/6 meses).
- **Postgres consolida o consumo por sessão de conexão**
  (`hotspot_device_sessions`, `services/backend/hotspot_sessions.go`)
  em vez de guardar um trace por ciclo de reconciliação: uma sessão
  nasce quando o MAC aparece na lista de clientes ao vivo do hotspot e
  fecha (`ended_at`) quando some dela (ou quando o hotspot para). A
  cada ciclo de reconciliação, o tráfego real do dispositivo
  (`deltaDown+deltaUp`, o mesmo delta de `recordDeviceUsage`)
  incrementa `total_bytes` da sessão aberta daquele MAC — **sempre**,
  independente de o dispositivo ter crédito habilitado (débito de
  saldo é uma consequência separada, só para quem tem crédito, ver
  `reconcileDeviceCredit`). Isso mantém o total consolidado disponível
  mesmo depois do TTL apagar o trace bruto de débito de crédito no
  Mongo, servindo de conferência posterior.
- **A conta corrente de crédito é uma única lista, toda em Postgres**
  (`GET /api/hotspot/devices/{mac}/credit/history`,
  `services/backend/hotspot_credit_history.go`; aba "Movimentações" no
  detalhe do dispositivo): mescla recarga manual/automática/resgate de
  voucher (`hotspot_device_credit_history`, sem TTL — eventos raros,
  ligados a ação humana ou dinheiro) com **toda sessão** de conexão
  (`hotspot_device_sessions`, ativa ou encerrada, com ou sem consumo
  ainda) como linha de débito `session_active`/`session_closed`. Essa
  lista nunca consulta o Mongo diretamente — só ao clicar numa linha de
  sessão específica é que `GET .../sessions/{id}/consumption` busca lá
  o trace bruto daquela janela de tempo (`started_at`–`ended_at`), que
  pode já ter expirado pelo TTL. A UI permite filtrar por tipo (crédito/
  débito ou o `entryType` específico).
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
  `/usr/local/share/ca-certificates`, `/etc/ssl/certs`,
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
  tocar `DNS_LOCAL_TLDS`, `DOMAINS`, `DISCOVER_REMOTE_ROUTES`,
  `DISCOVER_NODE_NAME` e `DISCOVER_PORT`. O editor preserva comentários,
  ordem e chaves não mencionadas — nunca regenera o arquivo do zero.
- "Salvar" configuração (grava no `.env`) e "aplicar" (recria o
  container via `docker compose up -d --no-build`) são ações
  separadas — o painel nunca reinicia o hotspot/DNS sozinho ao salvar,
  só quando o usuário confirma explicitamente "aplicar". Exceção:
  peers diretos da malha Bindnet são estado operacional e ficam no
  Postgres (`discover_configured_peers`), não no `.env`.
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
  `.env.example` (com placeholders) é template público. O fluxo
  operacional versiona/usa `.env.main` como arquivo principal do
  `promote.yml/current_stage`; `.env` real continua reservado para
  fallback local/legado e é ignorado (`.gitignore`).
- O entrypoint operacional é `bin/promote` (submódulo `docker-cli`) via
  `Makefile`. A topologia Compose deve ficar dividida em
  `docker-compose.infra.yml`, `docker-compose.services.yml`,
  `docker-compose.assets.build.yml`, `docker-compose.deploy.yml` e o
  agregador `docker-compose.yml`. Portas publicadas devem ficar nos
  compose de infra/services, não em overlays `ports`.
- Todo `entrypoint.sh`/script novo deve seguir `set -euo pipefail` (ou
  equivalente em `sh`), falhar alto (erro explícito + `exit 1`) em vez
  de continuar em estado inconsistente, e logar em português com
  prefixo `[nome-do-servico]`, no padrão já usado pelos scripts
  existentes.
- Serviços que dependem de estado de rede do host (`hotspot`,
  `dns-provider`, `worker`) usam `network_mode: host`; os demais
  (`nginx-ui`, `cert-proxy`, `backend`, `frontend`) ficam isolados na
  rede Docker `proxy`.
