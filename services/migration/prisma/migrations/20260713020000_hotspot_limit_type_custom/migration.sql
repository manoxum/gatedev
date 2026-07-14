-- Adiciona o 4o valor 'custom' ao CHECK de limit_type em
-- hotspot_device_limits/hotspot_profiles (ver services/backend/hotspot_device_limits.go).
-- 'custom' so tem sentido em perfil: significa "este perfil nao aplica
-- nenhum limite - o dispositivo que herdar este perfil e quem define a
-- propria estrategia" (ver effectiveDeviceLimits). O CHECK permite o
-- valor nas duas tabelas por simplicidade (constraint unica reaproveitada),
-- mas a API nunca aceita 'custom' como limitType de dispositivo (so de
-- perfil) - validado em Go, nao no banco.
--
-- Postgres nao tem "ALTER CONSTRAINT" para CHECK: idempotente via
-- DROP CONSTRAINT IF EXISTS + ADD CONSTRAINT (convergem sempre pro
-- mesmo estado final, mesmo espirito de ADD COLUMN IF NOT EXISTS usado
-- no resto das migrations deste repo). Nomes de constraint sao os
-- default do Postgres para CHECK inline sem nome explicito
-- ("<tabela>_<coluna>_check"), criados pela migration 20260713000000.

ALTER TABLE "hotspot_device_limits" DROP CONSTRAINT IF EXISTS "hotspot_device_limits_limit_type_check";
ALTER TABLE "hotspot_device_limits" ADD CONSTRAINT "hotspot_device_limits_limit_type_check" CHECK ("limit_type" IN ('unlimited','credit','quota','custom'));

ALTER TABLE "hotspot_profiles" DROP CONSTRAINT IF EXISTS "hotspot_profiles_limit_type_check";
ALTER TABLE "hotspot_profiles" ADD CONSTRAINT "hotspot_profiles_limit_type_check" CHECK ("limit_type" IN ('unlimited','credit','quota','custom'));
