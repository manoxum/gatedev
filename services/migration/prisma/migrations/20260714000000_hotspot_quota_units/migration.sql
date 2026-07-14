-- Adiciona coluna de unidade para cada campo de cota que ainda so
-- guardava bytes crus (daily/weekly/monthlyQuotaBytes em
-- hotspot_profiles/hotspot_device_limits, quotaBytes em
-- hotspot_global_limits) - sem isso o backend nao tinha onde persistir
-- a unidade (MB/GB/...) que o admin escolheu no formulario, e o
-- frontend tinha que reconstruir um valor a partir dos bytes sempre
-- assumindo 'gbyte' ao reidratar o formulario (ver
-- limitsToFormValues/hotspot-device-limits-schema.ts e
-- hotspot-limits-schema.ts), fazendo uma cota salva em MB reaparecer
-- arredondada/errada em GB. Mesmo padrao de tipo/CHECK ja usado pelas
-- colunas de taxa (download_rate_unit etc., ver migration
-- 20260706000000_hotspot_limits_rate_unit) - default 'gbyte' (nao
-- 'mbit' como as colunas de taxa) porque cota e uma quantidade de
-- dados, nao uma taxa, e bate com o fallback que o frontend ja usava.

ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "daily_quota_unit" TEXT NOT NULL DEFAULT 'gbyte' CHECK ("daily_quota_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "weekly_quota_unit" TEXT NOT NULL DEFAULT 'gbyte' CHECK ("weekly_quota_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_profiles" ADD COLUMN IF NOT EXISTS "monthly_quota_unit" TEXT NOT NULL DEFAULT 'gbyte' CHECK ("monthly_quota_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));

ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "daily_quota_unit" TEXT NOT NULL DEFAULT 'gbyte' CHECK ("daily_quota_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "weekly_quota_unit" TEXT NOT NULL DEFAULT 'gbyte' CHECK ("weekly_quota_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
ALTER TABLE "hotspot_device_limits" ADD COLUMN IF NOT EXISTS "monthly_quota_unit" TEXT NOT NULL DEFAULT 'gbyte' CHECK ("monthly_quota_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));

ALTER TABLE "hotspot_global_limits" ADD COLUMN IF NOT EXISTS "quota_unit" TEXT NOT NULL DEFAULT 'gbyte' CHECK ("quota_unit" IN ('kbit','mbit','gbit','kbyte','mbyte','gbyte'));
