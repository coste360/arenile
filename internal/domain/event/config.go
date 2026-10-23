package event

// ConfigChange is the payload (version 1) of ConfigPricingChanged and
// ConfigMapChanged, shared by every service that changes the configuration
// (spec 18.1, ADR 0008). Consumers treat both events as "reload the
// configuration of the establishment": the fields only say what changed.
//
// Entity and Change are always set; the other fields are present when they
// apply. Entity values of config.pricing_changed: price_rule, fixed_price,
// promo_code, agreement, booking_policy, season_copy, season, rate_period,
// establishment_settings, pack, extra, setup. Entity values of
// config.map_changed: map, zone, pool, setup. Change values: created,
// updated, deleted, archived, copied, saved, rows_generated, applied,
// reoriented.
type ConfigChange struct {
	Entity   string `json:"entity"`
	EntityID string `json:"entity_id,omitempty"`
	Change   string `json:"change"`
	// Code is the pack or extra code (catalog entities).
	Code string `json:"code,omitempty"`
	// SeasonID is set for season-bound entities (rules, fixed prices,
	// policies, rate periods, the season itself, the guided setup).
	SeasonID string `json:"season_id,omitempty"`
	// MapID is set for map, zone and pool changes bound to a map.
	MapID string `json:"map_id,omitempty"`
	// Version is the new version of versioned entities.
	Version int64 `json:"version,omitempty"`
	// Reason, Template, MapIDs, ZoneIDs and Counts describe bulk changes of
	// the guided setup (Entity "setup": Reason setup_template or
	// quick_pricelist).
	Reason   string         `json:"reason,omitempty"`
	Template string         `json:"template,omitempty"`
	MapIDs   []string       `json:"map_ids,omitempty"`
	ZoneIDs  []string       `json:"zone_ids,omitempty"`
	Counts   map[string]int `json:"counts,omitempty"`
}

// Change values of ConfigChange.
const (
	ChangeCreated       = "created"
	ChangeUpdated       = "updated"
	ChangeDeleted       = "deleted"
	ChangeArchived      = "archived"
	ChangeCopied        = "copied"
	ChangeSaved         = "saved"
	ChangeRowsGenerated = "rows_generated"
	ChangeApplied       = "applied"
	// ChangeReoriented: the sea side of a map moved and the layout was
	// mirrored or rotated (M5A, config.map_changed only).
	ChangeReoriented = "reoriented"
)

// Entity values of ConfigChange shared across packages.
const (
	EntitySetup = "setup"
)
