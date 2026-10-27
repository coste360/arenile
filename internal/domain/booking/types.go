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
	// Segments split the stay over several spots (spec 30.6, find spot):
	// consecutive, covering the stay exactly once (INV-102); SpotID is then
	// empty or the spot of the first segment.
	Segments []SegmentInput
}

// CreateInput is a booking creation (api.BookingCreate) plus the fields set
// only by internal callers (season passes, renewals).
type CreateInput struct {
	CustomerID  string
	Customer    *customer.Input
	Channel     string // default desk
	StartsOn    calendar.Date
	EndsOn      calendar.Date
	Slots       calendar.Slots
	Formula     string
	Lines       []LineInput
	Extras      []ExtraInput
	PromoCode   string
	AgreementID string
	// AgreementCode applies an agreement sold online by its code (public
	// channel only, M8); unknown or not usable online: 422.
	AgreementCode string
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
	// ExpectedTotal refuses the creation with ErrTotalChanged when the
	// computed total differs (find spot: the price shown with the proposal).
	ExpectedTotal *money.Cents

	// Group, party, partner and partner share (M8, spec 30.2.3-30.3).
	Group GroupInput
	// RequirementVerified records the check of the agreement requirement
	// (date and operator, never a document) with the creation.
	RequirementVerified bool

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
	// AgreementID changes the agreement (M8, spec 30.2.3): "" removes it,
	// nil leaves it; the booking is repriced (spec 10.3).
	AgreementID *string
	// Group changes group, party, partner and partner share (M8).
	Group GroupInput
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
	// History is read from audit_log: the entries of the booking (entity_type
	// booking) and of its payments and refunds (entity_type payment), oldest first.
	History []store.AuditEntry
	// SpotLabels, PackCodes and ExtraCodes label the ids of the lines.
	SpotLabels map[string]string
	PackCodes  map[string]string
	ExtraCodes map[string]string
	// RefundRequests are the guaranteed provider refunds of the booking
	// (ADR 0019 feature B), oldest first.
	RefundRequests []store.RefundRequest
	// LastMove is the most recent move touching the booking that can still
	// be undone (spec 30.7), nil if none.
	LastMove *UndoInfo
	// M8 holds the agreement, partner and requirement check (spec 30.2).
	M8 M8Detail
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
	// Undo is the 5 minute undo of an applied move (spec 30.7), nil otherwise.
	Undo *UndoInfo
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
	// it a refund settlement goes back to the online payments as guaranteed
	// provider refunds (ADR 0019, with refunds.issue) and the rest is only
	// recorded as due (the cashier records it later with the payments
	// API). Credits are always recorded.
	RefundMethod string
	Weather      bool
	DryRun       bool
	// FromTicket marks the cancellation run by the void of a daily ticket
	// (internal/domain/ticketing), the only caller allowed to cancel a
	// booking owned by a ticket (ErrTicketOwned otherwise, ADR 0020). HTTP
	// handlers never set it.
	FromTicket bool
}

// CancelResult is the outcome of a cancellation.
type CancelResult struct {
	Applied bool
	Detail  *Detail
	Refund  RefundPreview
	// Recorded is the refund or credit payment written (nil when none).
	Recorded *store.Payment
	// RefundReserved is the part of a money refund reserved as guaranteed
	// provider refunds (refund_requests, ADR 0019) in the cancellation
	// transaction; RefundRequestIDs are those requests (the caller may
	// send them at once, the job retries them anyway). RefundDue is what
	// is left for a manual refund (desk payments).
	RefundReserved   money.Cents
	RefundRequestIDs []string
	RefundDue        money.Cents
}

// MoveInput moves one line to another spot from a date (spec 11.3).
type MoveInput struct {
	ExpectedVersion *int64
	LineID          string
	SpotID          string
	// From is the first day on the new spot (nil = the whole stay).
	From   *calendar.Date
	DryRun bool
	// KeepPrice keeps the total of the booking as it is (the engine total of
	// the new spot is only recorded in the audit diff): the placement of a
	// daily ticket already paid at the desk never changes what the guest
	// owes (ADR 0020). Set by internal/domain/ticketassign through MoveThen.
	KeepPrice bool
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
