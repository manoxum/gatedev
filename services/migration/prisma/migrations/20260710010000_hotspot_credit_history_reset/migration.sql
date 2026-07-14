-- Reseta o extrato de credito para comecar do zero na nova conta
-- corrente unificada (recarga/voucher + debito agregado por sessao,
-- ver services/backend/hotspot_credit_history.go e
-- hotspot_sessions.go). As linhas antigas de entry_type='debit' em
-- hotspot_device_credit_history nao sao mais lidas por nenhum
-- endpoint (o debito passou a viver no Mongo + hotspot_device_sessions)
-- e sessoes anteriores a essa mudanca nao tem total_bytes confiavel
-- (a coluna acabou de ser criada) - apagar as duas tabelas evita
-- misturar dado incompleto/obsoleto com o novo extrato.
DELETE FROM "hotspot_device_credit_history";
DELETE FROM "hotspot_device_sessions";
