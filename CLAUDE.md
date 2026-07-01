# CLAUDE.md

Diretrizes para o Claude Code (e Claude em geral) trabalhando neste
repositório. Leia também [README.md](README.md) (visão geral e como
rodar) e [RULE.md](RULE.md) (regras de negócio de cada serviço) antes
de propor mudanças de comportamento.

## O que é este repositório

`central` é um stack Docker Compose que roda **em uma máquina física
real**, controlando hardware de verdade: cria um hotspot Wi-Fi
(`hotspot`), assume a placa Wi-Fi do host tirando-a do NetworkManager,
serve DNS split-horizon (`dns-provider`), termina TLS com uma CA local
autoassinada (`cert-proxy`) e expõe uma UI de administração
(`nginx-ui`). Não é um projeto de aplicação isolado — ações aqui podem
tirar a máquina do usuário da rede Wi-Fi ou da internet.

## Regras de segurança e cautela — leia antes de agir

- **Nunca execute `scripts/hotspot-on.sh` ou `scripts/hotspot-off.sh`
  por conta própria.** Eles usam `sudo`, reconfiguram o
  NetworkManager e desconectam/reconectam a placa Wi-Fi física do
  usuário. Proponha o comando e peça para o usuário rodar, ou peça
  confirmação explícita antes de rodar.
- **Nunca rode `docker compose up/down/restart` neste repo sem
  confirmar antes**, especialmente para `hotspot` e `dns-provider`:
  isso pode derrubar a conexão de rede que outras pessoas/dispositivos
  estão usando agora. `nginx-ui` e `cert-proxy` têm menor risco, mas
  ainda assim confirme.
- **Nunca edite ou apague os volumes Docker externos**
  (`central_nginx_config`, `central_nginx_ui_data`, `central_www_data`,
  `central_cert_proxy_data`) nem o volume onde `cert-proxy` guarda
  `/data/ca.crt` e `/data/ca.key` — apagar a CA local invalida todos os
  certificados já emitidos e confiados nos dispositivos do usuário.
- **Nunca commite `.env`** (contém `WIFI_PASSWORD` e outros dados
  específicos da máquina). Se precisar documentar uma variável nova,
  edite `.env.example`, nunca `.env`.
- Se uma tarefa pedir para "testar" mudanças em `hotspot/entrypoint.sh`
  ou nos scripts de rede, prefira validação estática (`bash -n`,
  leitura cuidadosa da lógica) a rodar de verdade — rodar de verdade
  exige acesso à placa Wi-Fi real e derruba a rede do usuário durante
  o teste.

## Convenções do projeto

- Scripts shell começam com `set -euo pipefail` (bash) ou `set -eu`
  (`sh`/`coredns/docker-entrypoint.sh`), falham alto com mensagem
  clara e `exit 1` em vez de continuar em estado inconsistente.
- Logs e comentários são em **português**, no padrão
  `[nome-do-servico] mensagem` (veja `log()` em cada entrypoint).
  Siga esse padrão em código novo/alterado neste repo.
- Variáveis de ambiente novas: sempre com valor padrão sensato via
  `${VAR:-padrao}` quando fizer sentido, documentadas em
  `.env.example` (com comentário explicando obrigatoriedade e efeito)
  e, se alterarem comportamento de negócio, descritas em `RULE.md`.
- Go (`cert-proxy/main.go`) é um único arquivo, sem dependências
  externas (`go.mod` sem `require`) — mantenha assim a menos que haja
  motivo forte para introduzir uma dependência; nomes de
  funções/variáveis em português seguem o padrão já existente no
  arquivo.
- Não introduza abstrações, flags ou configuração especulativa para
  cenários que este stack não tem hoje (é uma instalação única, não um
  produto multi-tenant).

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
  variáveis obrigatórias) continuam válidas — é fácil quebrar a lógica
  de fallback silenciosamente.

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
