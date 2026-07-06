-- Migration idempotente que adiciona um alias opcional e unico por
-- dispositivo do hotspot (definido pelo admin ao identificar o
-- dispositivo, distinto do device_name inferido automaticamente por
-- heuristica). TEXT UNIQUE sem NOT NULL: no Postgres, multiplos NULL
-- nao violam a constraint UNIQUE, entao dispositivos sem alias
-- definido convivem normalmente. Ver services/backend/hotspot_devices.go.
ALTER TABLE "hotspot_device_info" ADD COLUMN IF NOT EXISTS "alias" TEXT UNIQUE;
