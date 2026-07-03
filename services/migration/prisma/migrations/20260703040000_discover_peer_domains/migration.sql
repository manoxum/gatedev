-- Dominios anunciados pelo peer no ultimo scan manual.

ALTER TABLE "discover_peers"
  ADD COLUMN IF NOT EXISTS "domains" TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];
