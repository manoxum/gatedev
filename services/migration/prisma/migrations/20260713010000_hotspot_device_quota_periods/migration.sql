-- Acumulador de uso do periodo corrente por (dispositivo, tipo de
-- periodo) - 1 linha por periodo efetivamente configurado (diario/
-- semanal/mensal), criada de forma preguicosa (mesmo padrao de
-- hotspot_device_credit/hotspot_device_traffic). "blocked" e bloqueio
-- rigido (nao throttle) ao estourar o teto daquele periodo - ver
-- services/backend/hotspot_device_quota_store.go.

CREATE SEQUENCE IF NOT EXISTS hotspot_device_quota_periods_id_seq;

CREATE TABLE IF NOT EXISTS "hotspot_device_quota_periods" ();

ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "id" BIGINT PRIMARY KEY DEFAULT nextval('hotspot_device_quota_periods_id_seq');
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "mac_address" TEXT NOT NULL;
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "period_type" TEXT NOT NULL CHECK ("period_type" IN ('daily','weekly','monthly'));
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "download_bytes" BIGINT NOT NULL DEFAULT 0;
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "upload_bytes" BIGINT NOT NULL DEFAULT 0;
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "period_start" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "period_end" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "blocked" BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE "hotspot_device_quota_periods" ADD COLUMN IF NOT EXISTS "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE UNIQUE INDEX IF NOT EXISTS "hotspot_device_quota_periods_mac_period_idx" ON "hotspot_device_quota_periods" ("mac_address", "period_type");
