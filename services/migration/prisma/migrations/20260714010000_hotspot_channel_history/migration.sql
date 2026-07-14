-- Historico de sucesso/falha por combinacao (placa Wi-Fi, modo, banda,
-- canal) do hotspot (services/worker/hotspot/history.sh) - permite
-- priorizar na proxima subida os candidatos que ja funcionaram antes,
-- em vez de sempre reavaliar do zero (varredura de interferencia +
-- tentativa e erro em todos os candidatos) toda vez que o hotspot
-- inicia. "mode" distingue 'same-interface' (Wi-Fi para Wi-Fi, AP+STA
-- concorrente na mesma placa - canal travado pela estacao, sem escolha
-- real de canal) de 'different-interface' (qualquer outra combinacao,
-- ex. Ethernet para Wi-Fi - canal escolhido livremente entre
-- candidatos).

CREATE SEQUENCE IF NOT EXISTS hotspot_channel_history_id_seq;

CREATE TABLE IF NOT EXISTS "hotspot_channel_history" ();

ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "id" BIGINT PRIMARY KEY DEFAULT nextval('hotspot_channel_history_id_seq');
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "wifi_interface" TEXT NOT NULL;
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "mode" TEXT NOT NULL CHECK ("mode" IN ('same-interface','different-interface'));
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "band" TEXT NOT NULL;
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "channel" INTEGER NOT NULL;
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "success_count" INTEGER NOT NULL DEFAULT 0;
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "failure_count" INTEGER NOT NULL DEFAULT 0;
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "last_result" TEXT;
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "last_attempt_at" TIMESTAMPTZ;
ALTER TABLE "hotspot_channel_history" ADD COLUMN IF NOT EXISTS "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE UNIQUE INDEX IF NOT EXISTS "hotspot_channel_history_lookup_idx" ON "hotspot_channel_history" ("wifi_interface", "mode", "band", "channel");
