// types.go reune os tipos de dados compartilhados do hotspot (limites,
// perfil, dispositivo, voucher). Eles vivem na camada store porque tanto o
// acesso a dados quanto as rotas dependem deles - se ficassem nos arquivos
// de rota, o store nao poderia ser um pacote separado (daria ciclo de
// import store <-> hotspot).
package store

import (
	"database/sql"
	"time"
)

// LimitType e o tipo unico e mutuamente exclusivo de limitacao de um
// dispositivo (override) ou perfil - substitui a combinacao livre de
// cota+credito que causava o bug de "cota para de contabilizar" (um perfil
// com os dois habilitados ao mesmo tempo ativava debito de credito por
// baixo, que bloqueava o dispositivo de verdade - ver RULE.md). Taxa
// (download/upload) continua independente do tipo, sempre configuravel nos
// 3 casos concretos.
//
// LimitTypeCustom so e valido em PERFIL: significa que o perfil nao aplica
// limite nenhum - o dispositivo que herdar esse perfil e quem define a
// propria estrategia (unlimited/credit/quota), ver effectiveDeviceLimits em
// hotspot_profiles_apply.go. Um dispositivo nunca pode ter
// LimitType=custom ele mesmo (nao faria sentido - "customizado" so
// descreve "decisao delegada ao proximo nivel", e o dispositivo e o ultimo
// nivel).
type LimitType = string

const (
	LimitTypeUnlimited LimitType = "unlimited"
	LimitTypeCredit    LimitType = "credit"
	LimitTypeQuota     LimitType = "quota"
	LimitTypeCustom    LimitType = "custom"
)

// IsValidLimitType valida um LimitType vindo da API - allowCustom=true so
// para rotas de perfil (POST/PATCH /api/hotspot/profiles), nunca para
// rotas de dispositivo.
func IsValidLimitType(t LimitType, allowCustom bool) bool {
	switch t {
	case LimitTypeUnlimited, LimitTypeCredit, LimitTypeQuota:
		return true
	case LimitTypeCustom:
		return allowCustom
	default:
		return false
	}
}

// Limits e o shape de limite de um dispositivo (override) ou perfil.
// LimitType decide qual bloco esta em uso: nenhum (unlimited), a politica
// de credito vinculada em hotspot_device_credit (credit), ou ate os 3
// tetos de cota abaixo em simultaneo, cada um com seu proprio acumulador
// em hotspot_device_quota_periods (quota).
type Limits struct {
	// Taxa aceita valor fracionario (1.5MB/s, 17.5KB/s), por isso
	// *float64 e nao *int - a coluna e double precision desde a
	// migration 20260716000000_hotspot_rate_decimal. nil = sem limite.
	DownloadRateValue *float64 `json:"downloadRateValue"`
	DownloadRateUnit  RateUnit `json:"downloadRateUnit"`
	UploadRateValue   *float64 `json:"uploadRateValue"`
	UploadRateUnit    RateUnit `json:"uploadRateUnit"`

	LimitType         LimitType `json:"limitType"`
	DailyQuotaBytes   *int64    `json:"dailyQuotaBytes"`
	DailyQuotaUnit    RateUnit  `json:"dailyQuotaUnit"`
	WeeklyQuotaBytes  *int64    `json:"weeklyQuotaBytes"`
	WeeklyQuotaUnit   RateUnit  `json:"weeklyQuotaUnit"`
	MonthlyQuotaBytes *int64    `json:"monthlyQuotaBytes"`
	MonthlyQuotaUnit  RateUnit  `json:"monthlyQuotaUnit"`
}

// DefaultProfileID e o id fixo do perfil "Padrao" semeado pela migration -
// todo dispositivo sem vinculo explicito cai nele.
const DefaultProfileID = "00000000-0000-0000-0000-000000000001"

type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
	Limits
	CreditRechargeAmountBytes *int64  `json:"creditRechargeAmountBytes"`
	CreditRechargePeriod      *string `json:"creditRechargePeriod"`
	CreditPlafondBytes        *int64  `json:"creditPlafondBytes"`
	// Com o isolamento de clientes ligado (chave CLIENT_ISOLATION em
	// hotspot_config), decide se os clientes deste perfil comunicam
	// entre si - ver hotspot_isolation_policy.go.
	AllowInternalCommunication bool `json:"allowInternalCommunication"`
}

type ProfileRequest struct {
	Name string `json:"name"`
	Limits
	CreditRechargeAmountBytes  *int64  `json:"creditRechargeAmountBytes"`
	CreditRechargePeriod       *string `json:"creditRechargePeriod"`
	CreditPlafondBytes         *int64  `json:"creditPlafondBytes"`
	AllowInternalCommunication bool    `json:"allowInternalCommunication"`
}

type DeviceInfo struct {
	MACAddress string
	Vendor     sql.NullString
	DeviceName sql.NullString
	OSName     sql.NullString
	Confidence sql.NullInt64
	Alias      sql.NullString
}

type KnownDevice struct {
	MACAddress  string
	Vendor      sql.NullString
	DeviceName  sql.NullString
	OSName      sql.NullString
	Alias       sql.NullString
	FirstSeenAt sql.NullTime
	LastSeenAt  sql.NullTime
}

type Voucher struct {
	Code          string     `json:"code"`
	BatchID       string     `json:"batchId,omitempty"`
	AmountBytes   int64      `json:"amountBytes"`
	Status        string     `json:"status"`
	Note          string     `json:"note,omitempty"`
	RedeemedByMAC string     `json:"redeemedByMac,omitempty"`
	RedeemedAt    *time.Time `json:"redeemedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}
