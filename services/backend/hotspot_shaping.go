package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"
)

// hotspotWifiInterface busca WIFI_INTERFACE direto por ctx (variante
// de currentHotspotInterface para uso fora de um http.Request, como o
// loop de reconciliacao e as aplicacoes "ao vivo" apos salvar um
// limite).
func hotspotWifiInterface(ctx context.Context, worker *workerClient) (string, error) {
	var config map[string]string
	if err := worker.call(ctx, http.MethodGet, "/env?section=hotspot", nil, &config); err != nil {
		return "", err
	}
	iface := config["WIFI_INTERFACE"]
	if iface == "" {
		return "", errors.New("WIFI_INTERFACE nao configurado")
	}
	return iface, nil
}

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

// effectiveDeviceRates decide a taxa Mbps que deve valer agora: a
// configurada, ou a de throttle se a cota do periodo ja estourou, ou
// nil (sem classe HTB dedicada, so contagem) se nao houver nenhuma.
func effectiveDeviceRates(limits hotspotLimits, traffic hotspotDeviceTraffic) (downloadMbps, uploadMbps *int) {
	if traffic.Throttled {
		if limits.QuotaThrottleDownloadMbps != nil || limits.QuotaThrottleUploadMbps != nil {
			return limits.QuotaThrottleDownloadMbps, limits.QuotaThrottleUploadMbps
		}
	}
	return limits.DownloadRateMbps, limits.UploadRateMbps
}

func effectiveGlobalRates(limits hotspotLimits, traffic hotspotGlobalTraffic) (downloadMbps, uploadMbps *int) {
	if traffic.Throttled {
		if limits.QuotaThrottleDownloadMbps != nil || limits.QuotaThrottleUploadMbps != nil {
			return limits.QuotaThrottleDownloadMbps, limits.QuotaThrottleUploadMbps
		}
	}
	return limits.DownloadRateMbps, limits.UploadRateMbps
}

type shapingGlobalPayload struct {
	Interface    string `json:"interface"`
	DownloadMbps *int   `json:"downloadMbps"`
	UploadMbps   *int   `json:"uploadMbps"`
}

type shapingDevicePayload struct {
	Interface    string `json:"interface"`
	MAC          string `json:"mac"`
	IP           string `json:"ip"`
	Fwmark       int    `json:"fwmark"`
	DownloadMbps *int   `json:"downloadMbps"`
	UploadMbps   *int   `json:"uploadMbps"`
}

func applyGlobalShaping(ctx context.Context, worker *workerClient, iface string, downloadMbps, uploadMbps *int) error {
	return worker.call(ctx, http.MethodPost, "/hotspot/shaping/global", shapingGlobalPayload{
		Interface:    iface,
		DownloadMbps: downloadMbps,
		UploadMbps:   uploadMbps,
	}, nil)
}

// applyGlobalShapingLive aplica o limite global assim que o admin
// salva a configuracao, sem esperar o proximo ciclo de reconciliacao -
// best-effort (so loga se o hotspot estiver desligado/inacessivel, a
// config ja foi persistida no Postgres de qualquer forma).
func applyGlobalShapingLive(ctx context.Context, db *sql.DB, worker *workerClient) {
	iface, err := hotspotWifiInterface(ctx, worker)
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
	downloadMbps, uploadMbps := effectiveGlobalRates(limits, traffic)
	if err := applyGlobalShaping(ctx, worker, iface, downloadMbps, uploadMbps); err != nil {
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
	downloadMbps, uploadMbps := effectiveDeviceRates(limits, traffic)
	return worker.call(ctx, http.MethodPost, "/hotspot/shaping/device", shapingDevicePayload{
		Interface:    iface,
		MAC:          mac,
		IP:           ip,
		Fwmark:       fwmark,
		DownloadMbps: downloadMbps,
		UploadMbps:   uploadMbps,
	}, nil)
}

// applyDeviceShapingLive e o equivalente de applyGlobalShapingLive
// para um dispositivo especifico - so age se ele estiver conectado
// agora (senao nao ha IP pra aplicar; o loop de reconciliacao cuida
// quando ele aparecer).
func applyDeviceShapingLive(ctx context.Context, db *sql.DB, worker *workerClient, mac string) {
	iface, err := hotspotWifiInterface(ctx, worker)
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
	downloadMbps, uploadMbps := effectiveGlobalRates(limits, traffic)

	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		lastErr = applyGlobalShaping(ctx, worker, iface, downloadMbps, uploadMbps)
		if lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		log.Printf("[backend] reaplicacao do shaping global falhou: %v", lastErr)
	}
}
