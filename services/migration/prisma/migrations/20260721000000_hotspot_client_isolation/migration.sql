-- Migration idempotente para o isolamento de clientes do hotspot
-- (services/backend/internal/hotspot/hotspot_isolation.go): interruptor
-- geral CLIENT_ISOLATION vive em hotspot_config (chave/valor, sem
-- migration), aqui ficam a permissao de comunicacao interna por perfil
-- e as regras de comunicacao entre perfis/dispositivos.

-- Comunicacao interna do perfil: com o isolamento ligado, decide se os
-- clientes DESTE perfil comunicam entre si. Default false de proposito
-- (postura confirmada com o admin: ligar o isolamento bloqueia tudo ate
-- se configurar o contrario).
ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "allow_internal_communication" BOOLEAN NOT NULL DEFAULT false;

-- Regras de comunicacao: source/target polimorficos (device = MAC
-- normalizado aa:bb:cc:dd:ee:ff, profile = UUID de hotspot_profiles,
-- any = curinga so no target, target_ref NULL nesse caso). Sem FK de
-- banco de proposito (refs polimorficas); a remocao de regras de um
-- perfil apagado e feita na aplicacao (DeleteCommRulesForProfile em
-- services/backend/internal/hotspot/store/hotspot_comm_rules.go).
-- direction 'to' = source pode INICIAR trafego para target (respostas
-- voltam via conntrack); 'both' = os dois podem iniciar.
CREATE TABLE IF NOT EXISTS "hotspot_comm_rules" ();

ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "id" UUID PRIMARY KEY DEFAULT gen_random_uuid();
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "source_kind" TEXT NOT NULL CHECK ("source_kind" IN ('device','profile'));
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "source_ref" TEXT NOT NULL;
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "target_kind" TEXT NOT NULL CHECK ("target_kind" IN ('device','profile','any'));
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "target_ref" TEXT;
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "direction" TEXT NOT NULL DEFAULT 'both' CHECK ("direction" IN ('to','both'));
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "action" TEXT NOT NULL CHECK ("action" IN ('allow','deny'));
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "enabled" BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "note" TEXT;
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE INDEX IF NOT EXISTS "hotspot_comm_rules_source_idx" ON "hotspot_comm_rules" ("source_kind", "source_ref");
CREATE INDEX IF NOT EXISTS "hotspot_comm_rules_target_idx" ON "hotspot_comm_rules" ("target_kind", "target_ref");
