package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// limitType e o tipo unico e mutuamente exclusivo de limitacao de um
// dispositivo (override) ou perfil - substitui a combinacao livre de
// cota+credito que causava o bug de "cota para de contabilizar" (um
// perfil com os dois habilitados ao mesmo tempo ativava debito de
// credito por baixo, que bloqueava o dispositivo de verdade - ver
// RULE.md). Taxa (download/upload) continua independente do tipo,
// sempre configuravel nos 3 casos concretos.
//
// limitTypeCustom so e valido em PERFIL: significa que o perfil nao
// aplica limite nenhum - o dispositivo que herdar esse perfil e quem
// define a propria estrategia (unlimited/credit/quota), ver
// effectiveDeviceLimits em hotspot_profiles_apply.go. Um dispositivo
// nunca pode ter LimitType=custom ele mesmo (nao faria sentido -
// "customizado" so descreve "decisao delegada ao proximo nivel", e o
// dispositivo e o ultimo nivel).
type limitType = string

const (
	limitTypeUnlimited limitType = "unlimited"
	limitTypeCredit    limitType = "credit"
	limitTypeQuota     limitType = "quota"
	limitTypeCustom    limitType = "custom"
)

// isValidLimitType valida um limitType vindo da API - allowCustom=true
// so para rotas de perfil (POST/PATCH /api/hotspot/profiles), nunca
// para rotas de dispositivo.
func isValidLimitType(t limitType, allowCustom bool) bool {
	switch t {
	case limitTypeUnlimited, limitTypeCredit, limitTypeQuota:
		return true
	case limitTypeCustom:
		return allowCustom
	default:
		return false
	}
}

// hotspotLimits e o shape de limite de um dispositivo (override) ou
// perfil. LimitType decide qual bloco esta em uso: nenhum (unlimited),
// a politica de credito vinculada em hotspot_device_credit (credit), ou
// ate os 3 tetos de cota abaixo em simultaneo, cada um com seu proprio
// acumulador em hotspot_device_quota_periods (quota).
type hotspotLimits struct {
	DownloadRateValue *int     `json:"downloadRateValue"`
	DownloadRateUnit  rateUnit `json:"downloadRateUnit"`
	UploadRateValue   *int     `json:"uploadRateValue"`
	UploadRateUnit    rateUnit `json:"uploadRateUnit"`

	LimitType         limitType `json:"limitType"`
	DailyQuotaBytes   *int64    `json:"dailyQuotaBytes"`
	DailyQuotaUnit    rateUnit  `json:"dailyQuotaUnit"`
	WeeklyQuotaBytes  *int64    `json:"weeklyQuotaBytes"`
	WeeklyQuotaUnit   rateUnit  `json:"weeklyQuotaUnit"`
	MonthlyQuotaBytes *int64    `json:"monthlyQuotaBytes"`
	MonthlyQuotaUnit  rateUnit  `json:"monthlyQuotaUnit"`
}

// hotspotDeviceLimitsResponse e a resposta do GET - sempre os limites
// EFETIVOS (herdados do perfil, ou o override proprio do dispositivo
// quando o perfil e "custom" - ver effectiveDeviceLimits), acompanhados
// do nome/tipo do perfil vinculado para o frontend decidir se mostra um
// resumo so-leitura ("herdado do perfil X") ou o formulario editavel
// (so quando ProfileLimitType == "custom").
type hotspotDeviceLimitsResponse struct {
	hotspotLimits
	ProfileName      string    `json:"profileName"`
	ProfileLimitType limitType `json:"profileLimitType"`
}

func registerHotspotDeviceLimitRoutes(mux *http.ServeMux, admin *administrator, db *sql.DB, worker *workerClient) {
	mux.HandleFunc("GET /api/hotspot/devices/{mac}/limits", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		limits, err := effectiveDeviceLimits(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profileID, err := deviceProfileID(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profile, _, err := getProfile(db, profileID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hotspotDeviceLimitsResponse{
			hotspotLimits:    limits,
			ProfileName:      profile.Name,
			ProfileLimitType: profile.LimitType,
		})
	}))

	mux.HandleFunc("PATCH /api/hotspot/devices/{mac}/limits", requireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		var limits hotspotLimits
		if err := json.NewDecoder(r.Body).Decode(&limits); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if limits.LimitType != "" && !isValidLimitType(limits.LimitType, false) {
			http.Error(w, "campo 'limitType' invalido", http.StatusBadRequest)
			return
		}
		profileID, err := deviceProfileID(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profile, _, err := getProfile(db, profileID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if profile.LimitType != limitTypeCustom {
			http.Error(w, "so e possivel definir um limite proprio quando o perfil vinculado e 'customizado'", http.StatusConflict)
			return
		}
		if err := upsertDeviceLimits(db, mac, limits); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyDeviceShapingLive(r.Context(), db, worker, mac)
		w.WriteHeader(http.StatusNoContent)
	}))
}

