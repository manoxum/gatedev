// Comando worker e o unico componente do stack "bindnet" com acesso
// privilegiado ao host (docker.sock, NetworkManager, network_mode: host).
// Ele expoe uma API interna restrita, acessivel apenas via socket Unix
// compartilhado com o service/backend - nunca por rede TCP. A logica fica
// nos pacotes internal/{compose,network,peer,shaping,hotspot,trust}; este
// main so faz o wiring das rotas.
package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"bindnet/worker/internal/compose"
	"bindnet/worker/internal/hotspot"
	"bindnet/worker/internal/network"
	"bindnet/worker/internal/peer"
	"bindnet/worker/internal/shaping"
	"bindnet/worker/internal/trust"
)

const socketPath = "/run/bindnet-admin/worker.sock"

func main() {
	log.SetFlags(log.LstdFlags)
	log.Println("[worker] iniciando agente privilegiado")

	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		log.Fatalf("[worker] erro ao preparar diretorio do socket: %v", err)
	}
	// Remove socket de uma execucao anterior, se sobrou.
	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("[worker] erro ao escutar socket unix: %v", err)
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		log.Fatalf("[worker] erro ao ajustar permissao do socket: %v", err)
	}

	mux := http.NewServeMux()
	compose.RegisterContainerRoutes(mux)
	hotspot.RegisterServiceRoutes(mux)
	hotspot.RegisterClientRoutes(mux)
	hotspot.RegisterFingerprintRoutes(mux)
	network.RegisterRoutes(mux)
	peer.RegisterRoutes(mux)
	trust.RegisterRoutes(mux)
	shaping.RegisterShapingRoutes(mux)
	shaping.RegisterTrafficBlockRoutes(mux)
	shaping.RegisterCaptivePortalRoutes(mux)
	shaping.StartCaptivePortalResponder()

	log.Println("[worker] API interna pronta em", socketPath)
	if err := http.Serve(listener, mux); err != nil {
		log.Fatalf("[worker] erro no servidor: %v", err)
	}
}
