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
package hotspot

import (
	"bindnet/backend/internal/audit"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

var errPortalDeviceNotIdentified = errors.New("nao foi possivel identificar seu dispositivo - reconecte-se ao Wi-Fi e tente novamente")

type hotspotPortalMeResponse struct {
	MAC             string                             `json:"mac"`
	Alias           string                             `json:"alias,omitempty"`
	ProfileName     string                             `json:"profileName,omitempty"`
	Blocked         bool                               `json:"blocked"`
	LimitType       limitType                          `json:"limitType"`
	BlockedByCredit bool                               `json:"blockedByCredit"`
	BalanceBytes    int64                              `json:"balanceBytes"`
	PlafondBytes    *int64                             `json:"plafondBytes"`
	QuotaPeriods    []hotspotDeviceQuotaPeriodResponse `json:"quotaPeriods,omitempty"`
}

type hotspotPortalRedeemRequest struct {
	Code string `json:"code"`
}

func RegisterHotspotPortalRoutes(mux *http.ServeMux, db *sql.DB, worker *workerapi.Client, audit *audit.Client) {
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
		audit.Record(r.Context(), "voucher_redeemed", mac, map[string]any{"code": req.Code})
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

func portalMeResponse(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac string) (hotspotPortalMeResponse, error) {
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
		Blocked:         blocked[mac] != "",
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
