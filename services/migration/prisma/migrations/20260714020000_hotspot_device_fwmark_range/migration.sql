-- Repara fwmarks de hotspot_device_traffic que estouraram os 16 bits
-- que o classid HTB do tc aceita (ver deviceClassID em
-- services/worker/controller/shaping_tc.go). O bug de origem
-- (services/backend/hotspot_traffic.go, ensureDeviceTrafficRow
-- fazendo "INSERT ... ON CONFLICT DO UPDATE" a cada 1s por
-- dispositivo conectado) consumia um valor de
-- hotspot_device_fwmark_seq por chamada mesmo quando o dispositivo ja
-- existia, entao qualquer MAC visto pela primeira vez depois de
-- alguns minutos de uptime ja recebia um fwmark > 65535 - a classe
-- HTB dedicada nunca era criada (tc rejeitava o classid) e o
-- dispositivo caia sem limite na classe default 1:999.
--
-- Idempotente: so mexe em alguma coisa quando ainda existe fwmark
-- fora da faixa segura; rodar de novo depois de ja corrigido e no-op
-- (o UPDATE final so reafirma o mesmo valor de sequence).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM hotspot_device_traffic WHERE fwmark > 65535) THEN
        -- Fase 1: joga todo fwmark existente pra uma faixa negativa
        -- (unica, sem qualquer chance de colidir com a faixa positiva
        -- compacta da fase 2) - evita "duplicate key value" transiente
        -- que uma unica UPDATE fazendo swap de valores positivos
        -- causaria (constraint UNIQUE nao deferravel e checada a cada
        -- linha, nao so no fim do statement).
        UPDATE hotspot_device_traffic SET fwmark = fwmark - 100000000;

        -- Fase 2: recompacta em ordem estavel (menor fwmark original
        -- vira o menor novo fwmark) numa faixa que sempre cabe em 16
        -- bits, comecando de 100 igual ao START WITH original da
        -- sequence.
        WITH ranked AS (
            SELECT mac_address, ROW_NUMBER() OVER (ORDER BY fwmark) AS rn
            FROM hotspot_device_traffic
        )
        UPDATE hotspot_device_traffic t
        SET fwmark = 99 + ranked.rn
        FROM ranked
        WHERE t.mac_address = ranked.mac_address;
    END IF;

    -- Sempre resincroniza a sequence com o maior fwmark realmente em
    -- uso, corrigido ou nao - garante que o proximo dispositivo novo
    -- continue de onde os existentes pararam, sem colisao.
    PERFORM setval(
        'hotspot_device_fwmark_seq',
        (SELECT COALESCE(MAX(fwmark), 99) FROM hotspot_device_traffic) + 1,
        false
    );
END $$;
