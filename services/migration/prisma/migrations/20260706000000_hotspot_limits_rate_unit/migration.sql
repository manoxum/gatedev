-- Migration idempotente que adiciona unidade aos limites de taxa do
-- hotspot, que antes eram sempre Mbps implicito. Seis unidades: bits
-- por segundo (kbit/mbit/gbit) e bytes por segundo (kbyte/mbyte/
-- gbyte) - o worker traduz kbyte/mbyte/gbyte para os sufixos
-- kbps/mbps/gbps que o tc usa para bytes/s (nome confuso do tc, mas e
-- assim que ele distingue de kbit/mbit/gbit). Renomeia as colunas
-- *_mbps para *_value (o numero digitado pelo admin) e adiciona uma
-- coluna *_unit irma para cada uma - linhas existentes (Mbps) viram
-- unit='mbit' por padrao, preservando o valor/comportamento antigo.
-- Ver services/backend/hotspot_limits.go e
-- services/worker/controller/shaping_tc.go.

DO $$
BEGIN
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_global_limits' AND column_name = 'download_rate_mbps'
	) THEN
		ALTER TABLE "hotspot_global_limits" RENAME COLUMN "download_rate_mbps" TO "download_rate_value";
	END IF;
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_global_limits' AND column_name = 'upload_rate_mbps'
	) THEN
		ALTER TABLE "hotspot_global_limits" RENAME COLUMN "upload_rate_mbps" TO "upload_rate_value";
	END IF;
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_global_limits' AND column_name = 'quota_throttle_download_mbps'
	) THEN
		ALTER TABLE "hotspot_global_limits" RENAME COLUMN "quota_throttle_download_mbps" TO "quota_throttle_download_value";
	END IF;
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_global_limits' AND column_name = 'quota_throttle_upload_mbps'
	) THEN
		ALTER TABLE "hotspot_global_limits" RENAME COLUMN "quota_throttle_upload_mbps" TO "quota_throttle_upload_value";
	END IF;

	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_device_limits' AND column_name = 'download_rate_mbps'
	) THEN
		ALTER TABLE "hotspot_device_limits" RENAME COLUMN "download_rate_mbps" TO "download_rate_value";
	END IF;
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_device_limits' AND column_name = 'upload_rate_mbps'
	) THEN
		ALTER TABLE "hotspot_device_limits" RENAME COLUMN "upload_rate_mbps" TO "upload_rate_value";
	END IF;
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_device_limits' AND column_name = 'quota_throttle_download_mbps'
	) THEN
		ALTER TABLE "hotspot_device_limits" RENAME COLUMN "quota_throttle_download_mbps" TO "quota_throttle_download_value";
	END IF;
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'hotspot_device_limits' AND column_name = 'quota_throttle_upload_mbps'
	) THEN
		ALTER TABLE "hotspot_device_limits" RENAME COLUMN "quota_throttle_upload_mbps" TO "quota_throttle_upload_value";
	END IF;
END $$;

ALTER TABLE "hotspot_global_limits" ADD COLUMN IF NOT EXISTS "download_rate_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("download_rate_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_global_limits" ADD COLUMN IF NOT EXISTS "upload_rate_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("upload_rate_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_global_limits" ADD COLUMN IF NOT EXISTS "quota_throttle_download_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("quota_throttle_download_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_global_limits" ADD COLUMN IF NOT EXISTS "quota_throttle_upload_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("quota_throttle_upload_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));

ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "download_rate_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("download_rate_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "upload_rate_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("upload_rate_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "quota_throttle_download_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("quota_throttle_download_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "quota_throttle_upload_unit" TEXT NOT NULL DEFAULT 'mbit' CHECK ("quota_throttle_upload_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
