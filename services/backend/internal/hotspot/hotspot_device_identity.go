package hotspot

import (
	"bindnet/backend/internal/auth"
	"bindnet/backend/internal/hotspot/store"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// hotspotIdentityRequest e o corpo do PATCH .../identity - ponteiro
// nil (chave ausente no JSON) preserva o valor atual do campo,
// permitindo tanto edicao parcial (so alias, na aba de visao geral)
// quanto edicao completa (modal "Identificar", com alias + vendor +
// deviceName + osName juntos).
type hotspotIdentityRequest struct {
	Alias      *string `json:"alias"`
	Vendor     *string `json:"vendor"`
	DeviceName *string `json:"deviceName"`
	OSName     *string `json:"osName"`
}

func trimmedPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

// RegisterHotspotDeviceIdentityRoute e chamada por
// RegisterHotspotDeviceRoutes (hotspot_devices.go) - so foi separada
// em arquivo proprio pra nao estourar o limite de ~200 linhas por
// arquivo do projeto.
func RegisterHotspotDeviceIdentityRoute(mux *http.ServeMux, admin *auth.Administrator, db *sql.DB) {
	mux.HandleFunc("PATCH /api/hotspot/devices/{mac}/identity", auth.RequireSession(admin, func(w http.ResponseWriter, r *http.Request) {
		mac, err := normalizeHotspotMAC(r.PathValue("mac"))
		if err != nil {
			http.Error(w, "mac invalido", http.StatusBadRequest)
			return
		}
		var req hotspotIdentityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		edit := store.HotspotIdentityEdit{
			Alias:      trimmedPointer(req.Alias),
			Vendor:     trimmedPointer(req.Vendor),
			DeviceName: trimmedPointer(req.DeviceName),
			OSName:     trimmedPointer(req.OSName),
		}
		if err := store.UpdateHotspotDeviceIdentity(db, mac, edit); err != nil {
			if errors.Is(err, store.ErrHotspotAliasTaken) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		info, _, err := store.HotspotDeviceInfoByMAC(db, mac)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(infoToClientFields(mac, info, store.BlockReasonNone))
	}))
}
