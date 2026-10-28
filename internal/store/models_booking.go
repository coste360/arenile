package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Models of bookings and availability (spec 6.6, 7, 10). Ownership:
//   - bookings, booking_lines, booking_line_extras: internal/domain/booking
//     (repository in internal/store/booking.go);
//   - holds, spot_blocks, spot_allocations, pool_usage, pool_allocations:
//     internal/domain/inventory (repository in internal/store/inventory.go);
//   - season_releases: internal/domain/pass (repository in internal/store/pass.go).
//
// Availability invariants live in the schema (migration 00003, ADR 0003,
// ADR 0010): unique (spot_id, date, slot) on spot_allocations; releasing
// resources deletes allocation rows; pool_usage is changed only by the
// conditional UPDATE of the inventory repository.

// Booking channels (bookings.channel, holds.channel, pool_allocations.channel).
const (
	ChannelDesk   = "desk"
	ChannelOnline = "online"
	ChannelAPI    = "api"
)

// Allocation kinds (spot_allocations.kind, pool_allocations.kind).
const (
	AllocationBooking = "booking"
	AllocationHold    = "hold"
	AllocationBlock   = "block" // spot_allocations only
	AllocationSeason  = "season"
)

// Line assignment statuses (booking_lines.assignment_status).
const (
	AssignmentAssigned      = "assigned"
	AssignmentPending       = "pending"
	AssignmentNotApplicable = "not_applicable"
)

// Booking is a booking or a season pass (IsSeasonPass, formula season).
// Status follows the state machine of internal/domain/booking/status.go.
// PriceSnapshot is the pricing.PriceBreakdown and PolicySnapshot the applied
// policy ({policy_id, rules}) at creation: both are immutable after
// confirmation (INV-21). PriceCurrent is the breakdown of the latest
// re-quote of a booking modified after confirmation (nil until then, ADR
// 0011): TotalAmount always equals the total of CurrentPrice() (INV-12).
// PaidAmount = succeeded payments in - refunds (INV-22).
// Version is the optimistic concurrency version (ETag / If-Match).
type Booking struct {
	bun.BaseModel `bun:"table:bookings,alias:bk"`

	ID                 string         `bun:"id,pk"`
	EstablishmentID    string         `bun:"establishment_id,notnull"`
	Code               string         `bun:"code,notnull"`
	CustomerID         string         `bun:"customer_id,notnull"`
	Channel            string         `bun:"channel,notnull"`
	Status             string         `bun:"status,notnull"`
	StartsOn           calendar.Date  `bun:"starts_on,notnull"`
	EndsOn             calendar.Date  `bun:"ends_on,notnull"`
	Slots              calendar.Slots `bun:"slots,notnull"`
	Formula            string         `bun:"formula,notnull"`
	SeasonID           *string        `bun:"season_id"`
	IsSeasonPass       bool           `bun:"is_season_pass,notnull"`
	RenewalOfBookingID *string        `bun:"renewal_of_booking_id"`
	PriceSnapshot      JSON           `bun:"price_snapshot,notnull"`
	PriceCurrent       JSON           `bun:"price_current,nullzero"`
	TotalAmount        money.Cents    `bun:"total_amount,notnull"`
	PaidAmount         money.Cents    `bun:"paid_amount,notnull"`
	DepositDue         money.Cents    `bun:"deposit_due,notnull"`
	PolicySnapshot     JSON           `bun:"policy_snapshot,notnull"`
	PromoCodeID        *string        `bun:"promo_code_id"`
	AgreementID        *string        `bun:"agreement_id"`
	HoldExpiresAt      *time.Time     `bun:"hold_expires_at"`
	HoldID             *string        `bun:"hold_id"`
	ManageTokenHash    *string        `bun:"manage_token_hash"`
	TermsVersion       *string        `bun:"terms_version"`
	// TermsAcceptedAt is when the customer accepted TermsVersion (00005, spec 23).
	TermsAcceptedAt *time.Time `bun:"terms_accepted_at"`
	// Locale is the language of the customer communications (00005); nil = customer locale.
	Locale          *string    `bun:"locale"`
	NotesCustomer   string     `bun:"notes_customer,notnull"`
	NotesInternal   string     `bun:"notes_internal,notnull"`
	CreatedByUserID *string    `bun:"created_by_user_id"`
	ConfirmedAt     *time.Time `bun:"confirmed_at"`
	CheckedInAt     *time.Time `bun:"checked_in_at"`
	CancelledAt     *time.Time `bun:"cancelled_at"`
	CancelReason    *string    `bun:"cancel_reason"`
	// Test marks a booking created in test mode (00009, spec 15.4): excluded
	// from reports, KPIs, corrispettivi and exports; purged in bulk.
	Test bool `bun:"test,notnull"`
	// Manual price of a season pass (00009): the total decided by the
	// manager, the engine total it replaced, the reason and who/when.
	PriceOverrideAmount       *money.Cents `bun:"price_override_amount"`
	PriceOverrideEngineAmount *money.Cents `bun:"price_override_engine_amount"`
	PriceOverrideReason       *string      `bun:"price_override_reason"`
	PriceOverrideByUserID     *string      `bun:"price_override_by_user_id"`
	PriceOverrideAt           *time.Time   `bun:"price_override_at"`
	// M8 (00013, spec 30.2.4, 30.3): partner of provenance, group and party
	// (people feed the attendance in reports), optional guest names (no
	// other guest data), the partner share (type and value: basis points
	// for percent, cents for amount) and the resolved PartnerAmount, kept
	// with TotalAmount in the same transaction (statement, INV-104).
	PartnerID         *string        `bun:"partner_id"`
	GroupName         string         `bun:"group_name,notnull"`
	GroupOrigin       string         `bun:"group_origin,notnull"`
	PartyAdults       *int           `bun:"party_adults"`
	PartyChildren     *int           `bun:"party_children"`
	GuestNames        []string       `bun:"guest_names,type:json,nullzero"`
	PartnerShareType  *string        `bun:"partner_share_type"`
	PartnerShareValue *int64         `bun:"partner_share_value"`
	PartnerAmount     money.Cents    `bun:"partner_amount,notnull"`
	PartnerStayFrom   *calendar.Date `bun:"partner_stay_from"`
	PartnerStayTo     *calendar.Date `bun:"partner_stay_to"`
	Version           int64          `bun:"version,notnull"`
	CreatedAt         time.Time      `bun:"created_at,notnull"`
	UpdatedAt         time.Time      `bun:"updated_at,notnull"`
}

// BalanceDue is what remains to be collected (never negative).
func (b *Booking) BalanceDue() money.Cents { return BalanceDue(b.Status, b.TotalAmount, b.PaidAmount) }

// BalanceDue is what remains to be collected on a booking in status: the
// total less what was paid, never negative, and nothing on a booking that
// was cancelled, expired or rejected (what it retained is final; a
// cancellation never bills more afterwards).
func BalanceDue(status string, total, paid money.Cents) money.Cents {
	switch status {
	case "cancelled", "expired", "rejected":
		return 0
	}
	return max(total-paid, 0)
}

// CurrentPrice is the breakdown that prices the booking today: PriceCurrent
// after a modification following the confirmation, PriceSnapshot otherwise
// (payments split their VAT on it, INV-16).
func (b *Booking) CurrentPrice() JSON {
	if len(b.PriceCurrent) > 0 {
		return b.PriceCurrent
	}
	return b.PriceSnapshot
}

// BookingLine is one pack of a booking on a zone, optionally on a spot.
// Lines share the dates and slots of their booking.
type BookingLine struct {
	bun.BaseModel `bun:"table:booking_lines,alias:bl"`

	ID               string      `bun:"id,pk"`
	EstablishmentID  string      `bun:"establishment_id,notnull"`
	BookingID        string      `bun:"booking_id,notnull"`
	PackID           string      `bun:"pack_id,notnull"`
	ZoneID           string      `bun:"zone_id,notnull"`
	SpotID           *string     `bun:"spot_id"`
	AssignmentStatus string      `bun:"assignment_status,notnull"`
	Quantity         int         `bun:"quantity,notnull"`
	PriceCategory    string      `bun:"price_category,notnull"`
	LineTotal        money.Cents `bun:"line_total,notnull"`
	SortOrder        int         `bun:"sort_order,notnull"`
	CreatedAt        time.Time   `bun:"created_at,notnull"`
	UpdatedAt        time.Time   `bun:"updated_at,notnull"`
}

// BookingLineExtra is an extra of a line (BookingLineID set) or of the
// whole booking (BookingLineID nil).
type BookingLineExtra struct {
	bun.BaseModel `bun:"table:booking_line_extras,alias:ble"`

	ID              string      `bun:"id,pk"`
	EstablishmentID string      `bun:"establishment_id,notnull"`
	BookingID       string      `bun:"booking_id,notnull"`
	BookingLineID   *string     `bun:"booking_line_id"`
	ExtraID         string      `bun:"extra_id,notnull"`
	Quantity        int         `bun:"quantity,notnull"`
	Total           money.Cents `bun:"total,notnull"`
	CreatedAt       time.Time   `bun:"created_at,notnull"`
	UpdatedAt       time.Time   `bun:"updated_at,notnull"`
}

// HoldItem is one resource of a panel hold (spot, zone capacity or pool).
type HoldItem struct {
	SpotID   string `json:"spot_id,omitempty"`
	ZoneID   string `json:"zone_id,omitempty"`
	PoolID   string `json:"pool_id,omitempty"`
	Quantity int    `json:"quantity,omitempty"`
}

// Hold is a temporary reservation of a selection made from the panel before
// the booking exists (spec 7.4). Its allocations (kind = hold) reference
// hold_id and carry ExpiresAt; converting the hold into a booking re-points
// them to booking lines in one transaction. Expired holds are ignored and
// deleted on conflict (INV-05).
type Hold struct {
	bun.BaseModel `bun:"table:holds,alias:ho"`

	ID                 string         `bun:"id,pk"`
	EstablishmentID    string         `bun:"establishment_id,notnull"`
	Channel            string         `bun:"channel,notnull"`
	StartsOn           calendar.Date  `bun:"starts_on,notnull"`
	EndsOn             calendar.Date  `bun:"ends_on,notnull"`
	Slots              calendar.Slots `bun:"slots,notnull"`
	Items              []HoldItem     `bun:"items,type:json,notnull"`
	Note               string         `bun:"note,notnull"`
	ExpiresAt          time.Time      `bun:"expires_at,notnull"`
	ReleasedAt         *time.Time     `bun:"released_at"`
	ConvertedBookingID *string        `bun:"converted_booking_id"`
	CreatedByUserID    *string        `bun:"created_by_user_id"`
	CreatedAt          time.Time      `bun:"created_at,notnull"`
	UpdatedAt          time.Time      `bun:"updated_at,notnull"`
}

// SpotBlock is a manual block of a spot on a date range (allocations kind = block).
type SpotBlock struct {
	bun.BaseModel `bun:"table:spot_blocks,alias:sb"`

	ID              string         `bun:"id,pk"`
	EstablishmentID string         `bun:"establishment_id,notnull"`
	SpotID          string         `bun:"spot_id,notnull"`
	StartsOn        calendar.Date  `bun:"starts_on,notnull"`
	EndsOn          calendar.Date  `bun:"ends_on,notnull"`
	Slots           calendar.Slots `bun:"slots,notnull"`
	Reason          string         `bun:"reason,notnull"`
	CreatedByUserID *string        `bun:"created_by_user_id"`
	CreatedAt       time.Time      `bun:"created_at,notnull"`
	UpdatedAt       time.Time      `bun:"updated_at,notnull"`
}

// SpotAllocation occupies one spot on one half-day (spec 7.2). Exactly one
// owner is set: BookingLineID (+BookingID) for kind booking/season and for
// booking holds, HoldID for panel holds, BlockID for blocks. ExpiresAt is set
// iff Kind = hold (INV-06).
type SpotAllocation struct {
	bun.BaseModel `bun:"table:spot_allocations,alias:sa"`

	ID              string        `bun:"id,pk"`
	EstablishmentID string        `bun:"establishment_id,notnull"`
	SpotID          string        `bun:"spot_id,notnull"`
	Date            calendar.Date `bun:"date,notnull"`
	Slot            calendar.Slot `bun:"slot,notnull"`
	Kind            string        `bun:"kind,notnull"`
	BookingID       *string       `bun:"booking_id"`
	BookingLineID   *string       `bun:"booking_line_id"`
	HoldID          *string       `bun:"hold_id"`
	BlockID         *string       `bun:"block_id"`
	ExpiresAt       *time.Time    `bun:"expires_at"`
	CreatedAt       time.Time     `bun:"created_at,notnull"`
}

// PoolUsage is the counter of one pool on one half-day. Capacity is not
// stored: the inventory repository passes the effective capacity (pool
// capacity corrected by capacity_overrides) to the conditional UPDATE.
type PoolUsage struct {
	bun.BaseModel `bun:"table:pool_usage,alias:pu"`

	EstablishmentID string        `bun:"establishment_id,notnull"`
	PoolID          string        `bun:"pool_id,pk"`
	Date            calendar.Date `bun:"date,pk"`
	Slot            calendar.Slot `bun:"slot,pk"`
	Used            int           `bun:"used,notnull"`
	Held            int           `bun:"held,notnull"`
	OnlineUsed      int           `bun:"online_used,notnull"`
	UpdatedAt       time.Time     `bun:"updated_at,notnull"`
}

// PoolAllocation is the per-owner detail of pool consumption (INV-03).
type PoolAllocation struct {
	bun.BaseModel `bun:"table:pool_allocations,alias:pla"`

	ID              string        `bun:"id,pk"`
	EstablishmentID string        `bun:"establishment_id,notnull"`
	PoolID          string        `bun:"pool_id,notnull"`
	Date            calendar.Date `bun:"date,notnull"`
	Slot            calendar.Slot `bun:"slot,notnull"`
	Kind            string        `bun:"kind,notnull"`
	Channel         string        `bun:"channel,notnull"`
	Quantity        int           `bun:"quantity,notnull"`
	BookingID       *string       `bun:"booking_id"`
	BookingLineID   *string       `bun:"booking_line_id"`
	HoldID          *string       `bun:"hold_id"`
	ExpiresAt       *time.Time    `bun:"expires_at"`
	CreatedAt       time.Time     `bun:"created_at,notnull"`
}

// SeasonRelease is one half-day released by a season pass holder (spec 7.7).
// While RevokedAt is nil the season allocation of that half-day does not
// exist (INV-10); ResoldBookingLineID is set when the day is sold again.
type SeasonRelease struct {
	bun.BaseModel `bun:"table:season_releases,alias:sr"`

	ID                  string        `bun:"id,pk"`
	EstablishmentID     string        `bun:"establishment_id,notnull"`
	BookingID           string        `bun:"booking_id,notnull"`
	BookingLineID       string        `bun:"booking_line_id,notnull"`
	SpotID              string        `bun:"spot_id,notnull"`
	Date                calendar.Date `bun:"date,notnull"`
	Slot                calendar.Slot `bun:"slot,notnull"`
	ResoldBookingLineID *string       `bun:"resold_booking_line_id"`
	CreditAmount        money.Cents   `bun:"credit_amount,notnull"`
	Note                string        `bun:"note,notnull"`
	CreatedByUserID     *string       `bun:"created_by_user_id"`
	RevokedAt           *time.Time    `bun:"revoked_at"`
	CreatedAt           time.Time     `bun:"created_at,notnull"`
	UpdatedAt           time.Time     `bun:"updated_at,notnull"`
}
