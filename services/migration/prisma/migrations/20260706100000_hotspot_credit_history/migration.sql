CREATE SEQUENCE IF NOT EXISTS hotspot_device_credit_history_id_seq;

-- CreateTable
-- Uma linha por mutacao de saldo (recarga manual, recarga automatica
-- ou debito de trafego) - permite reconstruir o extrato/conta corrente
-- de credito de cada dispositivo.
CREATE TABLE IF NOT EXISTS "hotspot_device_credit_history" ();

ALTER TABLE "hotspot_device_credit_history" ADD COLUMN IF NOT EXISTS "id" BIGINT PRIMARY KEY DEFAULT nextval('hotspot_device_credit_history_id_seq');
ALTER TABLE "hotspot_device_credit_history" ADD COLUMN IF NOT EXISTS "mac_address" TEXT NOT NULL;
ALTER TABLE "hotspot_device_credit_history" ADD COLUMN IF NOT EXISTS "entry_type" TEXT NOT NULL CHECK ("entry_type" IN ('manual_recharge','auto_recharge','debit'));
ALTER TABLE "hotspot_device_credit_history" ADD COLUMN IF NOT EXISTS "amount_bytes" BIGINT NOT NULL;
ALTER TABLE "hotspot_device_credit_history" ADD COLUMN IF NOT EXISTS "balance_after_bytes" BIGINT NOT NULL;
ALTER TABLE "hotspot_device_credit_history" ADD COLUMN IF NOT EXISTS "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE INDEX IF NOT EXISTS "hotspot_device_credit_history_mac_created_idx" ON "hotspot_device_credit_history" ("mac_address", "created_at" DESC);
