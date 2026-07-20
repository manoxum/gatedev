// hotspot_sessions.go controla o ciclo conectado/desconectado de cada
// dispositivo no hotspot (Postgres, tabela hotspot_device_sessions) -
// uma sessao nasce quando o MAC aparece na lista de clientes ao vivo e
// fecha quando some dela (ver reconcileHotspotOnce em
// hotspot_reconcile.go). total_bytes e incrementado a cada ciclo de
// reconciliacao com o trafego real do dispositivo (deltaDown+deltaUp
// de recordDeviceUsage), independente de ter credito habilitado -
// debitar saldo de credito (so quando habilitado, ver
// reconcileDeviceCredit) e uma consequencia separada, que tambem grava
// o trace bruto no Mongo (hotspot_credit_debits, ver
// hotspot_credit_trace.go, com TTL). O total consolidado da sessao
// continua no Postgres mesmo depois do trace expirar, com bem menos
// linhas escritas do que gravar um trace por ciclo de reconciliacao.
// Toda sessao (aberta ou encerrada, com ou sem consumo) entra como
// linha de debito na conta corrente de credito (ver
// listSessionMovements, consumido por hotspot_credit_history.go).
package hotspot

import (
	"bindnet/backend/internal/auth"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type hotspotDeviceSession struct {
	ID         int64
	MACAddress string
	StartedAt  time.Time
	EndedAt    sql.NullTime
	TotalBytes int64
}

// ensureOpenSession abre uma sessao para o MAC se ele nao tiver
// nenhuma em aberto - upsert idempotente sobre o indice unico parcial
// hotspot_device_sessions_open_mac_idx, chamado a cada ciclo de
// reconciliacao para todo cliente ao vivo (ver reconcileDevice).
func ensureOpenSession(db *sql.DB, mac string) error {
	_, err := db.Exec(`
		INSERT INTO hotspot_device_sessions (mac_address)
		VALUES ($1)
		ON CONFLICT (mac_address) WHERE ended_at IS NULL DO NOTHING
	`, mac)
	return err
}

// closeStaleSessions fecha toda sessao em aberto cujo MAC nao esta
// mais entre os clientes ao vivo deste ciclo - liveMacs vazio (hotspot
// sem clientes, ou parado) fecha todas. Chamado uma vez por ciclo em
// reconcileHotspotOnce, nao por dispositivo.
func closeStaleSessions(db *sql.DB, liveMacs []string) error {
	_, err := db.Exec(`
		UPDATE hotspot_device_sessions
		SET ended_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE ended_at IS NULL AND NOT (mac_address = ANY($1::text[]))
	`, liveMacs)
	return err
}

// incrementSessionConsumption soma o trafego deste ciclo (deltaDown+
// deltaUp, chamado de reconcileDevice pra todo dispositivo ao vivo,
// com ou sem credito) ao total da sessao em aberto do MAC - no-op se
// nao houver sessao aberta (nao deveria acontecer, ja que
// reconcileDevice sempre chama ensureOpenSession antes).
func incrementSessionConsumption(db *sql.DB, mac string, amountBytes int64) error {
	_, err := db.Exec(`
		UPDATE hotspot_device_sessions
		SET total_bytes = total_bytes + $2, updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1 AND ended_at IS NULL
	`, mac, amountBytes)
	return err
}

// RegisterHotspotSessionRoutes expoe so o detalhe de consumo de uma
// sessao (a lista de sessoes em si aparece embutida na conta corrente
// de credito, ver GET .../credit/history em hotspot_credit_history.go).
func RegisterHotspotSessionRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, creditTrace *creditTraceClient) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/sessions/{id}/consumption", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		sessionID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "id de sessao invalido", http.StatusBadRequest)
			return
		}
		session, err := getDeviceSession(db, mac, sessionID)
		if err == sql.ErrNoRows {
			http.Error(w, "sessao nao encontrada", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var until *time.Time
		if session.EndedAt.Valid {
			until = &session.EndedAt.Time
		}
		entries, err := creditTrace.listDebits(r.Context(), mac, &session.StartedAt, until, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))
}

func getDeviceSession(db *sql.DB, mac string, id int64) (hotspotDeviceSession, error) {
	var s hotspotDeviceSession
	err := db.QueryRow(`
		SELECT id, mac_address, started_at, ended_at, total_bytes
		FROM hotspot_device_sessions
		WHERE mac_address = $1 AND id = $2
	`, mac, id).Scan(&s.ID, &s.MACAddress, &s.StartedAt, &s.EndedAt, &s.TotalBytes)
	return s, err
}

// listSessionMovements devolve toda sessao do MAC (aberta ou
// encerrada, com ou sem consumo ainda) como uma linha de debito da
// conta corrente de credito - entryType distingue "session_active" de
// "session_closed" para a UI diferenciar/filtrar. A data usada e
// updated_at (ultimo debito daquela sessao, ou o proprio started_at se
// nunca debitou nada), o jeito mais fiel de posicionar uma sessao
// ainda aberta na ordem cronologica do extrato. Sem saldo apos
// (BalanceAfterBytes fica nil): o saldo real muda a cada debito
// individual dentro da sessao, e esses ja nao moram mais no Postgres
// (ver hotspot_credit_trace.go) - so o total consolidado.
func listSessionMovements(db *sql.DB, mac string, limit int) ([]hotspotCreditHistoryResponse, error) {
	rows, err := db.Query(`
		SELECT id, started_at, ended_at, updated_at, total_bytes
		FROM hotspot_device_sessions
		WHERE mac_address = $1
		ORDER BY updated_at DESC
		LIMIT $2
	`, mac, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []hotspotCreditHistoryResponse{}
	for rows.Next() {
		var id, totalBytes int64
		var startedAt, updatedAt time.Time
		var endedAt sql.NullTime
		if err := rows.Scan(&id, &startedAt, &endedAt, &updatedAt, &totalBytes); err != nil {
			return nil, err
		}
		entryType := "session_active"
		startedAtStr := startedAt.Format(time.RFC3339)
		var endedAtStr *string
		if endedAt.Valid {
			entryType = "session_closed"
			formatted := endedAt.Time.Format(time.RFC3339)
			endedAtStr = &formatted
		}
		entries = append(entries, hotspotCreditHistoryResponse{
			EntryType:   entryType,
			AmountBytes: -totalBytes,
			CreatedAt:   updatedAt.Format(time.RFC3339),
			SessionID:   &id,
			StartedAt:   &startedAtStr,
			EndedAt:     endedAtStr,
		})
	}
	return entries, rows.Err()
}
