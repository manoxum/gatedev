// hotspot_portal.go expoe as unicas rotas do hotspot que nao exigem
// sessao autenticada - a pagina de autoatendimento que um dispositivo
// conectado ao Wi-Fi acessa sozinho (sem login) para ver seu
// saldo/cota e resgatar um voucher. Mesmo precedente de
// GET /api/mesh/ca (services/backend/certificates.go): uma excecao
// deliberada e restrita, nunca uma rota generica sem protecao. O MAC
// do chamador nunca vem do corpo/query - e sempre resolvido no
// servidor a partir do IP de origem, cruzado com a lista de clientes
// ao vivo do worker (mesma funcao ja usada por todo o resto do
// hotspot, liveHotspotClients).
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

var errPortalDeviceNotIdentified = errors.New("nao foi possivel identificar seu dispositivo - reconecte-se ao Wi-Fi e tente novamente")

type hotspotPortalMeResponse struct {
	MAC             string                             `json:"mac"`
	Alias           string                             `json:"alias,omitempty"`
	ProfileName     string                             `json:"profileName,omitempty"`
	Blocked         bool                               `json:"blocked"`
	LimitType       limitType                          `json:"limitType"`
	BlockedByCredit bool                                `json:"blockedByCredit"`
	BalanceBytes    int64                              `json:"balanceBytes"`
	PlafondBytes    *int64                             `json:"plafondBytes"`
	QuotaPeriods    []hotspotDeviceQuotaPeriodResponse `json:"quotaPeriods,omitempty"`
}

type hotspotPortalRedeemRequest struct {
	Code string `json:"code"`
}

func registerHotspotPortalRoutes(mux *http.ServeMux, db *sql.DB, worker *workerClient, audit *auditClient) {
	mux.HandleFunc("GET /api/hotspot/portal/me", func(w http.ResponseWriter, r *http.Request) {
		mac, err := resolvePortalMAC(r.Context(), r, db, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		response, err := portalMeResponse(r.Context(), db, worker, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})

	mux.HandleFunc("POST /api/hotspot/portal/vouchers/redeem", func(w http.ResponseWriter, r *http.Request) {
		mac, err := resolvePortalMAC(r.Context(), r, db, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		var req hotspotPortalRedeemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" {
			http.Error(w, "campo 'code' obrigatorio", http.StatusBadRequest)
			return
		}
		credit, err := redeemVoucher(r.Context(), db, worker, strings.TrimSpace(req.Code), mac)
		if errors.Is(err, errHotspotVoucherInvalid) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		audit.record(r.Context(), "voucher_redeemed", mac, map[string]any{"code": req.Code})
		w.Header().Set("Content-Type", "application/json")
		// redeemVoucher ja forca o dispositivo para o tipo credito (ver
		// hotspot_vouchers.go) - sem necessidade de reler effectiveDeviceLimits.
		_ = json.NewEncoder(w).Encode(creditResponse(credit, limitTypeCredit))
	})

	mux.HandleFunc("GET /api/hotspot/portal/credit/history", func(w http.ResponseWriter, r *http.Request) {
		mac, err := resolvePortalMAC(r.Context(), r, db, worker)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		entries, err := listCreditHistory(db, mac, hotspotCreditHistoryLimit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	})
}

func portalMeResponse(ctx context.Context, db *sql.DB, worker *workerClient, mac string) (hotspotPortalMeResponse, error) {
	credit, err := syncDeviceCreditFromProfile(ctx, db, worker, mac)
	if err != nil {
		return hotspotPortalMeResponse{}, err
	}
	limits, err := effectiveDeviceLimits(db, mac)
	if err != nil {
		return hotspotPortalMeResponse{}, err
	}
	info, _, err := hotspotDeviceInfoByMAC(db, mac)
	if err != nil {
		return hotspotPortalMeResponse{}, err
	}
	blocked, err := hotspotBlockedSet(db)
	if err != nil {
		return hotspotPortalMeResponse{}, err
	}
	profileID, err := deviceProfileID(db, mac)
	if err != nil {
		return hotspotPortalMeResponse{}, err
	}
	profile, _, err := getProfile(db, profileID)
	if err != nil {
		return hotspotPortalMeResponse{}, err
	}
	var quotaPeriods []hotspotDeviceQuotaPeriodResponse
	if limits.LimitType == limitTypeQuota {
		quotaPeriods, err = listDeviceQuotaPeriods(db, mac, limits)
		if err != nil {
			return hotspotPortalMeResponse{}, err
		}
	}

	response := hotspotPortalMeResponse{
		MAC:             mac,
		ProfileName:     profile.Name,
		Blocked:         blocked[mac],
		LimitType:       limits.LimitType,
		BlockedByCredit: credit.BlockedByCredit,
		BalanceBytes:    credit.BalanceBytes,
		PlafondBytes:    credit.PlafondBytes,
		QuotaPeriods:    quotaPeriods,
	}
	if info.Alias.Valid {
		response.Alias = info.Alias.String
	}
	return response, nil
}

// resolvePortalMAC identifica o dispositivo que fez a requisicao pelo
// IP de origem - nunca aceita um MAC vindo do cliente. Le o IP de
// X-Forwarded-For (o proprio nginx do frontend, primeiro salto entre o
// dispositivo e o backend, ja envia esse cabecalho - ver
// services/frontend/nginx.conf) e cai para RemoteAddr se ausente
// (acesso direto ao backend). Cruza contra a lista de clientes ao vivo
// do worker (liveHotspotClients, ja usada por todo o resto do
// hotspot) - tenta algumas vezes com um pequeno intervalo (mesmo padrao
// de retry de reapplyHotspotShaping/reapplyHotspotBlocklist) antes de
// desistir, pra absorver oscilacao passageira de associacao Wi-Fi ou a
// janela de boot do hotspot (troca de canal/banda), quando o
// dispositivo continua conectado de verdade mas ainda nao reapareceu na
// lista ao vivo do create_ap. Falha esgotada devolve um erro generico
// pro cliente, nunca aceita um MAC alternativo - a causa real fica só
// no log do servidor.
func resolvePortalMAC(ctx context.Context, r *http.Request, db *sql.DB, worker *workerClient) (string, error) {
	ip := clientIPFromRequest(r)
	if ip == "" {
		log.Printf("[backend] portal: nao foi possivel ler o IP de origem da requisicao (X-Forwarded-For/RemoteAddr ausentes)")
		return "", errPortalDeviceNotIdentified
	}

	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] portal: falha ao ler WIFI_INTERFACE para identificar %s: %v", ip, err)
		return "", errPortalDeviceNotIdentified
	}

	const attempts = 3
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		clients, err := liveHotspotClients(ctx, worker, iface)
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		for _, client := range clients {
			if client.IP == ip {
				return client.MAC, nil
			}
		}
	}
	if lastErr != nil {
		log.Printf("[backend] portal: falha ao listar clientes do hotspot ao identificar %s: %v", ip, lastErr)
	} else {
		log.Printf("[backend] portal: IP %s nao encontrado na lista de clientes ao vivo do hotspot apos %d tentativas", ip, attempts)
	}
	return "", errPortalDeviceNotIdentified
}

// hotspotPortalURL monta o endereco publico da pagina de
// autoatendimento a partir do HOTSPOT_GATEWAY configurado (mesma
// config lida por hotspotWifiInterface) - usado pelo worker como alvo
// do redirect do portal cativo (ver applyCaptivePortalRedirect em
// hotspot_credit_recharge.go). FRONTEND_PORT segue o mesmo default
// (9090) do docker-compose.services.yml.
func hotspotPortalURL(ctx context.Context, db *sql.DB) (string, error) {
	config, err := getHotspotConfig(ctx, db)
	if err != nil {
		return "", err
	}
	gateway := strings.TrimSpace(config["HOTSPOT_GATEWAY"])
	if gateway == "" {
		gateway = "192.168.12.1"
	}
	return "http://" + gateway + ":" + getenv("FRONTEND_PORT", "9090") + "/portal", nil
}

func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
