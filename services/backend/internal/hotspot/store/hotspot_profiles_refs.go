package store

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

type hotspotProfileRef struct {
	ID   string
	Name string
}

// HotspotDeviceProfileRefs devolve o perfil (id+nome) efetivamente
// vinculado a cada MAC que ja tem linha em hotspot_device_info -
// dispositivos sem linha ainda (nunca vistos) nao aparecem aqui;
// listEnrichedHotspotClients trata a ausencia como perfil Padrao. O
// join sempre resolve para algum perfil (no minimo o Padrao, protegido
// de remocao) mesmo que profile_id esteja NULL numa linha antiga.
func HotspotDeviceProfileRefs(db *sql.DB) (map[string]hotspotProfileRef, error) {
	rows, err := db.Query(`
		SELECT i.mac_address, p.id, p.name
		FROM hotspot_device_info i
		JOIN hotspot_profiles p ON p.id = COALESCE(i.profile_id, $1)
	`, DefaultProfileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refs := map[string]hotspotProfileRef{}
	for rows.Next() {
		var mac string
		var ref hotspotProfileRef
		if err := rows.Scan(&mac, &ref.ID, &ref.Name); err != nil {
			return nil, err
		}
		refs[mac] = ref
	}
	return refs, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
