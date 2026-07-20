// Package core reune os tipos de dominio compartilhados pelo dns-provider
// (Config de runtime, View e a tabela de rotas) - o nucleo que os demais
// pacotes (zones, discover, dnsserver, store, cache) importam sem criar
// ciclos entre si.
package core

import (
	"database/sql"

	"github.com/redis/go-redis/v9"
)

// View identifica por qual IP de bind a consulta chegou - como cada view
// e um socket UDP separado (ver cmd/dns-provider/main.go), o bind em si ja
// diz de onde a consulta veio, sem precisar inspecionar o IP remoto do
// cliente: containers so alcancam o host pelos gateways Docker detectados
// no host, peers na LAN chegam pelo IP de HOST_SOURCE_CIDR, clientes do
// hotspot so alcancam via HOTSPOT_GATEWAY, e o proprio host consulta via
// 127.0.0.1 (resolv.conf/systemd-resolved apontam pra ai).
type View int

const (
	ViewHost View = iota
	ViewContainer
	ViewHotspot
)

// Config carrega o estado de runtime do dns-provider, montado uma vez na
// inicializacao (ver cmd/dns-provider/main.go) e lido pelos handlers e
// goroutines de descoberta. Nenhum campo muda depois do boot exceto o
// conteudo de Routes, trocado atomicamente pela goroutine de poll.
type Config struct {
	TLDs        map[string]bool
	DomainZones map[string]bool
	NginxHosts  map[string]bool
	NginxZones  map[string]bool
	DB          *sql.DB
	Cache       *redis.Client
	Routes      *Table
	NodeName    string
	Fingerprint string
	RemoteMode  string
}
