package main

import (
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
func liveHotspotClients(ctx context.Context, worker *workerClient, iface string) ([]workerHotspotClient, error) {
	var clients []workerHotspotClient
	if err := worker.call(ctx, http.MethodGet, "/hotspot/clients?interface="+url.QueryEscape(iface), nil, &clients); err != nil {
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
func liveHotspotClientIP(ctx context.Context, worker *workerClient, iface, mac string) (ip string, found bool) {
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
	Value *int     `json:"value"`
	Unit  rateUnit `json:"unit"`
}

// effectiveDeviceRates decide a taxa que deve valer agora: a
// configurada, ou a de throttle se a cota do periodo ja estourou, ou
// Value nil (sem classe HTB dedicada, so contagem) se nao houver
// nenhuma.
func effectiveDeviceRates(limits hotspotLimits, traffic hotspotDeviceTraffic) (download, upload rateLimit) {
	if traffic.Throttled {
		if limits.QuotaThrottleDownloadValue != nil || limits.QuotaThrottleUploadValue != nil {
			return rateLimit{limits.QuotaThrottleDownloadValue, limits.QuotaThrottleDownloadUnit},
				rateLimit{limits.QuotaThrottleUploadValue, limits.QuotaThrottleUploadUnit}
		}
	}
	return rateLimit{limits.DownloadRateValue, limits.DownloadRateUnit},
		rateLimit{limits.UploadRateValue, limits.UploadRateUnit}
}

func effectiveGlobalRates(limits hotspotLimits, traffic hotspotGlobalTraffic) (download, upload rateLimit) {
	if traffic.Throttled {
		if limits.QuotaThrottleDownloadValue != nil || limits.QuotaThrottleUploadValue != nil {
			return rateLimit{limits.QuotaThrottleDownloadValue, limits.QuotaThrottleDownloadUnit},
				rateLimit{limits.QuotaThrottleUploadValue, limits.QuotaThrottleUploadUnit}
		}
	}
	return rateLimit{limits.DownloadRateValue, limits.DownloadRateUnit},
		rateLimit{limits.UploadRateValue, limits.UploadRateUnit}
}

type shapingGlobalPayload struct {
	Interface         string   `json:"interface"`
	DownloadRateValue *int     `json:"downloadRateValue"`
	DownloadRateUnit  rateUnit `json:"downloadRateUnit"`
	UploadRateValue   *int     `json:"uploadRateValue"`
	UploadRateUnit    rateUnit `json:"uploadRateUnit"`
}

type shapingDevicePayload struct {
	Interface         string   `json:"interface"`
	MAC               string   `json:"mac"`
	IP                string   `json:"ip"`
	Fwmark            int      `json:"fwmark"`
	DownloadRateValue *int     `json:"downloadRateValue"`
	DownloadRateUnit  rateUnit `json:"downloadRateUnit"`
	UploadRateValue   *int     `json:"uploadRateValue"`
	UploadRateUnit    rateUnit `json:"uploadRateUnit"`
}

func applyGlobalShaping(ctx context.Context, worker *workerClient, iface string, download, upload rateLimit) error {
	return worker.call(ctx, http.MethodPost, "/hotspot/shaping/global", shapingGlobalPayload{
		Interface:         iface,
		DownloadRateValue: download.Value,
		DownloadRateUnit:  download.Unit,
		UploadRateValue:   upload.Value,
		UploadRateUnit:    upload.Unit,
	}, nil)
}

// applyGlobalShapingLive aplica o limite global assim que o admin
// salva a configuracao, sem esperar o proximo ciclo de reconciliacao -
// best-effort (so loga se o hotspot estiver desligado/inacessivel, a
// config ja foi persistida no Postgres de qualquer forma).
func applyGlobalShapingLive(ctx context.Context, db *sql.DB, worker *workerClient) {
	iface, err := hotspotWifiInterface(ctx, db)
	if err != nil {
		log.Printf("[backend] limite global persistido, mas nao foi possivel ler WIFI_INTERFACE: %v", err)
		return
	}
	limits, err := getGlobalLimits(db)
	if err != nil {
		log.Printf("[backend] limite global persistido, mas falha ao reler do Postgres: %v", err)
		return
	}
	traffic, err := ensureGlobalTrafficRow(db)
	if err != nil {
		log.Printf("[backend] limite global persistido, mas falha ao ler acumulado do periodo: %v", err)
		return
	}
	download, upload := effectiveGlobalRates(limits, traffic)
	if err := applyGlobalShaping(ctx, worker, iface, download, upload); err != nil {
		log.Printf("[backend] limite global persistido, mas aplicacao ao vivo falhou: %v", err)
	}
}

// ensureDeviceShaping calcula a taxa efetiva do dispositivo e manda o
// worker garantir contagem + (se houver taxa) classe HTB dedicada -
// chamada tanto ao salvar um limite quanto a cada ciclo do loop de
// reconciliacao (reenviar o IP atual resolve renovacao de DHCP sem o
// worker guardar estado).
func ensureDeviceShaping(ctx context.Context, db *sql.DB, worker *workerClient, iface, mac, ip string) error {
	fwmark, err := getOrCreateDeviceFwmark(db, mac)
	if err != nil {
		return err
	}
	limits, _, err := getDeviceLimits(db, mac)
	if err != nil {
		return err
	}
	traffic, err := ensureDeviceTrafficRow(db, mac)
	if err != nil {
		return err
	}
	download, upload := effectiveDeviceRates(limits, traffic)
	return worker.call(ctx, http.MethodPost, "/hotspot/shaping/device", shapingDevicePayload{
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
func applyDeviceShapingLive(ctx context.Context, db *sql.DB, worker *workerClient, mac string) {
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

// reapplyHotspotShaping recria o chain/qdiscs globais depois que o
// hotspot sobe (mesmo retry com backoff de reapplyHotspotBlocklist,
// ja que ap0/bn-uplink podem demorar alguns segundos para existir). As
// classes por dispositivo sao recriadas naturalmente pelo loop de
// reconciliacao assim que cada cliente conectado reaparecer.
func reapplyHotspotShaping(ctx context.Context, db *sql.DB, worker *workerClient, iface string) {
	limits, err := getGlobalLimits(db)
	if err != nil {
		log.Printf("[backend] falha ao ler limite global do hotspot: %v", err)
		return
	}
	traffic, err := ensureGlobalTrafficRow(db)
	if err != nil {
		log.Printf("[backend] falha ao ler acumulado global do hotspot: %v", err)
		return
	}
	download, upload := effectiveGlobalRates(limits, traffic)

	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		lastErr = applyGlobalShaping(ctx, worker, iface, download, upload)
		if lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		log.Printf("[backend] reaplicacao do shaping global falhou: %v", lastErr)
	}
}
