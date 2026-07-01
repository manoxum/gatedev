# Regras de negócio — central

Este documento descreve o comportamento funcional esperado de cada
serviço do stack `central`. É a referência de "o que o sistema deve
fazer"; para "como rodar" veja o [README.md](README.md).

## Visão geral do domínio

O `central` provê, para uma rede local doméstica/pequeno escritório:

1. Um ponto de acesso Wi-Fi (hotspot) compartilhando a internet de uma
   interface cabeada/outra Wi-Fi.
2. Resolução DNS *split-horizon*: domínios internos (TLDs locais, ex.
   `.local`) resolvem para o gateway local; todo o resto é encaminhado
   para DNS público.
3. Um proxy HTTP/HTTPS com CA própria, que emite certificados TLS sob
   demanda para qualquer domínio local, para permitir HTTPS sem avisos
   de certificado durante desenvolvimento.
4. Uma UI (`nginx-ui`) para administrar os sites servidos atrás do proxy.

## Serviço `hotspot` (`hotspot/entrypoint.sh`)

Regras:

- Variáveis obrigatórias: `WIFI_INTERFACE`, `INTERNET_INTERFACE`,
  `WIFI_SSID`, `WIFI_PASSWORD`. Ausência de qualquer uma delas é erro
  fatal (o container não sobe).
- `WIFI_CHANNE` (sem "L") é aceito como alias legado de
  `WIFI_CHANNEL`, com aviso de depreciação no log. Não usar em
  configurações novas.
- **`WIFI_CHANNEL`** aceita:
  - um número de canal fixo (validado como inteiro); ou
  - `auto` (padrão): o hotspot escaneia o ambiente Wi-Fi
    (`iw dev ... scan`) e escolhe, entre os canais candidatos da banda
    resolvida, o de **menor pontuação de interferência**.
    - Em 2.4GHz a pontuação penaliza canais sobrepostos (distância 0 a
      4 do canal observado); em 5GHz os canais são considerados
      ortogonais (pontuação binária, mesmo canal = interferência).
    - Se a varredura falhar ou não retornar redes, usa o primeiro
      canal candidato mesmo assim (com aviso no log) — o hotspot nunca
      trava por falta de varredura.
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
    suportar 5GHz ou se a detecção falhar (com aviso no log).
- O hotspot exige que o binário `create_ap` baixado suporte
  `--no-dns` e `--dhcp-dns`; sem isso, falha explicitamente em vez de
  criar um AP com comportamento de DNS inesperado.
- DNS entregue via DHCP aos clientes do hotspot é sempre o próprio
  `HOTSPOT_GATEWAY` (o `dns-provider` responde por trás dele) — o
  hotspot nunca delega DNS para o `create_ap` (`--no-dns`).
- Ao encerrar (`SIGTERM`/`SIGINT`/saída normal), o hotspot **sempre**
  chama `create_ap --stop` antes de sair, para não deixar a placa em
  modo AP "preso".
- O container precisa rodar `privileged: true` e `network_mode: host`
  porque manipula diretamente a interface Wi-Fi física do host.

## Scripts `scripts/hotspot-on.sh` / `scripts/hotspot-off.sh`

Regras:

- `hotspot-on.sh`:
  1. Marca `WIFI_INTERFACE` (e `ap0`) como **não gerenciada** pelo
     NetworkManager, via drop-in em
     `/etc/NetworkManager/conf.d/90-central-hotspot-unmanaged.conf`,
     para o `hostapd` (dentro do `create_ap`) poder assumir a placa.
  2. Sobe `hotspot` + `dns-provider` via `docker compose up -d`.
- `hotspot-off.sh` desfaz exatamente o inverso, na ordem inversa:
  1. Para `hotspot` + `dns-provider`.
  2. Remove o drop-in do NetworkManager.
  3. Devolve `WIFI_INTERFACE` ao controle do NetworkManager
     (`nmcli device set ... managed yes`).
- Ambos os scripts leem `WIFI_INTERFACE` do `.env` na raiz do repo, com
  fallback para `wlp0s20f3` se o `.env` não existir ou a variável não
  estiver definida.
- Essas operações mexem em configuração real do host (NetworkManager,
  placa Wi-Fi física) e exigem `sudo` — não são operações "só Docker".
  Rodar `hotspot-on.sh` desconecta a placa Wi-Fi do uso normal como
  cliente; `hotspot-off.sh` é o único caminho suportado para reverter
  isso de forma limpa.

## Serviço `dns-provider` (CoreDNS split-horizon — `coredns/`)

Regras:

- `DNS_LOCAL_TLDS` define os TLDs tratados como locais (padrão
  `local,test,example`). Cada TLD é validado (`[a-z0-9-]`, sem começar
  ou terminar com `-`); TLD inválido é erro fatal. Duplicatas são
  ignoradas silenciosamente.
- Para os TLDs locais:
  - Registros `A` sempre respondem com `HOTSPOT_GATEWAY`.
  - Registros `AAAA` e `ANY` sempre respondem `NXDOMAIN` — o stack não
    tem IPv6 nem faz split-horizon multi-tipo para esses domínios.
- Para qualquer outro domínio, a consulta é encaminhada para DNS
  público (`8.8.8.8`, `1.1.1.1`).
- O CoreDNS escuta obrigatoriamente em `127.0.0.1` (o host resolve
  DNS local via `resolved.conf.d` apontando para `127.0.0.1`), mais
  `DOCKER_HOST_GATEWAY` e `HOTSPOT_GATEWAY` quando definidos — isso é
  proposital: escutar só em `HOTSPOT_GATEWAY` não é suficiente para o
  próprio host resolver os domínios locais.
- Antes de iniciar o CoreDNS, o entrypoint **espera** (até
  `COREDNS_WAIT_TIMEOUT`, padrão 90s) os IPs obrigatórios existirem na
  máquina. Isso existe porque `dns-provider` depende do `hotspot` ter
  criado a interface/gateway antes; se o timeout estourar, o container
  falha com uma mensagem explicando a causa provável (hotspot não
  subiu, ou `DOCKER_HOST_GATEWAY`/`HOTSPOT_GATEWAY` incorretos).

## Serviço `cert-proxy` (`cert-proxy/main.go`)

Regras:

- Na primeira execução, gera uma CA local (RSA 4096, válida 10 anos) e
  persiste em `/data/ca.crt` + `/data/ca.key` (volume
  `cert_proxy_data`). Execuções seguintes **reaproveitam** a CA
  existente — a CA nunca é regerada automaticamente enquanto o volume
  existir.
- Para cada domínio (`SNI`) requisitado via HTTPS, emite (e cacheia em
  disco) um certificado *leaf* assinado pela CA local (RSA 2048,
  válido 2 anos), sob demanda, na primeira conexão TLS para aquele
  domínio.
- O nome do domínio é normalizado antes de emitir/buscar certificado:
  minúsculas, sem porta, sem `.` final; se o domínio não passar numa
  validação básica de caracteres (`[a-z0-9-]` por rótulo), cai para
  `localhost.local` em vez de emitir certificado para um valor não
  sanitizado — isso evita usar `ServerName` de TLS não confiável
  diretamente na geração de certificados/nomes de arquivo.
- Endpoints especiais servem o certificado da CA (para importar no
  cliente), independente de domínio: `/ca.crt`, `/local-ca.crt`,
  `/.well-known/local-ca.crt`, e `http://ca.local/`.
- Todo tráfego que não for esses endpoints é repassado (reverse proxy)
  para `UPSTREAM_HTTP_URL`/`UPSTREAM_HTTPS_URL` (por padrão, o
  `nginx-ui`); a verificação de certificado do upstream HTTPS é
  desabilitada de propósito, porque é um proxy interno entre
  containers confiáveis do mesmo stack, não tráfego externo.
- Escuta sempre `:80` e `:443` simultaneamente; falha ao subir se
  qualquer uma das portas não puder ser aberta.

## Serviço `nginx-ui`

- Interface administrativa para configurar os sites/rotas servidos
  atrás do `cert-proxy`. Exposta diretamente em `9080` (porta
  administrativa) além de ser alcançável via `cert-proxy` em 80/443.
- Estado (configuração de sites, dados da UI, arquivos servidos) é
  persistido nos volumes externos `nginx_config`, `nginx_ui_data` e
  `www_data` — esses volumes **não** são gerenciados pelo
  `docker-compose.yml` (são `external: true`) e precisam existir antes
  do primeiro `docker compose up` (ver README).

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
  `dns-provider`) usam `network_mode: host`; os demais (`nginx-ui`,
  `cert-proxy`) ficam isolados na rede Docker `proxy`.
