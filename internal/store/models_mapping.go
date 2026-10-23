package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Models of the mapping area (spec 6.2): maps, zones, spots, decorations and
// pools. Repository functions live in the area's own file (see
// internal/domain/mapping/doc.go for ownership).

// Shape is the geometry of a spot or decoration in logical map units.
type Shape struct {
	Type string  `json:"type"` // circle | rect
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// OnlineQuota limits what the online channel may sell in a zone (spec 7).
type OnlineQuota struct {
	Type  string `json:"type"` // percent | count
	Value int    `json:"value"`
}

// CapacityOverride changes a pool capacity on a date range.
type CapacityOverride struct {
	From     calendar.Date `json:"from"`
	To       calendar.Date `json:"to"`
	Capacity int           `json:"capacity"`
}

// Map is a graphical area (beach, pool). Version is bumped by every
// transactional save of the map document (spots and decorations included).
type Map struct {
	bun.BaseModel `bun:"table:maps,alias:m"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	Name            I18nText  `bun:"name,type:json,notnull"`
	Width           int       `bun:"width,notnull"`
	Height          int       `bun:"height,notnull"`
	GridSize        int       `bun:"grid_size,notnull"`
	Background      JSON      `bun:"background"`       // api.MapBackground document, NULL when absent
	SeaSide         string    `bun:"sea_side,notnull"` // top | bottom | left | right
	SortOrder       int       `bun:"sort_order,notnull"`
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// Zone groups spots; AssignmentMode is seat | zone | quantity (spec 5.2).
type Zone struct {
	bun.BaseModel `bun:"table:zones,alias:z"`

	ID              string       `bun:"id,pk"`
	EstablishmentID string       `bun:"establishment_id,notnull"`
	MapID           *string      `bun:"map_id"`
	Code            string       `bun:"code,notnull"`
	Name            I18nText     `bun:"name,type:json,notnull"`
	AssignmentMode  string       `bun:"assignment_mode,notnull"`
	OnlineVisible   bool         `bun:"online_visible,notnull"`
	OnlineQuota     *OnlineQuota `bun:"online_quota,type:json"`
	PriceCategory   string       `bun:"price_category,notnull"`
	AutoAssign      bool         `bun:"auto_assign,notnull"`
	SortOrder       int          `bun:"sort_order,notnull"`
	Version         int64        `bun:"version,notnull"`
	CreatedAt       time.Time    `bun:"created_at,notnull"`
	UpdatedAt       time.Time    `bun:"updated_at,notnull"`
}

// Spot is a bookable position on a map. Spots are versioned through their map.
type Spot struct {
	bun.BaseModel `bun:"table:spots,alias:sp"`

	ID                    string         `bun:"id,pk"`
	EstablishmentID       string         `bun:"establishment_id,notnull"`
	MapID                 string         `bun:"map_id,notnull"`
	ZoneID                string         `bun:"zone_id,notnull"`
	Label                 string         `bun:"label,notnull"`
	RowLabel              string         `bun:"row_label,notnull"`
	Number                int            `bun:"number,notnull"`
	Kind                  string         `bun:"kind,notnull"` // umbrella | gazebo | tent | palm | custom | sunbed | daybed | cabana | hammock
	Asset                 *string        `bun:"asset"`        // map kit illustration id (ADR 0012), NULL = default of the kind
	X                     float64        `bun:"x,notnull"`
	Y                     float64        `bun:"y,notnull"`
	Rotation              float64        `bun:"rotation,notnull"`
	Shape                 Shape          `bun:"shape,type:json,notnull"`
	Capacity              map[string]int `bun:"capacity,type:json,notnull"`
	MaxExtra              map[string]int `bun:"max_extra,type:json,notnull"`
	PriceCategoryOverride *string        `bun:"price_category_override"`
	Attributes            []string       `bun:"attributes,type:json,notnull"`
	Active                bool           `bun:"active,notnull"`
	CreatedAt             time.Time      `bun:"created_at,notnull"`
	UpdatedAt             time.Time      `bun:"updated_at,notnull"`
}

// MapDecoration is a non-bookable map element (walkway, shower, bar...).
type MapDecoration struct {
	bun.BaseModel `bun:"table:map_decorations,alias:md"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	MapID           string    `bun:"map_id,notnull"`
	Kind            string    `bun:"kind,notnull"`
	X               float64   `bun:"x,notnull"`
	Y               float64   `bun:"y,notnull"`
	Rotation        float64   `bun:"rotation,notnull"`
	Shape           Shape     `bun:"shape,type:json,notnull"`
	Label           I18nText  `bun:"label,type:json,nullzero"`
	Asset           *string   `bun:"asset"`   // map kit illustration id (ADR 0012), NULL = default of the kind
	Variant         *string   `bun:"variant"` // colour variant of the illustration (e.g. sea flag)
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// Pool is quantity inventory without position. SystemManaged pools are the
// implicit capacity pools of `zone` zones (spec 7.5), maintained by the
// mapping service: capacity = number of active spots of the zone.
type Pool struct {
	bun.BaseModel `bun:"table:pools,alias:pl"`

	ID                string             `bun:"id,pk"`
	EstablishmentID   string             `bun:"establishment_id,notnull"`
	ZoneID            *string            `bun:"zone_id"`
	ComponentCode     string             `bun:"component_code,notnull"`
	Name              I18nText           `bun:"name,type:json,notnull"`
	Capacity          int                `bun:"capacity,notnull"`
	CapacityOverrides []CapacityOverride `bun:"capacity_overrides,type:json,notnull"`
	SystemManaged     bool               `bun:"system_managed,notnull"`
	Version           int64              `bun:"version,notnull"`
	CreatedAt         time.Time          `bun:"created_at,notnull"`
	UpdatedAt         time.Time          `bun:"updated_at,notnull"`
}
