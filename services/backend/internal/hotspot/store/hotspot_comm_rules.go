// hotspot_comm_rules.go e o acesso a dados das regras de comunicacao
// entre clientes do hotspot (tabela hotspot_comm_rules) - quem avalia
// precedencia/sentido e o motor puro em hotspot_isolation_policy.go;
// aqui e so CRUD + validacao de shape.
package store

import (
	"database/sql"
	"time"
)

const (
	CommEndpointDevice  = "device"
	CommEndpointProfile = "profile"
	CommEndpointAny     = "any"

	CommDirectionTo   = "to"
	CommDirectionBoth = "both"

	CommActionAllow = "allow"
	CommActionDeny  = "deny"

	// Zonas do firewall - qual caminho de trafego a regra governa.
	CommZoneClients = "clients"
	CommZoneWAN     = "wan"
	CommZoneLocal   = "local"

	CommProtocolAny  = "any"
	CommProtocolTCP  = "tcp"
	CommProtocolUDP  = "udp"
	CommProtocolICMP = "icmp"
)

type CommRule struct {
	ID         string    `json:"id"`
	Zone       string    `json:"zone"`
	SourceKind string    `json:"sourceKind"`
	SourceRef  string    `json:"sourceRef"`
	TargetKind string    `json:"targetKind"`
	TargetRef  *string   `json:"targetRef"`
	Direction  string    `json:"direction"`
	Protocol   string    `json:"protocol"`
	DstPorts   *string   `json:"dstPorts"`
	DstHost    *string   `json:"dstHost"`
	Action     string    `json:"action"`
	Enabled    bool      `json:"enabled"`
	Note       *string   `json:"note"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type CommRuleRequest struct {
	Zone       string  `json:"zone"`
	SourceKind string  `json:"sourceKind"`
	SourceRef  string  `json:"sourceRef"`
	TargetKind string  `json:"targetKind"`
	TargetRef  *string `json:"targetRef"`
	Direction  string  `json:"direction"`
	Protocol   string  `json:"protocol"`
	DstPorts   *string `json:"dstPorts"`
	DstHost    *string `json:"dstHost"`
	Action     string  `json:"action"`
	Enabled    *bool   `json:"enabled"`
	Note       *string `json:"note"`
}

const commRuleColumns = `
	id, zone, source_kind, source_ref, target_kind, target_ref,
	direction, protocol, dst_ports, dst_host, action, enabled, note, created_at, updated_at
`

func scanCommRule(row interface{ Scan(...any) error }) (CommRule, error) {
	var rule CommRule
	err := row.Scan(&rule.ID, &rule.Zone, &rule.SourceKind, &rule.SourceRef, &rule.TargetKind, &rule.TargetRef,
		&rule.Direction, &rule.Protocol, &rule.DstPorts, &rule.DstHost, &rule.Action, &rule.Enabled, &rule.Note,
		&rule.CreatedAt, &rule.UpdatedAt)
	return rule, err
}

func ListCommRules(db *sql.DB) ([]CommRule, error) {
	rows, err := db.Query(`SELECT ` + commRuleColumns + ` FROM hotspot_comm_rules ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []CommRule{}
	for rows.Next() {
		rule, err := scanCommRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func InsertCommRule(db *sql.DB, req CommRuleRequest) (CommRule, error) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return scanCommRule(db.QueryRow(`
		INSERT INTO hotspot_comm_rules (
			zone, source_kind, source_ref, target_kind, target_ref,
			direction, protocol, dst_ports, dst_host, action, enabled, note
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+commRuleColumns,
		req.Zone, req.SourceKind, req.SourceRef, req.TargetKind, req.TargetRef,
		req.Direction, req.Protocol, req.DstPorts, req.DstHost, req.Action, enabled, req.Note,
	))
}

func UpdateCommRule(db *sql.DB, id string, req CommRuleRequest) (CommRule, bool, error) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule, err := scanCommRule(db.QueryRow(`
		UPDATE hotspot_comm_rules
		SET zone = $2, source_kind = $3, source_ref = $4, target_kind = $5, target_ref = $6,
		    direction = $7, protocol = $8, dst_ports = $9, dst_host = $10, action = $11, enabled = $12, note = $13,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING `+commRuleColumns,
		id, req.Zone, req.SourceKind, req.SourceRef, req.TargetKind, req.TargetRef,
		req.Direction, req.Protocol, req.DstPorts, req.DstHost, req.Action, enabled, req.Note,
	))
	if err == sql.ErrNoRows {
		return CommRule{}, false, nil
	}
	if err != nil {
		return CommRule{}, false, err
	}
	return rule, true, nil
}

func DeleteCommRule(db *sql.DB, id string) (bool, error) {
	result, err := db.Exec(`DELETE FROM hotspot_comm_rules WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// deleteCommRulesForProfileTx apaga toda regra que referencia o perfil
// em qualquer extremidade - chamada dentro da transacao de
// DeleteProfile (hotspot_profiles_store.go).
func deleteCommRulesForProfileTx(tx *sql.Tx, profileID string) error {
	_, err := tx.Exec(`
		DELETE FROM hotspot_comm_rules
		WHERE (source_kind = $1 AND source_ref = $2) OR (target_kind = $1 AND target_ref = $2)
	`, CommEndpointProfile, profileID)
	return err
}
