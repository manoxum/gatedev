package hotspot

import (
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"time"
)

// liveHotspotClients busca a lista de clientes conectados agora, com
// os MACs ja normalizados - usada tanto pelo loop de reconciliacao
// quanto por liveHotspotClientIP.
func liveHotspotClients(ctx context.Context, worker *workerapi.Client, iface string) ([]workerHotspotClient, error) {
	var clients []workerHotspotClient
	if err := worker.Call(ctx, http.MethodGet, "/hotspot/clients?interface="+url.QueryEscape(iface), nil, &clients); err != nil {
		return nil, err
	}
	for i, client := range clients {
		if normalized, err := normalizeHotspotMAC(client.MAC); err == nil {
			clients[i].MAC = normalized
		}
	}
	return clients, nil
}

// liveHotspotClientIP procura o IP atual de um MAC entre os clientes
// conectados agora - devolve found=false se o dispositivo nao estiver
// conectado (nesse caso nao ha nada pra aplicar ao vivo ainda; o
// proximo ciclo de reconciliacao cuida disso quando ele aparecer).
func liveHotspotClientIP(ctx context.Context, worker *workerapi.Client, iface, mac string) (ip string, found bool) {
	clients, err := liveHotspotClients(ctx, worker, iface)
	if err != nil {
		return "", false
	}
	for _, client := range clients {
		if client.MAC == mac {
			return client.IP, true
		}
	}
	return "", false
}

// rateLimit e um par valor+unidade de taxa, o shape que trafega do
// Postgres ate o tc no worker sem normalizar pra uma unidade comum -
// Value nil = sem limite (Unit e ignorada nesse caso).
type rateLimit struct {
	Value *float64 `json:"value"`
	Unit  rateUnit `json:"unit"`
}

// effectiveDeviceRates decide a taxa que deve valer agora para um
// dispositivo: sempre a taxa configurada (download/upload), Value nil
// (sem classe HTB dedicada, so contagem) se nao houver nenhuma. Taxa e
// independente do LimitType (requisito confirmado com o admin) - cota
// de dispositivo/perfil nunca throttla: estourar um periodo
// configurado e bloqueio rigido (ver reconcileDeviceQuota em
// hotspot_device_quota_store.go), nao reducao de taxa.
func effectiveDeviceRates(limits hotspotLimits) (download, upload rateLimit) {
	return rateLimit{limits.DownloadRateValue, limits.DownloadRateUnit},
		rateLimit{limits.UploadRateValue, limits.UploadRateUnit}
}

type shapingGlobalPayload struct {
	Interface string `json:"interface"`
}

type shapingDevicePayload struct {
	Interface         string   `json:"interface"`
	MAC               string   `json:"mac"`
	IP                string   `json:"ip"`
	Fwmark            int      `json:"fwmark"`
	DownloadRateValue *float64 `json:"downloadRateValue"`
	DownloadRateUnit  rateUnit `json:"downloadRateUnit"`
	UploadRateValue   *float64 `json:"uploadRateValue"`
	UploadRateUnit    rateUnit `json:"uploadRateUnit"`
}

// applyGlobalShaping garante so a contagem agregada de todo o hotspot
// (regras iptables bn-global-up/down) - nao existe mais teto/cota
// global (removido, ver RULE.md), o admin so configura taxa/cota por
// dispositivo ou perfil (hotspot_device_limits.go/hotspot_profiles.go).
// Mantida (chamada todo ciclo por reconcileGlobal e no boot do hotspot
// por reapplyHotspotShaping) so para alimentar o velocimetro/grafico
// geral (useGlobalStats/useGlobalSpeedHistory no frontend) - sem essa
// reaplicacao periodica, se as regras se perdessem uma vez (ex.:
// container do hotspot reiniciado), o painel travava em 0bps
// indefinidamente.
func applyGlobalShaping(ctx context.Context, worker *workerapi.Client, iface string) error {
	return worker.Call(ctx, http.MethodPost, "/hotspot/shaping/global", shapingGlobalPayload{Interface: iface}, nil)
}

// ensureDeviceShaping calcula a taxa efetiva do dispositivo e manda o
// worker garantir contagem + (se houver taxa) classe HTB dedicada -
// chamada tanto ao salvar um limite quanto a cada ciclo do loop de
// reconciliacao (reenviar o IP atual resolve renovacao de DHCP sem o
// worker guardar estado).
func ensureDeviceShaping(ctx context.Context, db *sql.DB, worker *workerapi.Client, iface, mac, ip string) error {
	fwmark, err := getOrCreateDeviceFwmark(db, mac)
	if err != nil {
		return err
	}
	limits, err := effectiveDeviceLimits(db, mac)
	if err != nil {
		return err
	}
	download, upload := effectiveDeviceRates(limits)
	return worker.Call(ctx, http.MethodPost, "/hotspot/shaping/device", shapingDevicePayload{
		Interface:         iface,
		MAC:               mac,
		IP:                ip,
		Fwmark:            fwmark,
		DownloadRateValue: download.Value,
		DownloadRateUnit:  download.Unit,
		UploadRateValue:   upload.Value,
		UploadRateUnit:    upload.Unit,
	}, nil)
}

// applyDeviceShapingLive e o equivalente de applyGlobalShapingLive
// para um dispositivo especifico - so age se ele estiver conectado
// agora (senao nao ha IP pra aplicar; o loop de reconciliacao cuida
// quando ele aparecer).
func applyDeviceShapingLive(ctx context.Context, db *sql.DB, worker *workerapi.Client, mac string) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] limite do dispositivo %s persistido, mas nao foi possivel ler WIFI_INTERFACE: %v", mac, err)
		return
	}
	ip, found := liveHotspotClientIP(ctx, worker, iface, mac)
	if !found {
		return
	}
	if err := ensureDeviceShaping(ctx, db, worker, iface, mac, ip); err != nil {
		log.Printf("[backend] limite do dispositivo %s persistido, mas aplicacao ao vivo falhou: %v", mac, err)
	}
}

// reapplyHotspotShaping recria a contagem agregada global (chain/regras
// bn-global-up/down) depois que o hotspot sobe (mesmo retry com backoff
// de reapplyHotspotBlocklist, ja que ap0/bn-uplink podem demorar alguns
// segundos para existir). As classes por dispositivo sao recriadas
// naturalmente pelo loop de reconciliacao assim que cada cliente
// conectado reaparecer.
func reapplyHotspotShaping(ctx context.Context, worker *workerapi.Client, iface string) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		lastErr = applyGlobalShaping(ctx, worker, iface)
		if lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		log.Printf("[backend] reaplicacao da contagem global falhou: %v", lastErr)
	}
}
