package inventory

import (
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
)

// Kind is the kind of the allocations to create (store.Allocation*).
type Kind string

// Allocation kinds created through AllocateRequest.
const (
	KindHold    Kind = "hold"
	KindBooking Kind = "booking"
	KindSeason  Kind = "season"
)

// LineRequest is one resource to occupy for the whole interval.
type LineRequest struct {
	// LineID is the booking_lines.id owning the rows (empty for panel holds).
	LineID string
	// ZoneID is the zone of the line; its assignment mode decides what is
	// consumed: seat -> SpotID rows; zone -> implicit zone pool (+ SpotID
	// rows when already assigned); quantity -> the zone pool, Quantity units.
	ZoneID string
	// SpotID is required for seat zones, optional for zone-mode zones.
	SpotID string
	// PoolID consumes Quantity units of a pool directly (panel hold items
	// on a pool); ZoneID and SpotID are then empty.
	PoolID string
	// Quantity is the number of units (quantity zones), default 1.
	Quantity int
	// Components are the tracked components consumed from their pools
	// (pack composition x quantity + extras with component_code), keyed by
	// component code.
	Components map[string]int
}

// AllocateRequest occupies resources for a booking or a panel hold.
// Exactly one of BookingID and HoldID is set.
type AllocateRequest struct {
	BookingID string
	HoldID    string
	Channel   string // desk | online | api (quota applies to online only)
	StartsOn  calendar.Date
	EndsOn    calendar.Date
	Slots     calendar.Slots
	Kind      Kind
	// ExpiresAt is required iff Kind == KindHold (INV-06).
	ExpiresAt *time.Time
	Lines     []LineRequest
	// AgreementID is the agreement of the booking (M8): its quota counts
	// this sale; every other reserve is protected from it (INV-103).
	AgreementID string
	// Skip leaves these half-days out of the allocation: the days a season
	// pass holder released stay free when the pass is moved or swapped
	// (INV-10). Half-days outside the interval are ignored.
	Skip []availability.DateSlot
	// ByDesk marks an operation of the cash desk (a panel user moving,
	// swapping or editing a booking of any channel): the spots excluded
	// from online sale are allowed, "la cassa le vede tutte" (spec 30.11);
	// the online quotas still follow Channel.
	ByDesk bool
}

// LineAllocation is the outcome for one line.
type LineAllocation struct {
	LineID string
	// SpotID is the spot occupied (seat zones and assigned zone lines).
	SpotID string
	// AssignmentStatus is store.AssignmentAssigned, AssignmentPending or
	// AssignmentNotApplicable.
	AssignmentStatus string
}

// Allocation summarises what Allocate, Release or ConvertHold changed; the
// caller uses SpotIDs, PoolIDs and Dates for the outbox event hints.
type Allocation struct {
	Lines    []LineAllocation
	SpotRows int
	PoolRows int
	SpotIDs  []string
	PoolIDs  []string
	Dates    []calendar.Date
}

// ReleaseRequest frees the allocations of a booking (all lines, or LineIDs),
// optionally only from date From onwards (no-show, mid-stay moves).
type ReleaseRequest struct {
	BookingID string
	LineIDs   []string
	From      *calendar.Date
}

// ConvertHoldRequest turns a panel hold into the allocations of a booking:
// the hold rows are re-pointed to the booking lines (Lines[i].LineID, same
// resources and interval as the hold items) with Kind and ExpiresAt, in the
// caller's transaction. An expired hold fails with availability.ErrHoldExpired.
type ConvertHoldRequest struct {
	HoldID    string
	BookingID string
	Kind      Kind
	ExpiresAt *time.Time
	Lines     []LineRequest
}

// SeasonDays identifies half-days of a season line (pass releases).
type SeasonDays struct {
	BookingID     string
	BookingLineID string
	SpotID        string
	Days          []availability.DateSlot
}
