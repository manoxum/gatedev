package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// hotspotDeviceCredit representa o saldo/config de recarga de um
// dispositivo marcado como "precisa de credito para trafegar".
// blockedByCredit fica separado de hotspot_blocked_devices (bloqueio
// manual do admin) de proposito - os dois mecanismos nao devem se
// confundir nem se sobrescrever.
type hotspotDeviceCredit struct {
	MACAddress          string
	Enabled             bool
	BalanceBytes        int64
	RechargeAmountBytes *int64
	RechargePeriod      *string
	PlafondBytes        *int64
	NextRechargeAt      *time.Time
	BlockedByCredit     bool
}

type hotspotCreditConfigRequest struct {
	Enabled             bool    `json:"enabled"`
	RechargeAmountBytes *int64  `json:"rechargeAmountBytes"`
	RechargePeriod      *string `json:"rechargePeriod"`
	PlafondBytes        *int64  `json:"plafondBytes"`
}

type hotspotCreditResponse struct {
	Enabled             bool    `json:"enabled"`
	BalanceBytes        int64   `json:"balanceBytes"`
	RechargeAmountBytes *int64  `json:"rechargeAmountBytes"`
	RechargePeriod      *string `json:"rechargePeriod"`
	PlafondBytes        *int64  `json:"plafondBytes"`
	NextRechargeAt      *string `json:"nextRechargeAt"`
	BlockedByCredit     bool    `json:"blockedByCredit"`
}

type hotspotCreditRechargeRequest struct {
	AmountBytes int64 `json:"amountBytes"`
}

func registerHotspotCreditRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, worker *workerClient, audit *auditClient) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/credit", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		credit, err := ensureDeviceCreditRow(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(creditResponse(credit))
	}))

	mux.HandleFunc("PATCH /api/hotspot/devices/{mac}/credit", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		var req hotspotCreditConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if err := upsertDeviceCreditConfig(db, mac, req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/hotspot/devices/{mac}/credit/recharge", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		var req hotspotCreditRechargeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AmountBytes <= 0 {
			http.Error(w, "campo 'amountBytes' deve ser positivo", http.StatusBadRequest)
			return
		}
		credit, err := applyManualRecharge(r.Context(), db, worker, mac, req.AmountBytes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		username, _ := sessionUser(r, admin)
		audit.record(r.Context(), "device_credit_recharged", username, map[string]any{"mac": mac, "amountBytes": req.AmountBytes})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(creditResponse(credit))
	}))
}

func creditResponse(credit hotspotDeviceCredit) hotspotCreditResponse {
	response := hotspotCreditResponse{
		Enabled:             credit.Enabled,
		BalanceBytes:        credit.BalanceBytes,
		RechargeAmountBytes: credit.RechargeAmountBytes,
		RechargePeriod:      credit.RechargePeriod,
		PlafondBytes:        credit.PlafondBytes,
		BlockedByCredit:     credit.BlockedByCredit,
	}
	if credit.NextRechargeAt != nil {
		formatted := credit.NextRechargeAt.Format(time.RFC3339)
		response.NextRechargeAt = &formatted
	}
	return response
}

func ensureDeviceCreditRow(db *sql.DB, mac string) (hotspotDeviceCredit, error) {
	var c hotspotDeviceCredit
	err := db.QueryRow(`
		INSERT INTO hotspot_device_credit (mac_address)
		VALUES ($1)
		ON CONFLICT (mac_address) DO UPDATE SET mac_address = EXCLUDED.mac_address
		RETURNING mac_address, enabled, balance_bytes, recharge_amount_bytes, recharge_period,
		          plafond_bytes, next_recharge_at, blocked_by_credit
	`, mac).Scan(&c.MACAddress, &c.Enabled, &c.BalanceBytes, &c.RechargeAmountBytes, &c.RechargePeriod,
		&c.PlafondBytes, &c.NextRechargeAt, &c.BlockedByCredit)
	return c, err
}

// debitDeviceCredit desconta o total trafegado (download+upload) de um
// ciclo de reconciliacao do saldo de credito - chamado pelo loop em
// hotspot_reconcile.go, so quando o dispositivo tem credito habilitado.
func debitDeviceCredit(db *sql.DB, mac string, totalBytes int64) (newBalance int64, err error) {
	err = db.QueryRow(`
		UPDATE hotspot_device_credit
		SET balance_bytes = balance_bytes - $2, updated_at = CURRENT_TIMESTAMP
		WHERE mac_address = $1
		RETURNING balance_bytes
	`, mac, totalBytes).Scan(&newBalance)
	if err != nil {
		return 0, err
	}
	if err := recordCreditHistory(db, mac, "debit", -totalBytes, newBalance); err != nil {
		return 0, err
	}
	return newBalance, nil
}

// applyLiveCreditBlock aplica (ou remove) o bloqueio de trafego por
// falta de credito - diferente de applyLiveHotspotBlock (hostapd
// deny_acl+deauth, usado so pelo bloqueio manual do admin), aqui o
// dispositivo continua associado ao Wi-Fi, so o trafego para de
// passar (DROP via iptables no worker, ver
// services/worker/controller/traffic_block.go). ip so e necessario
// para block=true (a regra de download precisa do IP atual); no
// unblock a remocao e so por comentario, sem IP.
func applyLiveCreditBlock(ctx context.Context, db *sql.DB, worker *workerClient, mac, ip string, block bool) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] bloqueio por credito persistido, mas nao foi possivel ler WIFI_INTERFACE: %v", err)
		return
	}
	path := "/hotspot/trafficunblock"
	payload := map[string]string{"interface": iface, "mac": mac}
	if block {
		path = "/hotspot/trafficblock"
		payload["ip"] = ip
	}
	if err := worker.call(ctx, http.MethodPost, path, payload, nil); err != nil {
		log.Printf("[backend] bloqueio por credito persistido, mas aplicacao ao vivo de %s falhou: %v", mac, err)
	}
}

func hotspotCreditBlockedSet(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT mac_address FROM hotspot_device_credit WHERE blocked_by_credit`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocked := map[string]bool{}
	for rows.Next() {
		var mac string
		if err := rows.Scan(&mac); err != nil {
			return nil, err
		}
		blocked[mac] = true
	}
	return blocked, rows.Err()
}
