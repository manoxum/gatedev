CREATE SEQUENCE IF NOT EXISTS hotspot_device_sessions_id_seq;

-- CreateTable
-- Uma linha por ciclo conectado/desconectado de um dispositivo no
-- hotspot: nasce quando o MAC aparece na lista de clientes ao vivo
-- (reconcileHotspotOnce) e fecha (ended_at) quando some dela. total_bytes
-- e incrementado a cada debito de credito registrado no Mongo
-- (hotspot_credit_debits) - o extrato bruto pode ser podado por TTL, mas
-- o total consolidado da sessao continua no Postgres para conferencia.
CREATE TABLE IF NOT EXISTS "hotspot_device_sessions" ();

ALTER TABLE "hotspot_device_sessions" ADD COLUMN IF NOT EXISTS "id" BIGINT PRIMARY KEY DEFAULT nextval('hotspot_device_sessions_id_seq');
ALTER TABLE "hotspot_device_sessions" ADD COLUMN IF NOT EXISTS "mac_address" TEXT NOT NULL;
ALTER TABLE "hotspot_device_sessions" ADD COLUMN IF NOT EXISTS "started_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE "hotspot_device_sessions" ADD COLUMN IF NOT EXISTS "ended_at" TIMESTAMPTZ;
ALTER TABLE "hotspot_device_sessions" ADD COLUMN IF NOT EXISTS "total_bytes" BIGINT NOT NULL DEFAULT 0;
ALTER TABLE "hotspot_device_sessions" ADD COLUMN IF NOT EXISTS "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE INDEX IF NOT EXISTS "hotspot_device_sessions_mac_started_idx" ON "hotspot_device_sessions" ("mac_address", "started_at" DESC);

-- So uma sessao aberta (ended_at IS NULL) por dispositivo por vez -
-- garante que o upsert de abertura (ON CONFLICT nesse indice) seja
-- idempotente entre ciclos de reconciliacao.
CREATE UNIQUE INDEX IF NOT EXISTS "hotspot_device_sessions_open_mac_idx" ON "hotspot_device_sessions" ("mac_address") WHERE "ended_at" IS NULL;
