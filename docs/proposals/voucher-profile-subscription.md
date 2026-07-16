# Proposta: vouchers vinculados a perfil, com subscrição por prazo

- **Status**: proposta (aguardando aprovação)
- **Data**: 2026-07-16
- **Áreas afetadas**: `services/backend` (vouchers, perfis, portal,
  reconciliação), `services/migration` (schema), `services/frontend`
  (emissão de vouchers, portal de autoatendimento), `RULE.md`

## 1. Contexto — como funciona hoje

- **Voucher é "solto"**: um código com valor fixo em bytes
  (`hotspot_vouchers.amount_bytes`), emitido em lote
  (`hotspot_voucher_batches`) e resgatado uma única vez pelo próprio
  dispositivo no portal (`POST /api/hotspot/portal/vouchers/redeem`,
  MAC resolvido no servidor pelo IP de origem). O resgate
  (`redeemVoucher`, `hotspot_vouchers.go`) credita o saldo e **força**
  o dispositivo para `limit_type = "credit"` com `configured = true` —
  a partir daí o perfil vinculado deixa de influenciar aquele MAC.
- **Perfil** (`hotspot_profiles`) é um bundle reutilizável de limites +
  política de crédito. O vínculo dispositivo→perfil vive em
  `hotspot_device_info.profile_id` e é resolvido por
  `effectiveDeviceLimits` (`hotspot_profiles_apply.go`). Não existe
  noção de "subscrição com prazo": o vínculo dura até o admin trocar.
- **Não há validade**: um voucher emitido fica `active` para sempre até
  ser resgatado ou revogado.

## 2. Objetivos

1. **Voucher vinculado a um perfil**: todo voucher novo pertence a um
   perfil; resgatá-lo **inscreve o dispositivo naquele perfil** (em vez
   de só creditar saldo e desligar o dispositivo do sistema de perfis).
2. **Subscrição com prazo**: o voucher pode carregar uma duração de
   subscrição (ex.: 7 dias, 1 mês). Ao expirar, o dispositivo volta
   automaticamente ao perfil "Padrão".
3. **Validade de ativação**: na emissão, o admin pode definir até
   quando o voucher pode ser resgatado (ex.: "válido para ativação até
   31/08"). Depois disso o código fica inutilizável.
4. **Conflito de perfil resolvido com consentimento**: se um
   dispositivo já inscrito no perfil A resgata um voucher do perfil B,
   o portal pede confirmação explícita antes de trocar — o código nunca
   é consumido sem a troca acontecer.

## 3. Modelo proposto

### 3.1 Conceitos

```
Lote de vouchers ──► perfil alvo (profile_id)
                 ──► validade de ativação (activation_expires_at, opcional)
                 ──► duração da subscrição (value + unit, opcional = sem prazo)
                 ──► valor em bytes (amount_bytes, opcional — só faz
                     sentido se o perfil alvo for tipo "credit")

Resgate ──► cria/renova a SUBSCRIÇÃO do MAC no perfil do voucher
        ──► hotspot_device_info.profile_id continua sendo o ponteiro
            efetivo (effectiveDeviceLimits NÃO muda)
        ──► se o perfil é "credit" e o voucher tem valor, credita saldo
```

A subscrição é uma entidade própria (nova tabela
`hotspot_device_subscriptions`) que **comanda** o ponteiro
`hotspot_device_info.profile_id`, nunca o substitui. Isso mantém
`effectiveDeviceLimits`, `syncDeviceCreditFromProfile` e todo o
shaping intocados — eles continuam lendo só o ponteiro.

### 3.2 Schema (nova migration, convenção idempotente do repo)

```sql
-- Lote ganha o perfil alvo e os prazos (NULL = comportamento legado)
ALTER TABLE hotspot_voucher_batches ADD COLUMN IF NOT EXISTS profile_id UUID;
ALTER TABLE hotspot_voucher_batches ADD COLUMN IF NOT EXISTS activation_expires_at TIMESTAMPTZ;
ALTER TABLE hotspot_voucher_batches ADD COLUMN IF NOT EXISTS subscription_duration_value INT;
ALTER TABLE hotspot_voucher_batches ADD COLUMN IF NOT EXISTS subscription_duration_unit TEXT NOT NULL DEFAULT 'day';

-- Voucher denormaliza o que o resgate precisa (evita JOIN no caminho
-- quente e segue o padrão do repo de colunas FK simples, sem @relation)
ALTER TABLE hotspot_vouchers ADD COLUMN IF NOT EXISTS profile_id UUID;
ALTER TABLE hotspot_vouchers ADD COLUMN IF NOT EXISTS activation_expires_at TIMESTAMPTZ;

-- Subscrição por dispositivo
CREATE TABLE IF NOT EXISTS hotspot_device_subscriptions ();
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY;
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS mac_address TEXT NOT NULL;
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS profile_id UUID NOT NULL;
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS voucher_code TEXT;
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS ended_at TIMESTAMPTZ;
ALTER TABLE hotspot_device_subscriptions ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

-- No máximo 1 subscrição ativa por MAC
CREATE UNIQUE INDEX IF NOT EXISTS hotspot_device_subscriptions_active_mac
  ON hotspot_device_subscriptions (mac_address) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS hotspot_device_subscriptions_expiry
  ON hotspot_device_subscriptions (expires_at) WHERE status = 'active';
```

Semântica dos campos:

- `subscription_duration_value/unit` (`hour`/`day`/`week`/`month`,
  padrão `day`): `value` NULL = subscrição **sem prazo** (permanente
  até troca manual/novo voucher). Mesmo padrão valor+unidade já usado
  em taxas/cotas.
- `activation_expires_at` NULL = voucher sem validade de ativação
  (comportamento atual).
- `hotspot_vouchers.profile_id` NULL = **voucher legado** (emitido
  antes desta mudança): resgate mantém o comportamento atual
  (credita saldo, força `credit` + `configured = true`). Nenhuma linha
  existente precisa de backfill.
- `status` da subscrição: `active` → `expired` (prazo venceu),
  `replaced` (novo voucher assumiu), `cancelled` (admin trocou o
  perfil manualmente). Linhas encerradas ficam como histórico.

### 3.3 Emissão (`POST /api/hotspot/vouchers`)

Request ganha campos novos:

```json
{
  "profileId": "uuid-do-perfil",           // obrigatório em vouchers novos
  "amountBytes": 5368709120,               // opcional; só aceito se o perfil alvo for "credit"
  "amountUnit": "gbyte",
  "quantity": 10,
  "activationValidityDays": 30,            // opcional; servidor grava activation_expires_at = now() + N dias
  "subscriptionDurationValue": 7,          // opcional; NULL = sem prazo
  "subscriptionDurationUnit": "day",
  "note": "Promoção agosto"
}
```

Validações no backend (`hotspot_voucher_batches.go`):

- `profileId` deve existir; **rejeitar perfil `custom`** como alvo de
  voucher (um voucher precisa entregar uma política concreta —
  `custom` delega ao dispositivo e não faz sentido como "produto").
- `amountBytes > 0` só é aceito se o perfil alvo tiver
  `limit_type = "credit"`; para `quota`/`unlimited` o campo é
  rejeitado com 400 (evita emitir voucher com promessa de bytes que
  nunca seriam usados).
- `activationValidityDays` e `subscriptionDurationValue`, quando
  presentes, devem ser positivos.

### 3.4 Resgate (`POST /api/hotspot/portal/vouchers/redeem`)

O `redeemVoucher` atual vira duas variantes dentro da mesma transação
(arquivo novo `hotspot_voucher_redeem.go`, mantendo cada arquivo
< ~200 linhas):

1. **Claim do código** — o `UPDATE ... WHERE status = 'active'` atual
   ganha a validade: `AND (activation_expires_at IS NULL OR
   activation_expires_at > CURRENT_TIMESTAMP)`. Código vencido devolve
   o mesmo erro genérico `errHotspotVoucherInvalid` (não vira oráculo).
2. **Voucher legado** (`profile_id` NULL): fluxo atual inalterado.
3. **Voucher de perfil**:
   - Detecta o perfil efetivo atual do MAC (`deviceProfileID`).
   - **Conflito** (ver §3.5): se o perfil atual difere do alvo e não é
     o "Padrão", e o request não trouxe `confirmSwitch: true`, a
     transação é **abortada com rollback** (o código continua
     `active`) e o backend responde `409` com payload estruturado.
   - Cria a subscrição: encerra a ativa anterior (`replaced`), insere
     nova com `expires_at = now() + duração` (NULL se sem prazo) e
     grava `hotspot_device_info.profile_id = profile_id` via
     `assignDeviceProfile`.
   - **Renovação** (voucher do MESMO perfil já ativo): em vez de
     substituir, estende — `expires_at = GREATEST(expires_at, now()) +
     duração` (soma o tempo restante; sem prazo no voucher novo ⇒
     `expires_at = NULL`). Sem confirmação nenhuma: renovar não é
     conflito.
   - Se o perfil alvo é `credit` e o voucher tem `amount_bytes`,
     credita o saldo respeitando `plafond` (mesmo SQL de hoje) e grava
     `voucher_redemption` no extrato.
   - **Não** força override `limit_type = "credit"` nem
     `configured = true` — diferente do voucher legado, aqui o perfil
     deve continuar mandando no dispositivo (é o ponto da feature).
     `syncDeviceCreditFromProfile` com `configured = false` já herda a
     política de recarga do perfil sozinho.
   - Reaplica shaping ao vivo no MAC (`ensureDeviceShaping` +
     `syncDeviceCreditFromProfile`), mesmo espírito de
     `applyProfileShapingLive`.

### 3.5 Conflito: cliente do perfil A escaneia voucher do perfil B

Resgate em duas fases, com o código **nunca consumido sem a troca**:

```
Dispositivo (perfil A) ──POST redeem {code}──► backend
backend: perfil atual A ≠ alvo B, sem confirmSwitch
       ◄── 409 {
             "conflict": "profile_switch",
             "currentProfileName": "Residencial",
             "targetProfileName": "Premium",
             "currentExpiresAt": "2026-07-20T00:00:00Z",   // null = sem prazo
             "warnings": ["O saldo de crédito atual ficará dormente."]
           }
portal: mostra diálogo "Trocar de Residencial para Premium?
        Sua subscrição atual (válida até 20/07) será substituída."
usuário confirma ──POST redeem {code, confirmSwitch: true}──► backend
backend: consome o código, marca subscrição antiga 'replaced', ativa B
```

Regras:

- A confirmação é exigida sempre que o perfil efetivo atual difere do
  alvo **e** não é o "Padrão" — tanto faz se veio de voucher ou de
  atribuição manual do admin (o usuário do portal não sabe a origem;
  o que importa é que ele está prestes a perder uma condição vigente).
- Dispositivo no perfil "Padrão" (estado de fábrica): inscreve direto,
  sem diálogo.
- O `409` de conflito tem shape distinto do `409` de código inválido
  (campo `conflict` presente) para o portal saber qual UI mostrar.
- Saldo de crédito remanescente **não é zerado** na troca: fica na
  linha de `hotspot_device_credit` e volta a valer se o dispositivo
  retornar a um perfil `credit` (dormência é o comportamento que o
  `limitType` único já garante hoje — documentar, não codar).

### 3.6 Expiração da subscrição

Novo passo no loop de reconciliação existente
(`startHotspotReconciliationLoop`, `hotspot_reconcile.go`, ciclo de
15s), em arquivo novo `hotspot_subscriptions.go`:

1. `SELECT ... FROM hotspot_device_subscriptions WHERE status =
   'active' AND expires_at <= CURRENT_TIMESTAMP` (índice parcial já
   proposto acima).
2. Para cada linha: marca `expired` + `ended_at`, aponta o dispositivo
   de volta ao perfil "Padrão" (`assignDeviceProfile(mac,
   defaultProfileID)`), reaplica shaping/crédito ao vivo se o MAC
   estiver conectado e grava evento de auditoria
   `subscription_expired`.
3. Voltar ao "Padrão" (e não "ao perfil anterior") é deliberado:
   é o único estado sempre válido (o perfil anterior pode ter sido
   apagado) e é o mesmo estado de um dispositivo novo.

Se o "Padrão" for restritivo (ex.: `credit` sem saldo), o bloqueio por
crédito + portal cativo existentes já conduzem o usuário de volta ao
portal para resgatar outro voucher — nenhum mecanismo novo necessário.

Troca manual de perfil pelo admin
(`PATCH /api/hotspot/devices/{mac}/profile`) passa a encerrar a
subscrição ativa como `cancelled` — a atribuição manual vence, e a
tela do admin deve avisar quando houver subscrição ativa.

### 3.7 Validade de ativação — sem job novo

- **Enforcement** é 100% no claim do resgate (cláusula `WHERE`).
- **Exibição**: listagens calculam o status na leitura
  (`CASE WHEN status = 'active' AND activation_expires_at <= now()
  THEN 'expired' ...`) — sem job de background marcando linhas, sem
  estado novo para reconciliar. Contadores do lote
  (`active/redeemed/revoked`) ganham `expired` pelo mesmo `CASE`.

## 4. Mudanças por serviço

### `services/backend`

| Arquivo | Mudança |
|---|---|
| `hotspot_voucher_batches.go` | emissão: `profileId`, validade, duração + validações |
| `hotspot_vouchers.go` | listagem com perfil/validade; status `expired` computado |
| `hotspot_voucher_redeem.go` (novo) | resgate transacional: claim + subscrição + conflito + crédito |
| `hotspot_subscriptions.go` (novo) | store da subscrição + passo de expiração do loop |
| `hotspot_reconcile.go` | chama o passo de expiração no ciclo |
| `hotspot_portal.go` | `/portal/me` ganha `subscriptionExpiresAt`; redeem aceita `confirmSwitch` |
| `hotspot_profiles.go` | `PATCH .../profile` cancela subscrição ativa; `DELETE` de perfil rejeitado se houver voucher `active` ou subscrição ativa apontando pra ele |
| auditoria | `voucher_issued` ganha `profileId`; novos: `subscription_started`, `subscription_replaced`, `subscription_expired`, `subscription_cancelled` |

### `services/migration`

- Migration nova `2026xxxxxxxxxx_hotspot_voucher_profile_subscription`
  (SQL do §3.2, seguindo a referência de
  `20260702000000_init_certificates`), + models
  `HotspotDeviceSubscription` e colunas novas no `schema.prisma`.

### `services/frontend`

- `HotspotVoucherIssueForm`: select de perfil (obrigatório), campo de
  valor visível só para perfil `credit`, validade de ativação (dias) e
  duração da subscrição (valor + unidade) — grid responsivo
  `grid gap-4 sm:grid-cols-2` como já é padrão.
- `HotspotVouchersCard` / `HotspotVoucherBatchDetail` / PDF
  (`hotspot-voucher-pdf.ts`): coluna/rótulo do perfil, validade e
  duração impressos no cartão (coluna secundária oculta no mobile via
  `hidden sm:table-cell`).
- `Portal.tsx`: mostra perfil atual + "subscrição válida até …";
  diálogo de confirmação de troca alimentado pelo `409` estruturado
  (`DialogContent` padrão, largura só com `sm:max-w-*`).
- Detalhe do dispositivo (admin): badge da subscrição ativa
  (perfil, origem voucher, vencimento).

### `RULE.md`

- Seção "Perfis, vouchers e portal cativo": documentar voucher→perfil,
  validade de ativação, subscrição com prazo, política de conflito
  (confirmação em duas fases), expiração no loop de 15s e o fato de o
  resgate de voucher de perfil **não** setar mais
  `configured = true`.

## 5. Decisões tomadas (e alternativas descartadas)

1. **Duração no voucher, não no perfil.** O mesmo perfil "Premium"
   pode ser vendido como voucher de 7 dias e de 30 dias — a duração é
   atributo comercial da emissão. Alternativa (duração padrão no
   perfil, sobrescrevível na emissão) adia-se até haver necessidade
   real (evitar configuração especulativa, ver CLAUDE.md).
2. **Tabela de subscrição separada, ponteiro intacto.**
   `effectiveDeviceLimits` e todo o shaping continuam lendo só
   `hotspot_device_info.profile_id` — a subscrição só comanda esse
   ponteiro nas transições (resgate, expiração, cancelamento). Menor
   superfície de mudança no caminho quente.
3. **Conflito exige consentimento, nunca decide sozinho.** Substituir
   silenciosamente perderia tempo restante pago; recusar sempre
   obrigaria intervenção do admin em um fluxo que é de autoatendimento.
4. **Expiração volta ao "Padrão"**, não ao perfil anterior (§3.6).
5. **Vouchers legados continuam funcionando** com o comportamento
   antigo (`profile_id` NULL) — sem backfill, sem quebra.

## 6. Fases de implementação

1. **Fase 1 — schema + emissão**: migration, `schema.prisma`, emissão
   com perfil/validade/duração, listagens e PDF. (Sem mudança de
   comportamento no resgate ainda — vouchers novos sem resgate de
   perfil ficam retidos até a Fase 2, então as fases 1 e 2 devem ir
   juntas para produção.)
2. **Fase 2 — resgate + subscrição + conflito**: `redeemVoucher` novo,
   `409` estruturado, diálogo no portal, auditoria.
3. **Fase 3 — expiração + visibilidade**: passo no loop de
   reconciliação, cancelamento na troca manual, badge no admin,
   "válida até" no portal.
4. **Fase 4 — documentação**: RULE.md + `.env.example` (nenhuma
   variável nova prevista, confirmar ao final).

Validação por fase: `go build ./... && go vet ./...` no backend,
`npx prisma validate` na migration, `npm run build` no frontend —
nenhuma etapa exige subir o hotspot real.

## 7. Pontos em aberto (decidir antes da Fase 2)

1. Ao trocar de um perfil `credit` para outro perfil `credit` via
   voucher, o saldo remanescente **soma** com o valor do voucher novo
   (proposta atual) ou deveria ser zerado no momento da troca?
2. Voucher de perfil sem prazo (`subscription_duration_value` NULL)
   deve existir mesmo? Se todo voucher de perfil precisar de prazo,
   o campo vira obrigatório na emissão e o caso "sem prazo" some do
   portal. (Proposta atual: permitido, subscrição permanente.)
3. Revogar em massa um lote inteiro (`DELETE` por lote) ficou de fora
   de propósito — confirmar que a revogação individual atual basta.
