package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Models of the season area (spec 6.1): seasons, closure days, rate periods.
// Repository functions live in the area's own file (see
// internal/domain/season/doc.go for ownership).

// I18nText is a localized text: locale -> text (spec 20). Readers fall back
// to the establishment default locale when the requested one is missing.
type I18nText map[string]string

// DateRange is an inclusive range of calendar dates.
type DateRange struct {
	From calendar.Date `json:"from"`
	To   calendar.Date `json:"to"`
}

// Season is the yearly opening period of an establishment.
type Season struct {
	bun.BaseModel `bun:"table:seasons,alias:sn"`

	ID              string        `bun:"id,pk"`
	EstablishmentID string        `bun:"establishment_id,notnull"`
	Year            int           `bun:"year,notnull"`
	OpensOn         calendar.Date `bun:"opens_on,notnull"`
	ClosesOn        calendar.Date `bun:"closes_on,notnull"`
	Status          string        `bun:"status,notnull"` // draft | active | closed
	Version         int64         `bun:"version,notnull"`
	CreatedAt       time.Time     `bun:"created_at,notnull"`
	UpdatedAt       time.Time     `bun:"updated_at,notnull"`
}

// ClosureDay is an extraordinary closure (bad weather, events).
type ClosureDay struct {
	bun.BaseModel `bun:"table:closure_days,alias:cd"`

	ID              string          `bun:"id,pk"`
	EstablishmentID string          `bun:"establishment_id,notnull"`
	Date            calendar.Date   `bun:"date,notnull"`
	Slots           []calendar.Slot `bun:"slots,type:json,notnull"`
	Reason          string          `bun:"reason,notnull"`
	Version         int64           `bun:"version,notnull"`
	CreatedAt       time.Time       `bun:"created_at,notnull"`
	UpdatedAt       time.Time       `bun:"updated_at,notnull"`
}

// RatePeriod is a named set of date ranges of a season used by price rules.
// Periods of the same season never overlap (checked by the season service).
type RatePeriod struct {
	bun.BaseModel `bun:"table:rate_periods,alias:rp"`

	ID              string      `bun:"id,pk"`
	EstablishmentID string      `bun:"establishment_id,notnull"`
	SeasonID        string      `bun:"season_id,notnull"`
	Code            string      `bun:"code,notnull"`
	Name            I18nText    `bun:"name,type:json,notnull"`
	Color           string      `bun:"color,notnull"`
	Ranges          []DateRange `bun:"ranges,type:json,notnull"`
	SortOrder       int         `bun:"sort_order,notnull"`
	Version         int64       `bun:"version,notnull"`
	CreatedAt       time.Time   `bun:"created_at,notnull"`
	UpdatedAt       time.Time   `bun:"updated_at,notnull"`
}
