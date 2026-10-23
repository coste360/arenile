package booking

import (
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/money"
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
	CustomerID *string
	StartsOn   *calendar.Date
	EndsOn     *calendar.Date
	Slots      *calendar.Slots
	Formula    *string
	Lines      []LineInput
	Extras     []ExtraInput
	// ExtrasSet tells an explicit (possibly empty) Extras from "unchanged".
	ExtrasSet     bool
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
	// SpotLabels, PackCodes and ExtraCodes label the ids of the lines.
	SpotLabels map[string]string
	PackCodes  map[string]string
	ExtraCodes map[string]string
}

// Quote is the preview of a creation (no writes): price, policy and the
// conflicts the allocation would hit.
type Quote struct {
	Result    *pricingconfig.SimulateResult
	Conflicts []availability.Conflict
}

// ChangeResult is the outcome of a modification or a move (dry run or
// applied): the booking (as it would be, for a dry run), the totals and the
// conflicts that refused the change (dry run only; an applied change with
// conflicts fails with *availability.ConflictError).
type ChangeResult struct {
	Applied       bool
	Detail        *Detail
	PreviousTotal money.Cents
	NewTotal      money.Cents
	Conflicts     []availability.Conflict
}

// PriceDifference is NewTotal - PreviousTotal: positive to collect,
// negative to refund (spec 10.3).
func (r ChangeResult) PriceDifference() money.Cents { return r.NewTotal - r.PreviousTotal }

// ActionInput is a simple status change (confirm, approve, reject, check-in,
// no-show) with its optimistic concurrency guard.
type ActionInput struct {
	// ExpectedVersion is the If-Match version (nil = no precondition).
	ExpectedVersion *int64
	Note            string
}

// CancelInput is a cancellation (api.BookingCancelRequest).
type CancelInput struct {
	ExpectedVersion *int64
	Reason          string
	Settlement      Settlement // default SettlePolicy
	// RefundAmount overrides the amount computed from the policy (capped at paid).
	RefundAmount *money.Cents
	// RefundMethod records the money refund now (payment method); without
	// it a refund settlement is only recorded as due (the cashier records
	// it later with the payments API). Credits are always recorded.
	RefundMethod string
	Weather      bool
	DryRun       bool
}

// CancelResult is the outcome of a cancellation.
type CancelResult struct {
	Applied bool
	Detail  *Detail
	Refund  RefundPreview
	// Recorded is the refund or credit payment written (nil when none).
	Recorded *store.Payment
}

// MoveInput moves one line to another spot from a date (spec 11.3).
type MoveInput struct {
	ExpectedVersion *int64
	LineID          string
	SpotID          string
	// From is the first day on the new spot (nil = the whole stay).
	From   *calendar.Date
	DryRun bool
}

// NoteInput appends a note to the booking.
type NoteInput struct {
	Text string
	// Visibility is "internal" (default) or "customer".
	Visibility string
}

// AutoAssignInput assigns the pending lines of zone-mode zones (spec 7.5).
type AutoAssignInput struct {
	From, To calendar.Date
	ZoneID   string
	DryRun   bool
}

// AutoAssignment is one line placed on a spot.
type AutoAssignment struct {
	BookingID, LineID, SpotID, SpotLabel string
}

// AutoUnassigned is one line left pending with the reason.
type AutoUnassigned struct {
	BookingID, LineID, Reason string
}

// AutoAssignResult is the outcome of AutoAssign.
type AutoAssignResult struct {
	Applied    bool
	Assigned   []AutoAssignment
	Unassigned []AutoUnassigned
}

// ListInput filters the booking list.
type ListInput struct {
	Filter store.BookingFilter
	Cursor string
	Limit  int
}
