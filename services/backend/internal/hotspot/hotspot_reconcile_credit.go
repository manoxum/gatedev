package hotspot

import (
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
)

// reconcileDeviceCredit desconta o trafego deste ciclo do saldo de
// credito (so quando o LimitType efetivo do dispositivo e "credit") e
// bloqueia ao vivo assim que o saldo zera - desbloquear e
// responsabilidade exclusiva de uma recarga (manual ou automatica) ou
// de uma troca de tipo (ver syncDeviceCreditFromProfile), nunca deste
// loop. Enquanto o dispositivo continuar bloqueado por credito, reforca
// o bloqueio a cada ciclo (auto-cura): o hotspot flusha o chain
// BINDNET-HOTSPOT a cada start/apply, o que apagaria as regras DROP
// junto - reenviar aqui, mesmo idioma ja usado por ensureDeviceShaping.
// O trace bruto no Mongo (creditTrace) e gravado pra TODO dispositivo,
// tipo credito ou nao - e o que alimenta o detalhe de consumo de uma
// sessao (GET .../sessions/{id}/consumption, ver hotspot_sessions.go);
// fora do tipo credito so o trace acontece, o saldo nunca e debitado
// (nao ha "conta" pra descontar).
func reconcileDeviceCredit(ctx context.Context, db *sql.DB, worker *workerapi.Client, creditTrace *creditTraceClient, mac, ip string, effectiveType limitType, totalBytes int64) error {
	credit, err := syncDeviceCreditFromProfile(ctx, db, worker, mac)
	if err != nil {
		return err
	}
	if effectiveType != limitTypeCredit {
		if totalBytes == 0 {
			return nil
		}
		return creditTrace.recordDebit(ctx, mac, -totalBytes, credit.BalanceBytes)
	}
	if credit.BlockedByCredit {
		applyLiveTrafficBlock(ctx, db, worker, mac, ip, true)
		applyCaptivePortalRedirect(ctx, db, worker, mac, true)
	}
	if totalBytes == 0 {
		return nil
	}
	newBalance, err := debitDeviceCredit(ctx, db, creditTrace, mac, totalBytes)
	if err != nil {
		return err
	}
	if newBalance <= 0 && !credit.BlockedByCredit {
		if err := blockDeviceForCredit(db, mac); err != nil {
			return err
		}
		applyLiveTrafficBlock(ctx, db, worker, mac, ip, true)
		applyCaptivePortalRedirect(ctx, db, worker, mac, true)
	}
	return nil
}
