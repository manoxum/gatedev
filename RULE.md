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
  `WIFI_SSID`. Ausência de qualquer uma delas é erro fatal (o container
  não sobe). `WIFI_PASSWORD` é obrigatória só quando `WIFI_OPEN` não é
  `true` (ver item abaixo).
- **`WIFI_OPEN`** (padrão `false`) cria um hotspot livre, sem
  autenticação nenhuma, quando `true`. O `create_ap` baixado
  (`oblique/create_ap`) só escreve as diretivas `wpa_*` no
  `hostapd.conf` quando recebe uma passphrase como último argumento
  posicional; sem ela, o AP sobe aberto. Por isso o hotspot **omite o
  argumento inteiro** (não passa uma string vazia) quando `WIFI_OPEN=true`,
  em vez de reaproveitar `WIFI_PASSWORD` de uma configuração anterior
  com senha. `WIFI_PASSWORD` continua podendo ficar salva no painel
  nesse estado (não é apagada ao ligar o modo livre) - só deixa de ser
  obrigatória e de ser repassada ao `create_ap`, para que voltar
  `WIFI_OPEN` para `false` não exija digitar a senha de novo. A UI
  (`services/frontend/src/components/hotspot/HotspotWifiTab.tsx`)
  desabilita o campo de senha enquanto o modo livre está marcado, e o QR
  de conexão (`HotspotWifiQr.tsx`) usa o formato `WIFI:T:nopass;...`
  (sem campo de senha) em vez de `WIFI:T:WPA;...`.
- `WIFI_CHANNE` (sem "L") é aceito como alias legado de
  `WIFI_CHANNEL`, com aviso de depreciação no log. Não usar em
  configurações novas.
- **`ensure_wifi_radio_unblocked`** (`regulatory.sh`) roda uma vez no
  início, antes de qualquer diagnóstico regulatório ou tentativa de
  canal: confere via `rfkill list wifi` se o rádio Wi-Fi está
  bloqueado (soft ou hard) e, se estiver, tenta `rfkill unblock wifi`
  automaticamente. Um rádio bloqueado rejeita **todos** os
  canais/bandas igualmente com erros genéricos do driver (ex.:
  `RTNETLINK answers: No error information`), indistinguível de fora
  de uma trava regulatória ou de canal específico — sem essa checagem,
  o hotspot esgotaria todos os candidatos de 2.4GHz e 5GHz (e as 5
  tentativas do loop de retry) sem nunca revelar a causa real. Um
  `docker compose up --build` que recria o container `hotspot` com o
  `create_ap` ativo (interrompendo a limpeza normal no meio — ver
  `stop_grace_period` do serviço `hotspot` no
  `docker-compose.services.yml`, aumentado pra 30s de propósito, acima
  do pior caso de `force_stop_create_ap`) já deixou o driver `iwlwifi`
  reportando bloqueio "hard" via `rfkill` numa sessão real, e um
  `rfkill unblock` comum (sem privilégio de root) resolveu de verdade
  — confirma que não era um interruptor físico genuíno (esse
  continuaria bloqueado logo em seguida). Só loga e tenta desbloquear;
  nunca falha o script por si só, já que o `create_ap` reporta o erro
  de qualquer forma se o desbloqueio não funcionar (ex.: bloqueio
  físico genuíno de verdade, que nenhum comando resolve).
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
  - **Histórico persistente** (`hotspot_channel_history` no Postgres,
    `services/worker/hotspot/history.sh`) influencia a ordem dos
    candidatos muito mais que a pontuação de interferência ao vivo:
    cada tentativa de `create_ap` num banda/canal grava sucesso (o log
    mostrou `AP-ENABLED`, mesmo que o AP caia depois por outro motivo)
    ou falha, por combinação (`WIFI_INTERFACE`, modo, banda, canal).
    `rank_channels_for_band` soma `(falhas − sucessos) × 1000` à
    pontuação de interferência de cada candidato antes de ordenar —
    um canal que já falhou consistentemente (ex.: "adapter can not
    transmit") vai sempre pro fim da fila nas próximas subidas, mesmo
    que a varredura ao vivo o mostre como o menos congestionado agora;
    interferência só desempata candidatos com o mesmo histórico
    (incluindo nenhum ainda). "Modo" distingue `same-interface`
    (Wi-Fi para Wi-Fi — histórico é só diagnóstico ali, já que o canal
    é sempre travado pelo canal da estação, sem escolha real) de
    `different-interface` (Ethernet para Wi-Fi/auto — histórico
    realmente influencia a ordem). `resolve_wifi_band` usa a mesma
    lógica num nível acima: se uma banda inteira tem histórico de
    sucesso melhor que a banda preferida por capacidade de hardware
    (5GHz), a automática escolhe a banda historicamente melhor direto,
    evitando reaprender do zero (testando todos os candidatos de 5GHz
    de novo) uma trava regulatória de firmware já conhecida. Histórico
    nunca é uma dependência funcional — qualquer falha ao ler/gravar
    no Postgres é ignorada em silêncio (só perde a otimização de
    velocidade, não trava o hotspot).
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
  ranqueia as candidatas por maior velocidade reportada em
  `/sys/class/net/<iface>/speed` (métrica da rota desempata; Wi-Fi não
  reporta velocidade e fica atrás de qualquer Ethernet de propósito);
  sem rota padrão, entram interfaces com link físico ativo (carrier)
  como último recurso; se ainda assim não houver candidata, falha
  explicitamente — mesma filosofia de nunca adivinhar silenciosamente
  já usada para canal/banda. A **própria placa Wi-Fi do hotspot entra
  como candidata do `auto`** somente quando está associada como
  estação agora (`sta_link_probe`, que rejeita a placa em modo AP) —
  é o único fallback possível numa máquina com uma única porta
  Ethernet; em `--no-virt` ela nunca é candidata. A interface real
  escolhida **não é passada diretamente ao `create_ap`**: o hotspot
  cria uma interface dummy estável (`BINDNET_UPLINK_INTERFACE`, padrão
  `bn-uplink`), instala regras próprias em chains `BINDNET-HOTSPOT` de
  `FORWARD`/`POSTROUTING` **e roteamento por política** (tabela
  dedicada `91` + `ip rule` para o `HOTSPOT_CIDR`, ver `uplink.sh`):
  o `MASQUERADE -o <iface>` sozinho não muda por onde o tráfego sai —
  com duas rotas default simultâneas (Ethernet + Wi-Fi) o kernel
  continuaria usando a de menor métrica, ignorando a fonte escolhida;
  a tabela dedicada garante que o tráfego dos clientes do hotspot sai
  sempre pela fonte selecionada, independente da rota preferida do
  host. Na troca, as entradas de conntrack dos clientes são derrubadas
  (`conntrack -D`, best-effort) para os fluxos reconectarem já pela
  fonte nova. Um monitor de uplink (`services/worker/hotspot/
  uplink_monitor.sh`) roda **sempre**: lê `INTERNET_INTERFACE` direto
  do banco e, quando o painel troca a fonte (quick-switch do card de
  resumo → `POST /api/hotspot/uplink`, que só grava a chave), alterna
  NAT/rota **ao vivo, sem derrubar/recriar o AP** — os clientes
  conectados não caem. No modo `auto` ele também vigia a **saúde** da
  fonte atual a cada `UPLINK_HEALTH_CHECK_INTERVAL` segundos (padrão
  3s; env do container, sem equivalente no painel): link físico via
  `carrier` **e internet de verdade** via `ping -I` pela própria
  interface (alvos = `HOTSPOT_DNS_FALLBACKS`, timeout 2s — 1s dava
  falso-negativo na STA Wi-Fi, cujo rádio é compartilhado com o AP).
  Link físico morto = failover **imediato e orientado a evento**: o
  monitor dorme escutando `ip monitor link` (netlink) e acorda no
  instante em que a interface atual muda de estado — desligar a placa
  à mão é percebido em ~1s, sem esperar o tick de polling (que
  continua existindo como retaguarda); internet perdida com link de
  pé = failover após `UPLINK_HEALTH_FAILURES_THRESHOLD` checagens
  ruins seguidas (padrão 2, ~6-9s — 1 ping perdido é normal e trocar
  na primeira falha causava ping-pong entre fontes). O destino é
  sempre a próxima candidata **com internet comprovada**; sem nenhuma
  comprovada, mantém a fonte atual (nunca troca "no escuro") — exceto
  com o link físico morto, quando qualquer candidata com carrier
  serve;
  a reavaliação "apareceu candidata melhor?" e a leitura do painel
  continuam na cadência de `UPLINK_MONITOR_INTERVAL` (padrão 10s). O
  painel acompanha: cada troca loga "Fonte real de internet do
  hotspot: ...", linha da qual `GET /api/hotspot/status` extrai a
  interface em uso (`parseHotspotInternetInterface`) — o card de
  resumo mostra "auto (interface)" atualizado no poll de 5s. A única
  troca que exige reiniciar o hotspot é passar a fonte para a própria
  placa Wi-Fi (`INTERNET_INTERFACE == WIFI_INTERFACE`) enquanto o AP
  está em `--no-virt` (sem associação de estação): sem estação não há
  uplink Wi-Fi possível, o monitor loga um aviso e mantém a fonte
  atual até um restart reassociar a placa. `attempt_hotspot_cycle`
  também relê essa chave do banco antes de cada rodada
  (`refresh_internet_strategy_from_db`), para um retry pós-queda já
  raciocinar com a fonte mais recente. `INTERNET_INTERFACE` pode ser a
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
- **Wi-Fi para Wi-Fi com `WIFI_CHANNEL=auto`** (`WIFI_INTERFACE` ==
  `INTERNET_INTERFACE`, ver `attempt_hotspot_cycle`/`try_create_ap` em
  `entrypoint.sh`): se a placa já está associada como cliente Wi-Fi no
  momento da tentativa, o hotspot **pula a varredura/ranking de canais
  candidatos inteira** (`rank_channels_for_band`, que faz `iw dev ...
  scan`) e trava direto no canal/banda que a associação já está usando
  (`sta_current_band_channel`) — o próprio `create_ap` sobrescreveria
  banda/canal pra bater com a estação de qualquer forma, então escanear
  antes só arrisca derrubar essa mesma conexão cliente (que alimenta o
  hotspot) sem nenhum ganho. Se a placa **não** está associada como
  cliente nesse momento e `WIFI_INTERFACE` é igual a
  `INTERNET_INTERFACE`, o hotspot **falha explicitamente** (retentável,
  mesmo loop de backoff usado para qualquer outra queda) em vez de
  aceitar `--no-virt` como sucesso: sem uma interface virtual `ap0`
  preservando a associação, a mesma placa não pode ser AP puro e
  estação ao mesmo tempo, e um AP sem nenhuma internet por trás
  apareceria como "rodando" no painel enganosamente. Essa checagem não
  se aplica quando `WIFI_INTERFACE` ≠ `INTERNET_INTERFACE` (ex.:
  hotspot pela Wi-Fi com internet vindo de uma interface cabeada) —
  `--no-virt` nesse caso é comportamento correto e esperado.
  **`sta_link_connected`** (`sta_link.sh`), usada por essa checagem em
  `try_create_ap` e por `sta_current_band_channel` (o "trava direto no
  canal" acima também depende dela), lê o **estado vivo do driver**, e
  não a saída de `iw dev ... link`: `iw dev ... station dump` (tabela
  de estações do mac80211 — em modo managed a única entrada possível é
  o AP da associação atual) confirma a associação, e `iw dev ... info`
  (linha `channel`, o chanctx sintonizado) dá banda/canal; `iw dev
  link` fica só como sinal alternativo. Motivo: `iw dev link` responde
  a partir do **cache de resultados de varredura** do cfg80211, e foi
  confirmado ao vivo (2026-07-16, iwlwifi/AX211, cruzando
  `journalctl` do NetworkManager/`wpa_supplicant` com o log do
  container) que ele reporta "Not connected" por **minutos** seguidos
  com a associação real de pé o tempo todo — foi exatamente isso que
  prendia o Wi-Fi-para-Wi-Fi no loop "não foi possível confirmar o
  canal atual da associação" até desistir. Além do sinal confiável,
  `sta_link_connected` ainda tolera reconexões em andamento: até
  `STA_LINK_CHECK_ATTEMPTS` leituras (padrão 20, a cada
  `STA_LINK_CHECK_INTERVAL_SECONDS`, padrão 0.4s — env do container,
  sem equivalente no painel, mesmo padrão de
  `HOTSPOT_BEACON_FAILURE_THRESHOLD` em `watchdog.sh`) antes de
  considerar "não associada" de verdade, porque o custo de esperar
  mais um pouco aqui é bem menor que cair pro loop de retry externo
  (backoff de 3s/6s/9s.../`HOTSPOT_RESTART_BACKOFF_SECONDS` por
  tentativa) só pra repetir a mesma checagem.
  **Essa paciência só se aplica quando `WIFI_INTERFACE` também é a
  fonte de internet** (Wi-Fi para Wi-Fi de verdade) — em qualquer
  outro modo (ex.: Ethernet para Wi-Fi), `sta_link_connected` faz uma
  única leitura rápida e sem espera, porque a placa foi deliberadamente
  desconectada/desgerenciada antes do hotspot subir e nunca vai
  "reconectar" sozinha; aplicar a mesma paciência ali é inútil e
  caro — `rank_channels_for_band`/`start_hotspot_auto` chamam
  `try_create_ap` (que usa essa checagem) uma vez por canal candidato,
  então 8s de espera à toa por candidato somava mais de um minuto de
  atraso na subida do hotspot Ethernet-para-Wi-Fi.
  **Mesmo com essa paciência, se `sta_current_band_channel` ainda
  assim falhar** (associação genuinamente instável por mais tempo que
  isso), Wi-Fi para Wi-Fi **nunca** cai pro caminho antigo de
  varredura ativa de canais (`start_hotspot_auto`/
  `rank_channels_for_band`, que roda `iw dev ... scan`) — confirmado
  ao vivo que esse scan ativo é bem mais perturbador pra uma estação
  associada do que a varredura passiva de fundo do NetworkManager, e
  cair nele só piorava a instabilidade da própria conexão cliente que
  alimenta o hotspot, num círculo vicioso. Em vez disso, retorna falha
  retentável direto e deixa o loop de retry externo (sem nenhum scan
  no meio) esperar a estação estabilizar sozinha — não há perda
  nenhuma, já que o canal do AP é sempre forçado pro canal da estação
  neste modo de qualquer forma, nunca escolhido pelo ranking de
  interferência. Pela mesma razão, `try_create_ap` também nunca sobe o
  AP com banda/canal não confirmados contra a estação: se
  `sta_current_band_channel` falhar bem no meio de `try_create_ap`
  (STA associada segundos atrás, mas o canal não confirma agora), a
  tentativa falha (retentável) em vez de seguir com os valores
  originais (ex.: um candidato de banda/canal vindo de uma tentativa
  anterior) — confirmado ao vivo que isso produz um `hostapd`
  inconsistente (o `create_ap` força o canal certo nos bastidores, mas
  a banda/capacidades declaradas ficam erradas) e clientes reais nunca
  completam a autenticação ("did not acknowledge authentication
  response"), mesmo com o AP aparecendo "ENABLED" no log.
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
  mDNS/outros links. Entradas com vários labels (`local.com`) são
  aceitas e anunciadas como estão; prefixos `*.`/`**.`/`.` são
  descartados, igual à normalização do `dns-provider`.
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
  2. O `worker`, bem em cima do `docker exec ... start`
     (`unmanageWifiInterfaceIfIdle`, `services/worker/controller/compose.go`),
     decide quem fica dono de `WIFI_INTERFACE` no NetworkManager:
     - `WIFI_INTERFACE == INTERNET_INTERFACE` (Wi-Fi para Wi-Fi):
       a placa fica gerenciada **incondicionalmente**, sem checar se
       está associada agora — checar seria uma corrida real (pegar a
       placa momentaneamente sem associação durante uma reconexão
       desgerenciava mesmo assim, travando-a desconectada **para
       sempre**, já que só o NetworkManager a reconectaria; confirmado
       ao vivo). Além de não desgerenciar, o worker **re-gerencia**
       (`manageWifiInterface`, idempotente — remove o drop-in e roda
       `managed yes`): sem isso, trocar de um modo que desgerenciou a
       placa (ex.: Ethernet para Wi-Fi com a placa ociosa) para Wi-Fi
       para Wi-Fi deixava o drop-in órfão e a placa presa "unmanaged".
     - Placas **diferentes** com a placa Wi-Fi **associada como
       cliente** (`interfaceAssociated`): a associação é preservada
       (placa continua gerenciada; mesmo `manageWifiInterface`
       idempotente). O hotspot sobe numa `ap0` virtual travada no
       canal da estação — mesma topologia de rádio do Wi-Fi para
       Wi-Fi, que comprovadamente convive com o NetworkManager. É o
       que garante que **partilhar internet do Ethernet não desconecta
       o Wi-Fi cliente do usuário nem o faz "sumir do sistema"**
       (device unmanaged desaparece do menu de rede do desktop).
     - Placas diferentes **sem associação**: desgerencia/desconecta
       como antes — o AP vai subir em `--no-virt` na placa física
       inteira, e deixá-la gerenciada faria o NetworkManager competir
       pelo rádio com o `hostapd` (escaneando/tentando reassociar),
       derrubando o beacon. Corrida residual (associação cair entre a
       checagem e o `create_ap`): o watchdog de beacon é a rede de
       segurança, e o NetworkManager reassociando devolve o caminho
       preservado na tentativa seguinte.
     O container `hotspot` delega ao `create_ap` a criação da
     interface AP virtual (`ap0`) quando há estação a preservar. A
     fonte de internet entregue ao `create_ap` é sempre o uplink
     virtual `BINDNET_UPLINK_INTERFACE`, alimentado por regras Bindnet
     de NAT/forward a partir da interface real configurada.
- `POST /api/hotspot/stop` desfaz exatamente o inverso, na ordem
  inversa:
  1. Para `hotspot` + `dns-provider` (`docker stop`, via `worker`) —
     internamente, o comando `stop` do `entrypoint.sh` sempre passa
     por `force_stop_create_ap` (mesma função usada por `cleanup()` na
     saída normal do serviço), que aplica timeout + escalona pra
     `SIGKILL` (o `create_ap`/`hostapd` diretos e, por varredura final,
     qualquer processo órfão ainda referenciando o diretório de config
     dessa interface) em vez de confiar só num `create_ap --stop`
     "educado" — um `hostapd` preso no loop de falha de beacon (ver
     `watchdog.sh`) ignora sinal de parada limpa e ficaria órfão
     segurando a placa física indefinidamente, mesmo com o `stop`
     reportando sucesso.
  2. Garante que qualquer drop-in antigo do NetworkManager seja removido
     e devolve `WIFI_INTERFACE` ao controle do NetworkManager
     (`worker`: `POST /network/wifi-manage`, que roda `nmcli device set
     ... managed yes`). O container também remove `bn-uplink` e as
     chains `BINDNET-HOTSPOT` ao sair.
- `POST /api/hotspot/recover-wifi` é a ação operacional do botão
  "Recuperar Wi-Fi" na tela "Hotspot Wi-Fi": derruba/para o hotspot em
  execução (mesmo caminho de `POST /api/hotspot/stop`, incluindo o
  `force_stop_create_ap` acima — garante que nenhum `hostapd`/`create_ap`
  fica órfão segurando a placa) e em seguida recupera o controle da
  interface via `wifi-manage`, devolvendo-a ao NetworkManager. É a ação
  segura para destravar a placa quando ela ficou presa como não
  gerenciada (ou com um hotspot zumbi ainda vivo) após queda/restart/
  interrupção fora do fluxo normal.
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
- **Taxa e cota aceitam valor fracionário** na unidade escolhida (ex.:
  cota diária de `1.5GB`, taxa de download de `17.5KB/s`), tanto em
  perfil quanto em override de dispositivo. O formulário aceita `.` ou
  `,` como separador decimal, já que a UI é em português (ver
  `optionalPositiveDecimal` em `hotspot-number-schema.ts`). Cota sempre
  coube no banco (é gravada em bytes, `BIGINT`); taxa era `INTEGER` e
  passou a `double precision` na migration
  `20260716000000_hotspot_rate_decimal` — a fração chega até o `tc`
  como está (`rate 17.5kbps`, ver `rate()` em `shaping_tc.go`). A
  política de crédito (recarga/plafond, em GB) **continua só inteira**.
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
  genérico — não existe mais um limite global independente de
  perfil/dispositivo, removido por não fazer mais sentido com limite
  sempre por dispositivo ou perfil selecionado):
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

## Isolamento de clientes (`services/backend/internal/hotspot/hotspot_isolation*.go`, `services/worker/controller/internal/shaping/isolation*.go`)

- **Interruptor geral**: chave `CLIENT_ISOLATION` em `hotspot_config`
  (default `false`), editável pela aba Isolamento do painel
  (`PUT /api/hotspot/isolation`). Ligado, o `entrypoint.sh` sobe o
  create_ap com `--isolate-clients` (`ap_isolate` do hostapd): o AP
  para de retransmitir frames entre as próprias estações em L2.
  **Mudar o interruptor só vale após reiniciar o hotspot** — o
  `ap_isolate` é fixado no `hostapd.conf` no start; o painel avisa e
  nunca reinicia sozinho. Se o create_ap baixado não suportar a flag
  com o isolamento ligado, o entrypoint falha alto em vez de subir um
  AP silenciosamente sem isolamento.
- **Mecânica**: com o L2 direto cortado, o worker liga
  `proxy_arp_pvlan=1` (RFC 3069) e `send_redirects=0` na interface AP —
  o host responde ARP em nome dos outros clientes e o tráfego
  cliente↔cliente passa a ser roteado em hairpin
  (cliente→host→cliente), atravessando `filter/FORWARD` onde o chain
  `BINDNET-ISOLATION` (jump `-i <ap> -o <ap>`) decide:
  `RELATED,ESTABLISHED` no topo, um ACCEPT por par permitido
  (`--mac-source` origem + IP destino, comentários `bn-iso-pair-*`) e
  DROP no fim — **default deny**. Tráfego cliente→internet
  (`-o bn-uplink`) e cliente→gateway (painel/DNS/portal, INPUT) nunca
  passam por esse chain e não são afetados.
- **Política** (motor puro em `hotspot_isolation_policy.go`): a
  comunicação de X para Y é decidida pela regra mais **específica** que
  casar — especificidade = soma das extremidades (dispositivo=2,
  perfil=1, qualquer=0); empate na mesma especificidade → **bloquear
  vence**; nenhuma regra → bloqueado. Regras (`hotspot_comm_rules`) têm
  origem (dispositivo|perfil), destino (dispositivo|perfil|qualquer),
  sentido e ação (permitir|bloquear).
- **Firewall por zonas / camada L4** (base em `hotspot_comm_rules`,
  colunas `zone`/`protocol`/`dst_ports`/`dst_host`, migration
  `20260721010000_hotspot_firewall_l4`): cada regra tem uma **zona** —
  `clients` (cliente↔cliente, chain BINDNET-ISOLATION), `wan`
  (cliente→internet, chain BINDNET-FW-WAN) e `local` (cliente→gateway,
  chain BINDNET-FW-LOCAL). Uma regra pode restringir por **protocolo**
  (`any`/`tcp`/`udp`/`icmp`) e **portas de destino** (`dst_ports`, lista
  `80,443,8000-8100`, só com tcp/udp). Na zona clients, o motor emite,
  por par ordenado (MAC origem→IP destino), as entradas **na ordem em
  que o firewall avalia**: ponta mais específica primeiro, depois L4
  mais específico (protocolo/porta concretos acima de `any`), e
  bloquear antes de permitir em empate. O worker reconstrói o chain só
  quando essa assinatura ordenada muda (idempotente por comparação de
  comentários), instala cada entrada como ACCEPT/DROP com
  `-p <proto> -m multiport --dports <portas>` e mantém o DROP final
  (default deny). Assim "permitir TCP/443 entre A e B" libera só 443 e o
  resto cai no DROP; "bloquear UDP + permitir tudo" bloqueia só UDP.
  Regras retrocompatíveis: sem zona = `clients`, sem protocolo = `any`.
- **Zonas wan e local** (`hotspot_firewall_policy.go`,
  `services/worker/controller/internal/shaping/firewall_{zones,wan,local}.go`):
  ao contrário da zona clients, **não** dependem de `ap_isolate` nem de
  reiniciar o hotspot — são puro iptables aplicado ao vivo enquanto o
  hotspot roda (reconciliação, reaplicação pós-start e teardown no stop,
  ver `hotspot_firewall_apply.go`). Cada regra casa pela **origem**
  (dispositivo|perfil|todos os clientes) + L4; a zona `wan` aceita ainda
  um `dst_host` (IP/CIDR externo). Cada zona tem uma **política padrão**
  (`FW_WAN_POLICY`/`FW_LOCAL_POLICY` em `hotspot_config`, default
  `allow`) para o tráfego que nenhuma regra cobre — default `allow` de
  propósito: adicionar o firewall não muda nada até haver regras, e nunca
  corta internet/painel sozinho.
  - **wan** (`BINDNET-FW-WAN`, jump em FORWARD para `-i <ap> -o
    <uplink>`): usa **RETURN** para "permitir" (o pacote segue para
    `BINDNET-HOTSPOT`, onde ainda passam o bloqueio por crédito e o
    MASQUERADE — permitir na wan **não** fura o crédito) e **DROP** para
    "bloquear". O jump é mantido **acima** do de `BINDNET-HOTSPOT` no
    FORWARD (senão o ACCEPT genérico do hotspot liberaria antes das
    regras de bloqueio); a correção de ordem acontece no ciclo de
    reconciliação (janela ≤15s após uma troca de uplink).
  - **local** (`BINDNET-FW-LOCAL`, jump em INPUT para `-i <ap>`): usa
    **ACCEPT** para "permitir" e **DROP** para "bloquear". No topo há
    permissões **fixas e inegociáveis** (established, DHCP, DNS, painel
    HTTP/HTTPS, ICMP) para o operador nunca se trancar fora do
    painel/rede, mesmo com política `deny`.
- **Modalidades no painel** (aba Isolamento): uma regra tem dois
  escopos na UI. **Dentro de um perfil** = comunicação entre os
  clientes do mesmo perfil; é gravada como uma regra normal com origem
  **e** destino iguais ao mesmo perfil (perfil↔próprio-perfil,
  especificidade 2) — não há mais um flag separado por perfil, é uma
  regra como as outras. **Entre origem e destino** = origem e destino
  distintos (dispositivo|perfil, destino também "todos os clientes").
  Assim: dispositivo↔qualquer bloquear empata com a regra interna do
  perfil e vence (cliente totalmente isolado); dispositivo↔dispositivo
  permitir (especificidade 4) vence tudo (exceção pontual). A coluna
  `hotspot_profiles.allow_internal_communication` continua no banco e o
  motor ainda a respeita como allow implícito perfil↔próprio-perfil,
  mas o painel não a define mais (fica sempre `false`) — a comunicação
  interna é expressa por regra mesmo-perfil.
- **Sentido** (`direction`): `to` = a origem pode **iniciar** tráfego
  para o destino; as respostas voltam pelo conntrack
  (`RELATED,ESTABLISHED`), senão TCP não funcionaria. `both` = os dois
  podem iniciar. O caso "só destino→origem" da UI é gravado com as
  extremidades trocadas — o banco só conhece `to`/`both`.
- **Aplicação**: o backend compila o estado desejado completo (pares
  MAC origem→IP destino dos clientes conectados agora) e o worker
  materializa idempotente (`POST /hotspot/isolation/apply`), sem
  estado local — mesmo modelo do shaping. Reaplicado a cada ciclo de
  reconciliação (~15s: cobre cliente novo, renovação de DHCP e regra
  perdida por reinício de container), em toda mutação de
  regra/perfil/vínculo, e com retry pós-start
  (`reapplyHotspotIsolation`); o stop do hotspot desmonta
  chain/sysctls. Janela de até um ciclo para pares novos valerem é
  aceite por desenho.
- **Limitações aceites**: com o isolamento ligado, broadcast/multicast
  entre clientes (mDNS/Chromecast/descoberta de impressora) não
  funciona nem entre pares permitidos — só unicast roteado; IPv6
  link-local entre clientes fica bloqueado por desenho (`ap_isolate`
  corta o L2 e não há proxy NDP). Regras de perfil apagado somem junto
  com o perfil (mesma transação de `DeleteProfile`); os dispositivos
  dele voltam ao perfil Padrão.

## Serviço `dns-provider` (servidor DNS split-horizon próprio — `services/worker/dns/`)

Não usa mais CoreDNS/Corefile — é um binário Go próprio (`miekg/dns`),
pelo mesmo motivo que a gestão de certificados saiu do cert-proxy: o
CoreDNS/`template` plugin não tem como implementar split-horizon **por
IP de bind** nem alocação persistente de IP por hostname sem plugins
customizados fora da imagem oficial.

Regras:

- `DNS_LOCAL_TLDS` define os TLDs tratados como locais (padrão
  `local,test,example`). Cada entrada pode ser um TLD simples
  (`local`) ou um **sufixo com vários labels tratado como TLD local**
  (`local.com`, `a.b.local`): qualquer nome que termine nesse sufixo,
  em qualquer profundidade (`app.local.com`, `x.y.z.local.com`),
  resolve como zona local. Prefixos `*.`/`**.`/`.` e ponto final são
  descartados na normalização (`**.a.b.local` declara o mesmo sufixo
  que `a.b.local`). Cada label é validado (`[a-z0-9-]`, sem começar ou
  terminar com `-`); entrada inválida é erro fatal. Duplicatas são
  ignoradas silenciosamente. Um sufixo declarado em `DNS_LOCAL_TLDS`
  tem precedência sobre o fallback NXDOMAIN de nomes desconhecidos
  dentro de uma raiz ampla de `DOMAINS`. O painel
  (`PATCH /api/dns/config`) valida `DNS_LOCAL_TLDS` e `DOMAINS` com
  estas mesmas regras e rejeita valores inválidos com `400` antes de
  gravar na tabela `dns_config` — sem isso, um valor inválido salvo pelo
  painel deixava o `dns-provider` em loop de restart e derrubava toda a
  resolução local.
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
- Ao aplicar a configuração pelo painel, o worker mantém uma conexão
  dummy `bindnet-dns` (`bn-dns`) no NetworkManager com `127.0.0.1` como
  servidor e cada entrada de `DNS_LOCAL_TLDS`/`DOMAINS` como domínio
  route-only (`~sufixo`). Isso inclui sufixos com vários labels, como
  `~local.com`, para que o resolver do host envie a consulta ao socket
  da view host em vez do DNS público da ligação default.
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
     anos, CN vindo de `panel_config.CA_COMMON_NAME` (definido no painel
     em Configurações; padrão "Bindnet Local Development CA"). Mesmos
     parâmetros criptográficos do antigo `cert-proxy`. Trocar o CN depois
     não renomeia nem reemite a CA já criada — só valeria para uma CA
     gerada do zero.
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
  `/#/certificates/list?search={}`. Se as credenciais do `nginx-ui`
  estiverem definidas no painel (Configurações, tabela `panel_config`:
  `NGINX_UI_USERNAME`/`NGINX_UI_PASSWORD`), usa a API `/api/certs`;
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
- O painel **não edita mais o `.env`**. Toda configuração operacional
  vive no Postgres, em tabelas chave/valor: `hotspot_config`
  (`WIFI_*`, `HOTSPOT_GATEWAY`, `HOTSPOT_CIDR`, …), `dns_config`
  (`DNS_LOCAL_TLDS`, `DOMAINS`, `DISCOVER_NODE_NAME`,
  `DISCOVER_REMOTE_ROUTES`) e `panel_config` (`CA_COMMON_NAME`,
  `NGINX_UI_USERNAME`, `NGINX_UI_PASSWORD`). Cada serviço lê a sua
  tabela direto do banco quando inicia — ninguém reescreve arquivo de
  ambiente. `DISCOVER_PORT` continua vindo do `.env`/compose por ser
  porta de infraestrutura, não configuração de negócio.
- Na primeira subida após essa migração, `backend` e `dns-provider`
  fazem uma **importação única** do que ainda estiver no ambiente para
  as tabelas (`ON CONFLICT DO NOTHING`), para uma instalação existente
  não cair nos defaults. Depois disso o painel é a fonte de verdade e as
  variáveis podem sair do `.env`.
- "Salvar" configuração (grava no banco) e "aplicar" (recria o
  container via `docker compose up -d --no-build`) são ações
  separadas — o painel nunca reinicia o hotspot/DNS sozinho ao salvar,
  só quando o usuário confirma explicitamente "aplicar". Peers diretos
  da malha Bindnet seguem a mesma ideia, no Postgres
  (`discover_configured_peers`).
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
