package core

import (
	"strings"
	"sync"
	"time"
)

// Route e uma linha da tabela de descoberta: para um dominio remoto
// conhecido, guarda quem e o dono final ("Owner"), por onde encaminhar a
// consulta ("NextHop"), a distancia ate o dono e de onde esta rota foi
// aprendida - ver RULE.md, secao de discover mode.
type Route struct {
	Domain           string
	Owner            string
	OwnerFingerprint string
	NextHop          string
	Distance         int
	Source           string
	State            string // "ok" | "stale"
	LastSeen         time.Time
}

const (
	StateOK    = "ok"
	StateStale = "stale"
)

// Table e o snapshot em memoria consultado no caminho quente de resolucao
// DNS (zones.For) - nunca faz I/O; quem mantem o conteudo atualizado e a
// goroutine de poll em discover, que substitui o mapa inteiro a cada ciclo
// via Replace().
type Table struct {
	mu     sync.RWMutex
	routes map[string]Route
}

func NewTable() *Table {
	return &Table{routes: map[string]Route{}}
}

func (t *Table) Lookup(domain string) (Route, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	route, ok := t.routes[domain]
	return route, ok
}

func (t *Table) LookupSuffix(labels []string) (string, Route, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for i := 0; i < len(labels); i++ {
		zone := strings.Join(labels[i:], ".")
		route, ok := t.routes[zone]
		if ok {
			return zone, route, true
		}
	}
	return "", Route{}, false
}

func (t *Table) Snapshot() []Route {
	t.mu.RLock()
	defer t.mu.RUnlock()
	routes := make([]Route, 0, len(t.routes))
	for _, route := range t.routes {
		routes = append(routes, route)
	}
	return routes
}

func (t *Table) Replace(routes map[string]Route) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes = routes
}
