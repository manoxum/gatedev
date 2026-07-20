-- Configuracao do dns-provider e do painel administrada pelo painel web.
-- Estes valores nao ficam mais em .env.main/.env.example; o dns-provider le
-- dns_config direto do banco quando inicia, e o backend le panel_config.
-- Mesmo padrao chave/valor ja usado por hotspot_config.

-- dns_config: DNS_LOCAL_TLDS, DOMAINS, DISCOVER_NODE_NAME,
-- DISCOVER_REMOTE_ROUTES. (DISCOVER_PORT continua sendo env: e porta de
-- infraestrutura, nao configuracao de negocio.)
CREATE TABLE IF NOT EXISTS "dns_config" ();

ALTER TABLE "dns_config" ADD COLUMN IF NOT EXISTS "key" TEXT PRIMARY KEY;
ALTER TABLE "dns_config" ADD COLUMN IF NOT EXISTS "value" TEXT NOT NULL;
ALTER TABLE "dns_config" ADD COLUMN IF NOT EXISTS "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

-- panel_config: CA_COMMON_NAME, NGINX_UI_USERNAME, NGINX_UI_PASSWORD.
-- NGINX_UI_URL continua env (endereco interno do container, nao editavel
-- pelo operador).
CREATE TABLE IF NOT EXISTS "panel_config" ();

ALTER TABLE "panel_config" ADD COLUMN IF NOT EXISTS "key" TEXT PRIMARY KEY;
ALTER TABLE "panel_config" ADD COLUMN IF NOT EXISTS "value" TEXT NOT NULL;
ALTER TABLE "panel_config" ADD COLUMN IF NOT EXISTS "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
