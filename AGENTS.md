# AGENTS.md

Diretrizes para agentes de codificação (Codex e similares) trabalhando
neste repositório. Leia também [README.md](README.md) (visão geral e
como rodar) e [RULE.md](RULE.md) (regras de negócio de cada serviço)
antes de propor mudanças de comportamento.

## O que é este repositório

`central` é um stack Docker Compose que roda **em uma máquina física
real**, controlando hardware de verdade: cria um hotspot Wi-Fi
(`hotspot`), assume a placa Wi-Fi do host tirando-a do NetworkManager,
serve DNS split-horizon (`dns-provider`), termina TLS com uma CA local
autoassinada (`cert-proxy`) e expõe uma UI de administração
(`nginx-ui`). Não é um projeto de aplicação isolado — ações aqui podem
tirar a máquina do usuário da rede Wi-Fi ou da internet.

## Regras de segurança e cautela — leia antes de agir

- **Não execute `scripts/hotspot-on.sh` ou `scripts/hotspot-off.sh`
  por conta própria.** Eles usam `sudo`, reconfiguram o
  NetworkManager e desconectam/reconectam a placa Wi-Fi física do
  usuário. Proponha o comando para o usuário rodar, ou peça
  confirmação explícita antes de executar.
- **Não rode `docker compose up/down/restart` neste repo sem
  confirmar antes**, especialmente para os serviços `hotspot` e
  `dns-provider`: isso pode derrubar a conexão de rede que outras
  pessoas/dispositivos estão usando agora. `nginx-ui` e `cert-proxy`
  têm menor risco, mas ainda assim confirme antes de agir.
- **Não edite ou apague os volumes Docker externos**
  (`central_nginx_config`, `central_nginx_ui_data`, `central_www_data`,
  `central_cert_proxy_data`) nem o conteúdo onde `cert-proxy` guarda
  `/data/ca.crt` e `/data/ca.key` — apagar a CA local invalida todos os
  certificados já emitidos e confiados nos dispositivos do usuário.
- **Não commite `.env`** (contém `WIFI_PASSWORD` e outros dados
  específicos da máquina). Para documentar uma variável nova, edite
  `.env.example`, nunca `.env`.
- Para validar mudanças em `hotspot/entrypoint.sh` ou nos scripts de
  rede, prefira validação estática (`bash -n`, leitura cuidadosa da
  lógica) a executar de verdade — rodar de verdade exige acesso à
  placa Wi-Fi real e derruba a rede do usuário durante o teste.
- Não faça alterações fora do escopo pedido (sem refatorações
  especulativas, sem novas abstrações "para o futuro"): este stack é
  uma instalação única de infraestrutura pessoal, não um produto
  multi-tenant.

## Convenções do projeto

- Scripts shell começam com `set -euo pipefail` (bash) ou `set -eu`
  (`sh`, ex: `coredns/docker-entrypoint.sh`), falham alto com mensagem
  clara e `exit 1` em vez de continuar em estado inconsistente.
- Logs e comentários são em **português**, no padrão
  `[nome-do-servico] mensagem` (veja a função `log()` em cada
  entrypoint). Siga esse padrão em código novo/alterado.
- Variáveis de ambiente novas: defina um valor padrão sensato via
  `${VAR:-padrao}` quando fizer sentido, documente em `.env.example`
  (com comentário explicando obrigatoriedade e efeito) e, se alterarem
  comportamento de negócio, descreva em `RULE.md`.
- `cert-proxy/main.go` é um único arquivo Go, sem dependências
  externas (`go.mod` sem `require`) — mantenha assim a menos que haja
  motivo forte para introduzir uma dependência; identificadores em
  português seguem o padrão já existente no arquivo.

## Como validar mudanças

- Shell scripts: `bash -n <arquivo>` (sintaxe) no mínimo; para
  mudanças de lógica, releia manualmente os fluxos de
  `hotspot/entrypoint.sh` e `coredns/docker-entrypoint.sh` — não há
  suíte de testes automatizada neste repo.
- Go: `cd cert-proxy && go build ./...` e `go vet ./...`.
- Compose: `docker compose config` para validar sintaxe/interpolação de
  variáveis sem subir nada.
- Antes de considerar uma tarefa em `hotspot/entrypoint.sh` concluída,
  confirme que as regras de `RULE.md` (seleção de canal, banda,
  variáveis obrigatórias) continuam válidas.

## Estrutura rápida

```
docker-compose.yml   # orquestra todos os serviços
.env.example          # template documentado de variáveis (versionado)
.env                   # valores reais da máquina (git-ignored)
hotspot/               # cria o AP Wi-Fi (create_ap)
coredns/                # DNS split-horizon
cert-proxy/              # proxy TLS com CA local (Go)
scripts/                  # hotspot-on.sh / hotspot-off.sh (mexem no host)
```
