package booking

import (
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
)

// ExtraInput is an extra with its quantity.
type ExtraInput struct {
	ExtraID  string
	Quantity int
}

// LineInput is one pack of a booking request (api.BookingLineInput).
type LineInput struct {
	PackID   string
	ZoneID   string
	SpotID   string
	Quantity int
	Extras   []ExtraInput
}

// CreateInput is a booking creation (api.BookingCreate) plus the fields set
// only by internal callers (season passes, renewals).
type CreateInput struct {
	CustomerID    string
	Customer      *customer.Input
	Channel       string // default desk
	StartsOn      calendar.Date
	EndsOn        calendar.Date
	Slots         calendar.Slots
	Formula       string
	Lines         []LineInput
	Extras        []ExtraInput
	PromoCode     string
	AgreementID   string
	NotesCustomer string
	NotesInternal string
	HoldID        string
	// Status is the initial status: StatusDraft, StatusHeld or StatusConfirmed
	// (default).
	Status Status
	// HoldTTL overrides the hold duration for StatusHeld (default ARENILE_HOLD_TTL).
	HoldTTL time.Duration
	// Force ignores policy violations (panel only, audited).
	Force bool

	// Internal callers only.
	SeasonID           string
	IsSeasonPass       bool
	AllocationKind     inventory.Kind // default KindBooking (KindHold for StatusHeld); KindSeason for passes
	RenewalOfBookingID string
}

// PatchInput is a modification (api.BookingPatch): nil fields are unchanged,
// Lines replaces every line.
type PatchInput struct {
	CustomerID    *string
	StartsOn      *calendar.Date
	EndsOn        *calendar.Date
	Slots         *calendar.Slots
	Formula       *string
	Lines         []LineInput
	Extras        []ExtraInput
	NotesCustomer *string
	NotesInternal *string
	Force         bool
	DryRun        bool
}

// Detail is a booking with everything the detail page shows.
type Detail struct {
	Booking         store.Booking
	Customer        store.Customer
	Lines           []store.BookingLine
	Extras          []store.BookingLineExtra
	SpotAllocations []store.SpotAllocation
	PoolAllocations []store.PoolAllocation
	Payments        []store.Payment
	// History is read from audit_log (entity_type booking, entity_id = id).
	History []store.AuditEntry
}
