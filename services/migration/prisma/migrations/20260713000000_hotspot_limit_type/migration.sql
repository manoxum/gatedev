-- Tipo de limitacao unico e mutuamente exclusivo (ilimitado/credito/cota)
-- para hotspot_device_limits e hotspot_profiles - ver
-- services/backend/hotspot_device_limits.go. Cota vira 3 tetos
-- independentes (diario/semanal/mensal, podendo combinar os 3) em vez
-- de quota_bytes+quota_period unico. As colunas antigas (quota_bytes,
-- quota_period, quota_throttle_*, credit_enabled) ficam no banco sem
-- DROP (convencao deste repo, ver 20260710010000_hotspot_credit_history_reset
-- para o precedente de so invalidar dado via DELETE/UPDATE, nunca
-- remover coluna/tabela) - so perdem leitor/escritor Go para device/perfil.

ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "limit_type" TEXT NOT NULL DEFAULT 'unlimited' CHECK ("limit_type" IN ('unlimited','credit','quota'));
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "daily_quota_bytes" BIGINT;
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "weekly_quota_bytes" BIGINT;
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "monthly_quota_bytes" BIGINT;

ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "limit_type" TEXT NOT NULL DEFAULT 'unlimited' CHECK ("limit_type" IN ('unlimited','credit','quota'));
ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "daily_quota_bytes" BIGINT;
ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "weekly_quota_bytes" BIGINT;
ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "monthly_quota_bytes" BIGINT;

-- Backfill: cota primeiro, credito so pega quem sobrar com limit_type
-- ainda 'unlimited'. Decisao deliberada: uma linha com cota E credito
-- habilitados ao mesmo tempo (o cenario do bug de "cota para de
-- contabilizar", ver RULE.md) vira 'quota', nao 'credit' - e o
-- comportamento que resolve o bug a favor da intencao descrita pelo
-- admin. Idempotente: apos rodar uma vez, limit_type != 'unlimited'
-- nessas linhas, entao uma reaplicacao nao casa mais no WHERE.

UPDATE "hotspot_device_limits"
   SET limit_type = 'quota',
       daily_quota_bytes   = CASE WHEN quota_period = 'daily'   THEN quota_bytes END,
       weekly_quota_bytes  = CASE WHEN quota_period = 'weekly'  THEN quota_bytes END,
       monthly_quota_bytes = CASE WHEN quota_period = 'monthly' THEN quota_bytes END
 WHERE limit_type = 'unlimited' AND quota_bytes IS NOT NULL AND quota_period IS NOT NULL;

UPDATE "hotspot_device_limits" d
   SET limit_type = 'credit'
 WHERE d.limit_type = 'unlimited'
   AND EXISTS (
     SELECT 1 FROM "hotspot_device_credit" c
      WHERE c.mac_address = d.mac_address AND c.enabled = true
   );

UPDATE "hotspot_profiles"
   SET limit_type = 'quota',
       daily_quota_bytes   = CASE WHEN quota_period = 'daily'   THEN quota_bytes END,
       weekly_quota_bytes  = CASE WHEN quota_period = 'weekly'  THEN quota_bytes END,
       monthly_quota_bytes = CASE WHEN quota_period = 'monthly' THEN quota_bytes END
 WHERE limit_type = 'unlimited' AND quota_bytes IS NOT NULL AND quota_period IS NOT NULL;

UPDATE "hotspot_profiles"
   SET limit_type = 'credit'
 WHERE limit_type = 'unlimited' AND credit_enabled = true;
