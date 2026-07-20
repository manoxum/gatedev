package hotspot

import (
	"bindnet/backend/internal/platform/config"
	"bindnet/backend/internal/workerapi"
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

func resolvePortalMAC(ctx context.Context, r *http.Request, db *sql.DB, worker *workerapi.Client) (string, error) {
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
	hotspotCfg, err := GetHotspotConfig(ctx, db)
	if err != nil {
		return "", err
	}
	gateway := strings.TrimSpace(hotspotCfg["HOTSPOT_GATEWAY"])
	if gateway == "" {
		gateway = "192.168.12.1"
	}
	return "http://" + gateway + ":" + config.Getenv("FRONTEND_PORT", "9090") + "/portal", nil
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
