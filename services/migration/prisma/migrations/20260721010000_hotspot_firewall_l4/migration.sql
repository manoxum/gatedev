-- Migration idempotente: transforma as regras de comunicação de clientes
-- (hotspot_comm_rules) na base de um firewall por zonas. Esta camada
-- adiciona a zona e o casamento L4 (protocolo + portas + host remoto).
-- Tudo com default retrocompatível: uma regra antiga vira zona 'clients'
-- protocolo 'any' sem portas - exatamente o comportamento atual.

-- zone: qual caminho de tráfego a regra governa.
--  'clients' = tráfego entre clientes do hotspot (chain BINDNET-ISOLATION);
--  'wan'     = cliente -> internet (saída pelo uplink) [camada seguinte];
--  'local'   = cliente -> gateway/serviços do host (INPUT) [camada seguinte].
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "zone" TEXT NOT NULL DEFAULT 'clients' CHECK ("zone" IN ('clients','wan','local'));

-- protocol: 'any' (todos), 'tcp', 'udp' ou 'icmp'. Portas só se aplicam a tcp/udp.
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "protocol" TEXT NOT NULL DEFAULT 'any' CHECK ("protocol" IN ('any','tcp','udp','icmp'));

-- dst_ports: lista de portas/intervalos de destino ("80,443,8000-8100").
-- NULL/'' = qualquer porta. Validado no backend (formato) antes de gravar.
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "dst_ports" TEXT;

-- dst_host: para a zona 'wan', restringe o destino externo a um IP/CIDR
-- ("1.2.3.4" ou "10.0.0.0/24"). NULL = qualquer destino. Não usado nas
-- zonas 'clients'/'local' (o destino ali é implícito).
ALTER TABLE "hotspot_comm_rules" ADD COLUMN IF NOT EXISTS "dst_host" TEXT;

CREATE INDEX IF NOT EXISTS "hotspot_comm_rules_zone_idx" ON "hotspot_comm_rules" ("zone");
