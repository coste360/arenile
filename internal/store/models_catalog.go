package store

import (
	"time"

	"github.com/uptrace/bun"
)

// Models of the catalog area (spec 6.3): components, packs and extras.
// Repository functions live in the area's own file (see
// internal/domain/catalog/doc.go for ownership).
//
// Nullable JSON lists (RequiresSpotKind, AllowedZoneIDs, AllowedPackIDs) use
// nil for "no restriction" (stored as NULL); never store an empty list.

// Component is a physical inventory unit (umbrella, sunbed, cabin...).
type Component struct {
	bun.BaseModel `bun:"table:components,alias:cp"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	Code            string    `bun:"code,notnull"`
	Name            I18nText  `bun:"name,type:json,notnull"`
	Tracked         bool      `bun:"tracked,notnull"`
	PoolID          *string   `bun:"pool_id"`
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// Pack is a sellable product made of components.
type Pack struct {
	bun.BaseModel `bun:"table:packs,alias:pa"`

	ID               string         `bun:"id,pk"`
	EstablishmentID  string         `bun:"establishment_id,notnull"`
	Code             string         `bun:"code,notnull"`
	Name             I18nText       `bun:"name,type:json,notnull"`
	Description      I18nText       `bun:"description,type:json,notnull"`
	Image            *string        `bun:"image"`
	Composition      map[string]int `bun:"composition,type:json,notnull"`
	RequiresSpotKind []string       `bun:"requires_spot_kind,type:json,nullzero"`
	AllowedZoneIDs   []string       `bun:"allowed_zone_ids,type:json,nullzero"`
	AllowedFormulas  []string       `bun:"allowed_formulas,type:json,notnull"`
	Channels         []string       `bun:"channels,type:json,notnull"`
	MaxPerBooking    int            `bun:"max_per_booking,notnull"`
	RequiresApproval bool           `bun:"requires_approval,notnull"`
	SortOrder        int            `bun:"sort_order,notnull"`
	Active           bool           `bun:"active,notnull"`
	// VATRateBP nil means the establishment default (settings.pricing).
	VATRateBP *int64    `bun:"vat_rate_bp"`
	Version   int64     `bun:"version,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

// Extra is an optional add-on to a booking line or to the booking.
type Extra struct {
	bun.BaseModel `bun:"table:extras,alias:ex"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	Code            string    `bun:"code,notnull"`
	Name            I18nText  `bun:"name,type:json,notnull"`
	Description     I18nText  `bun:"description,type:json,notnull"`
	ComponentCode   *string   `bun:"component_code"`
	PricingUnit     string    `bun:"pricing_unit,notnull"` // per_booking | per_day | per_slot
	MaxQuantity     int       `bun:"max_quantity,notnull"`
	AttachTo        string    `bun:"attach_to,notnull"` // line | booking
	AllowedPackIDs  []string  `bun:"allowed_pack_ids,type:json,nullzero"`
	Channels        []string  `bun:"channels,type:json,notnull"`
	SortOrder       int       `bun:"sort_order,notnull"`
	Active          bool      `bun:"active,notnull"`
	VATRateBP       *int64    `bun:"vat_rate_bp"`
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
	// M8 (00013, spec 30.4): shown in the spot setup on the operational map
	// with MapLabel ("L" = lettino, "S" = sdraio).
	ShowOnMap bool   `bun:"show_on_map,notnull"`
	MapLabel  string `bun:"map_label,notnull"`
}
