# CLAUDE.md

Diretrizes para o Claude Code (e Claude em geral) trabalhando neste
repositório. Leia também [README.md](README.md) (visão geral e como
rodar) e [RULE.md](RULE.md) (regras de negócio de cada serviço) antes
de propor mudanças de comportamento.

## O que é este repositório

`bindnet` é um stack Docker Compose que roda **em uma máquina física
real**, controlando hardware de verdade: cria um hotspot Wi-Fi
(`hotspot`), assume a placa Wi-Fi do host tirando-a do NetworkManager,
serve DNS split-horizon (`dns-provider`), expõe uma UI de administração
(`nginx-ui`), e tem um painel de gestão web próprio (`services/frontend`
+ `services/backend` + `services/worker`) para operar tudo isso sem
shell — incluindo emitir/gerenciar certificados TLS assinados por uma
CA local própria, armazenada no PostgreSQL. O stack também inclui
MongoDB (trilha de auditoria do painel), Redis (cache do
`dns-provider`) e MinIO (provisionado, sem uso ainda), com o schema do
Postgres aplicado por um job Node.js/Prisma (`services/migration`).
Não é um projeto de aplicação isolado — ações aqui podem tirar a
máquina do usuário da rede Wi-Fi ou da internet.

## Regras de segurança e cautela — leia antes de agir

- **Nunca chame `POST /api/hotspot/start` ou `/stop` do painel (nem os
  endpoints equivalentes do `worker`, `/network/wifi-unmanage` e
  `/hotspot/apply`) por conta própria.** Não existem mais scripts de
  shell separados para isso (removidos) — ligar/desligar o hotspot é
  feito só pelo painel/API, e reconfigura o NetworkManager e
  desconecta/reconecta a placa Wi-Fi física do usuário exatamente como
  os scripts antigos faziam. Proponha a ação e peça para o usuário
  confirmar/clicar, ou peça confirmação explícita antes de chamar a
  API diretamente.
- **Nunca rode `docker compose up/down/restart` neste repo sem
  confirmar antes**, especialmente para `hotspot` e `dns-provider`
  (e para os endpoints do painel que fazem a mesma coisa via
  `services/worker` — iniciar/parar/aplicar hotspot/DNS pelo painel tem
  o mesmo risco que rodar os comandos direto): isso pode derrubar a
  conexão de rede que outras pessoas/dispositivos estão usando agora.
  `nginx-ui`, `worker`, `backend`, `frontend`, `postgres`, `mongo`,
  `minio` e `migration` têm menor risco, mas ainda assim confirme.
- **Nunca edite ou apague os volumes Docker persistentes do stack**
  (`nginx_config`, `nginx_ui_data`, `www_data`,
  `cert_proxy_data`, `admin_data`,
  `postgres_data`, `mongo_data`, `minio_data`). Esses são os nomes
  lógicos no `docker-compose.yml`; como não há `name:` fixo, o Docker
  Compose cria os volumes reais escopados ao projeto.
  **`postgres_data` é o mais crítico**: guarda a chave privada
  da CA local de gestão de certificados (tabela `ca`, coluna
  `chave_pem`, em `services/backend/certificates.go`) — apagar/recriar
  esse volume invalida a CA e todos os certificados já emitidos e
  confiados nos dispositivos do usuário, exatamente como apagar
  `cert_proxy_data` fazia com o antigo `cert-proxy` (removido).
  `cert_proxy_data` ainda existe só como fonte de import único
  da CA legada (`loadOrImportCA`, montado somente leitura em
  `backend`) — não apague, mesmo que pareça obsoleto.
- **Nunca commite `.env`** (contém `WIFI_PASSWORD`, `ADMIN_PASSWORD` e
  outros dados específicos da máquina). Se precisar documentar uma
  variável nova, edite `.env.example`, nunca `.env`.
- Se uma tarefa pedir para "testar" mudanças em
  `services/worker/hotspot/entrypoint.sh` ou nos scripts de rede,
  prefira validação estática (`bash -n`, leitura cuidadosa da lógica) a
  rodar de verdade — rodar de verdade exige acesso à placa Wi-Fi real e
  derruba a rede do usuário durante o teste.
- **`services/worker` é o único serviço com acesso root-equivalente ao
  host** (`/var/run/docker.sock`, `network_mode: host`,
  NetworkManager). Nunca proponha montar `docker.sock` em `backend` ou
  `frontend`, nem adicionar um endpoint de "exec arbitrário" na API
  interna do worker — toda ação nova ali deve ser uma rota específica
  e validada, igual às existentes.

## Convenções do projeto

- Scripts shell começam com `set -euo pipefail` (bash) ou `set -eu`
  (`sh`/`services/worker/dns/docker-entrypoint.sh`), falham alto com
  mensagem clara e `exit 1` em vez de continuar em estado
  inconsistente.
- Logs e comentários são em **português**, no padrão
  `[nome-do-servico] mensagem` (veja `log()` em cada entrypoint).
  Siga esse padrão em código novo/alterado neste repo, inclusive nos
  logs Go de `services/backend` e `services/worker/controller`.
- Variáveis de ambiente novas: sempre com valor padrão sensato via
  `${VAR:-padrao}` quando fizer sentido, documentadas em
  `.env.example` (com comentário explicando obrigatoriedade e efeito)
  e, se alterarem comportamento de negócio, descritas em `RULE.md`.
- Go em `services/worker/controller` não tem dependências externas
  (`go.mod` sem `require`) — mantenha assim a menos que haja motivo forte
  para introduzir uma dependência (por isso a autenticação do backend usa
  HMAC-SHA256 da biblioteca padrão em vez de bcrypt/JWT de terceiros).
  `services/backend`
  e `services/worker/dns` são a exceção deliberada: `backend` tem
  `github.com/jackc/pgx/v5` (Postgres) e `go.mongodb.org/mongo-driver/v2`
  (Mongo); `services/worker/dns` (dns-provider) tem `github.com/jackc/pgx/v5`
  (Postgres), `github.com/redis/go-redis/v9` (Redis) e `github.com/miekg/dns`
  (protocolo DNS — a mesma biblioteca em que o CoreDNS é construído).
  Justificativa em todos os casos: não há alternativa razoável de
  stdlib para esses protocolos de rede — não trate isso como
  precedente para adicionar outras dependências sem motivo forte
  equivalente.
- **Nomes de arquivos, funções, variáveis, atributos/campos JSON e
  colunas SQL são sempre em inglês** (ex.: `certificates.go`,
  `loadOrImportCA`, `requireSession`, campo JSON `domain`, coluna SQL
  `common_name`). **Logs e comentários continuam em português**, no
  padrão `[nome-do-servico] mensagem` (ver item acima) — essa é a
  única exceção: identificadores em código são a única coisa em
  inglês, texto voltado a humanos (logs, comentários, mensagens de
  erro, UI do frontend) continua em português. Ao criar/editar
  qualquer arquivo, siga esse padrão em vez de misturar os dois
  idiomas nos nomes.
- `services/frontend` segue o mesmo boilerplate `vite_react_shadcn_ts`
  usado nos outros projetos do usuário: Vite + React + TypeScript +
  Tailwind + shadcn/ui (estilo `default`, cor base `slate`) +
  TanStack Query + react-hook-form + zod. Mantenha esse padrão em
  telas novas em vez de introduzir outra biblioteca de UI/data
  fetching.
- **Toda alteração de UI em `services/frontend` deve considerar
  responsividade (mobile/tablet/desktop), não só desktop.** O painel é
  operado tanto do computador quanto do celular (ex.: operador
  ajustando o hotspot pelo Wi-Fi do próprio celular). Breakpoints são
  os padrões do Tailwind (`sm`=640px, `md`=768px, `lg`=1024px,
  `xl`=1280px, ver `tailwind.config.js`), e o app já segue estes
  padrões — reuse-os em vez de inventar outros:
  - Grids de cartões/formulário: `grid gap-4 sm:grid-cols-2` (e
    variações `md:`/`lg:` para mais colunas), nunca `grid-cols-N` fixo
    sem variante responsiva.
  - Listas de abas (`TabsList`): `grid h-auto w-full grid-cols-N
    sm:inline-grid sm:w-auto` (ver `HotspotTabsList.tsx`, `Dns.tsx`),
    para não estourar/cortar em telas pequenas.
  - Linhas de cabeçalho/ações lado a lado: `flex flex-col gap-4
    sm:flex-row sm:items-center sm:justify-between` (empilha no
    celular, vira linha no desktop).
  - Tabelas densas (`<Table>` de `components/ui/table.tsx`, que já
    rola horizontalmente via `overflow-auto`): oculte colunas
    secundárias no celular com `hidden sm:table-cell` /
    `hidden md:table-cell` no `TableHead`+`TableCell` correspondentes
    (mesmo índice de coluna nos dois), e reduza texto de botão de ação
    a só ícone abaixo de `sm` envolvendo o rótulo em `<span
    className="hidden sm:inline">` (mantendo `aria-label` no
    `Button` para acessibilidade).
  - Diálogos (`components/ui/dialog.tsx`): `DialogContent` já é
    `w-full max-w-lg` por padrão — ao sobrescrever a largura, use
    sempre `sm:max-w-*` (nunca `max-w-*` sem o prefixo `sm:`), senão o
    diálogo passa da largura da tela no celular.
  - Sidebar/nav (`Sidebar.tsx`/`MobileNav.tsx`) já resolvem
    desktop-vs-mobile (sidebar fixa `sm:flex` + drawer `MobileNav`
    abaixo de `sm`) — não duplique essa lógica em telas novas, reuse o
    `AppLayout` existente.
- Não introduza abstrações, flags ou configuração especulativa para
  cenários que este stack não tem hoje (é uma instalação única, não um
  produto multi-tenant).
- Migrations SQL (`services/migration/prisma/migrations/*/migration.sql`)
  são sempre **idempotentes**: `CREATE TABLE IF NOT EXISTS nome_tabela ()`
  sem declarar nenhuma coluna ali (nem `id`), seguido de
  `ALTER TABLE nome_tabela ADD COLUMN IF NOT EXISTS nome_coluna ...`
  para cada coluna, incluindo `id`; índices sempre com
  `CREATE INDEX IF NOT EXISTS`. Constraints que não suportam
  `IF NOT EXISTS` nativamente no Postgres (ex.: `PRIMARY KEY`,
  `UNIQUE`) vão inline na própria cláusula `ADD COLUMN` (que já é
  protegida por `IF NOT EXISTS`), nunca como `ALTER TABLE ADD CONSTRAINT`
  solto. Veja `services/migration/prisma/migrations/20260702000000_init_certificates/migration.sql`
  como referência.
- **Nenhum arquivo de implementação deve passar de ~200 linhas.** Ao
  editar ou criar um arquivo, se ele ultrapassar esse limite, refatore
  antes de considerar a tarefa concluída — extraia funções/componentes
  para arquivos novos em vez de deixar crescer um único arquivo.
- **Separação de responsabilidade é prioridade**: cada arquivo/função/
  componente deve ter um propósito único e claro. Os três serviços Go
  seguem o layout `cmd/<serviço>/main.go` (só o wiring) + pacotes
  `internal/` por domínio — `services/backend/internal/{auth,audit,
  workerapi,dns,cert,hotspot,setup,platform/{config,db}}`,
  `services/worker/controller/internal/{compose,network,shaping,peer,
  hotspot,trust}`, `services/worker/dns/internal/{core,config,store,cache,
  zones,discover,dnsserver,nginx,netdetect}` — e no frontend um
  componente/página por tela. Esse é o modelo a seguir para código novo.
  **Em Go um diretório é um pacote**: símbolos usados por outro pacote
  precisam ser exportados (maiúscula) e as dependências entre pacotes
  `internal/` têm que formar um DAG (sem ciclos de import).
- **Priorize criar funções/componentes reutilizáveis** em vez de
  duplicar a mesma lógica em arquivos diferentes: antes de escrever
  algo novo, procure se já existe uma função/componente/hook
  equivalente no repo (ex.: `requireSession` para proteger rotas,
  `api.get/post/patch/del` no frontend, os componentes em
  `services/frontend/src/components/ui/`) e reuse-o em vez de
  reimplementar.

## Como validar mudanças

- Shell scripts: `bash -n <arquivo>` (sintaxe) no mínimo; para
  mudanças de lógica, releia manualmente o fluxo de
  `services/worker/hotspot/entrypoint.sh` — não há suíte de testes
  automatizada neste repo. `services/worker/dns` não é mais shell
  script (ver abaixo).
- Go: `go build ./... && go vet ./...` dentro de cada módulo
  (`services/worker/controller`, `services/backend`,
  `services/worker/dns`). Para `services/backend` e
  `services/worker/dns`, rode `go mod tidy && go mod download` antes —
  ambos têm dependências reais (pgx e outras, ver "Convenções do
  projeto" acima).
- Frontend: `cd services/frontend && npm run build` (roda `tsc -b` +
  `vite build`).
- Migration: `cd services/migration && npm install && npx prisma validate`
  valida `prisma/schema.prisma` sem precisar de um Postgres vivo.
- Compose: `docker compose config` para validar sintaxe/interpolação de
  variáveis sem subir nada.
- Antes de considerar uma tarefa em
  `services/worker/hotspot/entrypoint.sh` concluída, confirme que as
  regras de `RULE.md` (seleção de canal, banda, variáveis obrigatórias)
  continuam válidas — é fácil quebrar a lógica de fallback
  silenciosamente.

## Estrutura rápida

```
docker-compose.yml    # orquestra todos os serviços
.env.example           # template documentado de variáveis (versionado)
.env                    # valores reais da máquina (git-ignored)
services/
  frontend/               # React (Vite/shadcn) - UI do painel de gestão
  backend/                # Go - API do painel; cmd/backend + internal/{auth,audit,workerapi,dns,cert,hotspot,setup,platform}
  migration/              # Node/TS + Prisma - aplica o schema do Postgres e encerra
  worker/
    controller/             # Go - único com docker.sock/NetworkManager/host; cmd/controller + internal/{compose,network,shaping,peer,hotspot,trust}
    hotspot/                # cria o AP Wi-Fi (create_ap) - shell, não Go
    dns/                    # Go - DNS split-horizon próprio (3 views); cmd/dns-provider + internal/{core,config,store,cache,zones,discover,dnsserver,nginx,netdetect}
```
Cada serviço Go compila o binário a partir de `cmd/<serviço>` (ver
`Dockerfile`: `go build ... ./cmd/<serviço>`). Referências a arquivos Go
neste documento e em `RULE.md` citam o nome do arquivo (ex.:
`certificates.go`, `hotspot_config_store.go`); o caminho completo agora
inclui o pacote `internal/` correspondente (ex.:
`services/backend/internal/cert/certificates.go`).
`postgres`, `mongo`, `minio` e `redis` são imagens oficiais
configuradas só no `docker-compose.yml`/`.env`, sem pasta própria em
`services/`.
