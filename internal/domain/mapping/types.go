package mapping

import (
	"fmt"
	"strings"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Enumerations of the mapping area (spec 6.2).
const (
	ModeSeat     = "seat"
	ModeZone     = "zone"
	ModeQuantity = "quantity"

	QuotaPercent = "percent"
	QuotaCount   = "count"

	ShapeCircle = "circle"
	ShapeRect   = "rect"

	SeaTop    = "top"
	SeaBottom = "bottom"
	SeaLeft   = "left"
	SeaRight  = "right"

	// SystemPoolComponentCode is the component code of the implicit pools of
	// `zone` zones: the pool counts spots, whatever their components.
	SystemPoolComponentCode = "spot"
)

// Limits of one map document (spec 11.1: 1,000 elements must stay fluid;
// the bound leaves room while keeping a save request reasonable).
const (
	MaxSpotsPerMap       = 5000
	MaxDecorationsPerMap = 2000
	MaxAttributes        = 32
	MaxCapacityOverrides = 100
)

var (
	// SpotKinds are the allowed spot kinds.
	SpotKinds = []string{"umbrella", "gazebo", "tent", "palm", "custom", "sunbed", "daybed", "cabana", "hammock"}
	// DecorationKinds are the allowed decoration kinds.
	DecorationKinds = []string{"walkway", "shower", "lifeguard", "bar", "entrance", "cabin_block", "text", "custom"}
	// AssignmentModes are the zone assignment modes (spec 5.2).
	AssignmentModes = []string{ModeSeat, ModeZone, ModeQuantity}
	// SeaSides are the allowed map sea sides.
	SeaSides = []string{SeaTop, SeaBottom, SeaLeft, SeaRight}
)

// Errors of the mapping package. They wrap the configerr kinds, which the
// HTTP layer maps to problem types.
var (
	ErrMapNotFound        = fmt.Errorf("%w: map", configerr.ErrNotFound)
	ErrZoneNotFound       = fmt.Errorf("%w: zone", configerr.ErrNotFound)
	ErrPoolNotFound       = fmt.Errorf("%w: pool", configerr.ErrNotFound)
	ErrZoneCodeTaken      = fmt.Errorf("%w: zone code already used in this establishment", configerr.ErrConflict)
	ErrZoneHasSpots       = fmt.Errorf("%w: the zone still has spots", configerr.ErrConflict)
	ErrZoneHasPools       = fmt.Errorf("%w: the zone has bound pools", configerr.ErrConflict)
	ErrZoneReferenced     = fmt.Errorf("%w: the zone is referenced by price rules, promo codes, agreements, packs or booking policies", configerr.ErrConflict)
	ErrSystemPoolReadOnly = fmt.Errorf("%w: system managed pools are read-only", configerr.ErrConflict)
	ErrLabelsTaken        = fmt.Errorf("%w: generated labels already exist on the map", configerr.ErrConflict)
	ErrSpotHasAllocations = configerr.ErrSpotHasFutureAllocations
)

// Background is the optional map background (spec 11.2); its JSON shape is
// the API MapBackground and is stored as is in maps.background.
type Background struct {
	Type     string         `json:"type"` // image | geojson
	URL      string         `json:"url,omitempty"`
	GeoJSON  map[string]any `json:"geojson,omitempty"`
	Scale    *float64       `json:"scale,omitempty"`
	Rotation *float64       `json:"rotation,omitempty"`
	OffsetX  *float64       `json:"offset_x,omitempty"`
	OffsetY  *float64       `json:"offset_y,omitempty"`
	Opacity  *float64       `json:"opacity,omitempty"`
}

// MapInput is the replaceable part of a map (create and save).
type MapInput struct {
	Name       store.I18nText
	Width      int
	Height     int
	GridSize   int
	Background *Background
	SeaSide    string
	SortOrder  int
}

// SpotInput is one spot of a saved document; ID empty means a new spot.
type SpotInput struct {
	ID                    string
	ZoneID                string
	Label                 string
	RowLabel              string
	Number                int
	Kind                  string
	Asset                 string // map kit illustration id; empty = default of the kind
	X, Y, Rotation        float64
	Shape                 store.Shape
	Capacity              map[string]int
	MaxExtra              map[string]int
	PriceCategoryOverride *string
	Attributes            []string
	Active                bool
	// OnlineExcluded is the negation of the API flag online_bookable (spec
	// 30.11, INV-105): the zero value keeps the spot on sale online.
	OnlineExcluded bool
}

// DecorationInput is one decoration of a saved document; ID empty means new.
type DecorationInput struct {
	ID             string
	Kind           string
	Asset          string // map kit illustration id; empty = default of the kind
	Variant        string // colour variant of the illustration; empty = default
	X, Y, Rotation float64
	Shape          store.Shape
	Label          store.I18nText
}

// SaveInput is the full map document of PUT /maps/{id}: spots and
// decorations absent from it are deleted.
type SaveInput struct {
	Map         MapInput
	Spots       []SpotInput
	Decorations []DecorationInput
}

// MapView is a map with its spot count.
type MapView struct {
	Map       store.Map
	SpotCount int
}

// Document is the full map document; Map.Map.Version is the ETag.
type Document struct {
	MapView
	Spots       []store.Spot
	Decorations []store.MapDecoration
}

// ZoneInput is the replaceable part of a zone.
type ZoneInput struct {
	MapID          *string
	Code           string
	Name           store.I18nText
	AssignmentMode string
	OnlineVisible  bool
	OnlineQuota    *store.OnlineQuota
	PriceCategory  string
	AutoAssign     bool
	SortOrder      int
}

// ZoneView is a zone with its active spot count and its pool: the implicit
// pool of a `zone` zone or the first pool bound to a `quantity` zone.
type ZoneView struct {
	Zone      store.Zone
	SpotCount int
	PoolID    *string
}

// PoolInput is the replaceable part of a pool.
type PoolInput struct {
	ZoneID            *string
	ComponentCode     string
	Name              store.I18nText
	Capacity          int
	CapacityOverrides []store.CapacityOverride
}

// GenerateResult is the outcome of the row generator: the generated spots,
// the labels already present on the map and, when applied, the new document.
type GenerateResult struct {
	Spots             []SpotInput
	ConflictingLabels []string
	Applied           bool
	Document          *Document
}

// ZoneReference is a configuration entity that names a zone: a price rule,
// promo code or agreement (conditions.zone_ids), a pack (allowed_zone_ids)
// or a booking policy (scope.zones, by id or code).
type ZoneReference struct {
	// Kind is price_rule | promo_code | agreement | pack | booking_policy.
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Name is the rule, agreement or policy name, or the promo/pack code.
	Name string `json:"name"`
}

// ZoneReferencedError refuses the deletion of a zone (or of its map) while
// other configuration entities reference it; References lists them so that
// the panel can show what to change first. It Is configerr.ErrConflict.
type ZoneReferencedError struct {
	ZoneID     string
	References []ZoneReference
}

func (e *ZoneReferencedError) Error() string {
	parts := make([]string, len(e.References))
	for i, r := range e.References {
		parts[i] = fmt.Sprintf("%s %q (%s)", r.Kind, r.Name, r.ID)
	}
	return fmt.Sprintf("%v: %s", ErrZoneReferenced, strings.Join(parts, ", "))
}

// Is reports ErrZoneReferenced and configerr.ErrConflict.
func (e *ZoneReferencedError) Is(target error) bool {
	return target == ErrZoneReferenced || target == configerr.ErrConflict
}
