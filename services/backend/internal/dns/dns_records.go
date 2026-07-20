// dns_records.go expoe a tabela local_dns_records (view host do
// split-horizon, ver RULE.md) para o painel: listagem, remocao
// individual/total e adicao manual (reserva antecipada de um IP de
// loopback, mesma alocacao que services/worker/dns/db.go faz na
// primeira consulta - so que disparada pelo operador em vez de por
// uma consulta DNS real).
package dns

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/auth"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

type localDnsRecord struct {
	Hostname       string `json:"hostname"`
	Address        string `json:"address"`
	LoopbackOffset int64  `json:"loopbackOffset"`
	CreatedAt      string `json:"createdAt"`
}

// offsetToLoopback replica services/worker/dns/zones.go:offsetToLoopback -
// os 24 bits menos significativos do offset viram os tres ultimos
// octetos de um IP 127.0.0.0/8.
func offsetToLoopback(offset int64) string {
	b := uint32(offset) & 0xFFFFFF
	return net.IPv4(127, byte(b>>16), byte(b>>8), byte(b)).String()
}

func RegisterDNSRecordRoutes(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB, audit *audit.Client) {
	mux.HandleFunc("GET /api/dns/records", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(), `
			SELECT hostname, loopback_offset, created_at FROM local_dns_records ORDER BY hostname
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		records := []localDnsRecord{}
		for rows.Next() {
			var record localDnsRecord
			if err := rows.Scan(&record.Hostname, &record.LoopbackOffset, &record.CreatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			record.Address = offsetToLoopback(record.LoopbackOffset)
			records = append(records, record)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(records)
	}))

	mux.HandleFunc("POST /api/dns/records", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hostname string `json:"hostname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		hostname := strings.ToLower(strings.TrimSpace(req.Hostname))
		if hostname == "" || strings.ContainsAny(hostname, " \t\n") {
			http.Error(w, "hostname invalido", http.StatusBadRequest)
			return
		}

		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO local_dns_records (hostname, loopback_offset)
			VALUES ($1, nextval('local_dns_records_offset_seq'))
			ON CONFLICT (hostname) DO NOTHING
		`, hostname); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var record localDnsRecord
		record.Hostname = hostname
		err := db.QueryRowContext(r.Context(), `
			SELECT loopback_offset, created_at FROM local_dns_records WHERE hostname = $1
		`, hostname).Scan(&record.LoopbackOffset, &record.CreatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		record.Address = offsetToLoopback(record.LoopbackOffset)

		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "dns_record_added", username, map[string]any{"hostname": hostname})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(record)
	}))

	mux.HandleFunc("DELETE /api/dns/records", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		if _, err := db.ExecContext(r.Context(), `DELETE FROM local_dns_records`); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "dns_records_cleared", username, nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("DELETE /api/dns/records/{hostname}", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		hostname := r.PathValue("hostname")
		if hostname == "" {
			http.Error(w, "hostname obrigatorio", http.StatusBadRequest)
			return
		}
		if _, err := db.ExecContext(r.Context(), `DELETE FROM local_dns_records WHERE hostname = $1`, hostname); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := auth.SessionUser(r, admin)
		audit.Record(r.Context(), "dns_record_removed", username, map[string]any{"hostname": hostname})
		w.WriteHeader(http.StatusNoContent)
	}))
}
