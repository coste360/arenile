package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Models of the pricing configuration area (spec 6.4): price rules, fixed
// duration prices, promo codes, agreements and booking policies.
// Repository functions live in the area's own file (see
// internal/domain/pricingconfig/doc.go for ownership).
//
// Conditions, Action, Scope and Rules hold the JSON documents of the pure
// engines (pricing.Conditions, pricing.Action, policy.Scope, policy.Rules):
// the store does not interpret them.

// PriceRule is a rule of the price list of a season (spec 8).
type PriceRule struct {
	bun.BaseModel `bun:"table:price_rules,alias:pr"`

	ID              string   `bun:"id,pk"`
	EstablishmentID string   `bun:"establishment_id,notnull"`
	SeasonID        string   `bun:"season_id,notnull"`
	Name            string   `bun:"name,notnull"`
	TargetType      string   `bun:"target_type,notnull"` // pack | extra
	TargetIDs       []string `bun:"target_ids,type:json,nullzero"`
	Conditions      JSON     `bun:"conditions,notnull"`
	Action          JSON     `bun:"action,notnull"`
	Priority        int      `bun:"priority,notnull"`
	Stage           string   `bun:"stage,notnull"` // base | modifier | discount
	Stackable       bool     `bun:"stackable,notnull"`
	Active          bool     `bun:"active,notnull"`
	// Seq orders rules by recency (higher = more recent), the last tie-break
	// of spec 8.4. Assign max(seq)+1 of the establishment on insert and update.
	Seq       int64     `bun:"seq,notnull"`
	Version   int64     `bun:"version,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

// FixedDurationPrice is a package price for week/fortnight/month/season.
type FixedDurationPrice struct {
	bun.BaseModel `bun:"table:fixed_duration_prices,alias:fp"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	SeasonID        string    `bun:"season_id,notnull"`
	PackID          string    `bun:"pack_id,notnull"`
	Formula         string    `bun:"formula,notnull"`
	PriceCategory   string    `bun:"price_category,notnull"`
	RatePeriodCode  *string   `bun:"rate_period_code"`
	Amount          int64     `bun:"amount,notnull"` // cents
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// PromoCode is a discount code, unique per establishment ignoring case.
type PromoCode struct {
	bun.BaseModel `bun:"table:promo_codes,alias:pc"`

	ID                 string         `bun:"id,pk"`
	EstablishmentID    string         `bun:"establishment_id,notnull"`
	Code               string         `bun:"code,notnull"`
	Description        string         `bun:"description,notnull"`
	Action             JSON           `bun:"action,notnull"`
	Conditions         JSON           `bun:"conditions,notnull"`
	ValidFrom          *calendar.Date `bun:"valid_from"`
	ValidTo            *calendar.Date `bun:"valid_to"`
	MaxUses            *int           `bun:"max_uses"`
	MaxUsesPerCustomer *int           `bun:"max_uses_per_customer"`
	Uses               int            `bun:"uses,notnull"`
	Stackable          bool           `bun:"stackable,notnull"`
	Active             bool           `bun:"active,notnull"`
	Version            int64          `bun:"version,notnull"`
	CreatedAt          time.Time      `bun:"created_at,notnull"`
	UpdatedAt          time.Time      `bun:"updated_at,notnull"`
}

// Agreement is a convention (convenzione) for a customer or a partner.
type Agreement struct {
	bun.BaseModel `bun:"table:agreements,alias:ag"`

	ID              string         `bun:"id,pk"`
	EstablishmentID string         `bun:"establishment_id,notnull"`
	Code            *string        `bun:"code"`
	Name            string         `bun:"name,notnull"`
	PartnerName     string         `bun:"partner_name,notnull"`
	Notes           string         `bun:"notes,notnull"`
	Action          JSON           `bun:"action,notnull"`
	Conditions      JSON           `bun:"conditions,notnull"`
	ValidFrom       *calendar.Date `bun:"valid_from"`
	ValidTo         *calendar.Date `bun:"valid_to"`
	Stackable       bool           `bun:"stackable,notnull"`
	Active          bool           `bun:"active,notnull"`
	Version         int64          `bun:"version,notnull"`
	CreatedAt       time.Time      `bun:"created_at,notnull"`
	UpdatedAt       time.Time      `bun:"updated_at,notnull"`
}

// BookingPolicy is a scoped set of booking constraints of a season (spec 9).
type BookingPolicy struct {
	bun.BaseModel `bun:"table:booking_policies,alias:bp"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	SeasonID        string    `bun:"season_id,notnull"`
	Name            string    `bun:"name,notnull"`
	Scope           JSON      `bun:"scope,notnull"`
	Rules           JSON      `bun:"rules,notnull"`
	Priority        int       `bun:"priority,notnull"`
	Active          bool      `bun:"active,notnull"`
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}
