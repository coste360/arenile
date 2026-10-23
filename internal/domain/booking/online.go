package booking

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/store"
)

// Use cases of the public channel (M4, spec 10.1, 12.4). Ownership: the
// "checkout" builder (internal/domain/checkout) implements these methods;
// they implement checkout.Bookings and are asserted in internal/app/m4.go.
//
// They take an audit.Actor (audit.ActorCustomer with the checkout session
// or magic link id, or audit.SystemActor) and an already resolved
// EstablishmentScope instead of an *auth.Principal: the public flow has no
// staff principal, and the checkout service resolves the scope from the
// slug and authenticates the session token or the magic link. Every method
// keeps the unit of work of doc.go (LockBooking, Transition, inventory,
// versioned update, audit + outbox in the caller's tx).
//
// Customer of a held online booking: bookings.customer_id is NOT NULL, but
// the customer types their details only at step 4. CreateOnline points the
// booking to the establishment's online placeholder customer
// (customer.Service, created on first use, tag customer.TagOnlinePlaceholder,
// hidden from lists, searches and exports); AttachCustomer re-points it to
// the resolved customer (existing by email/phone in es, or new).

// OnlineCustomerInput is step 4 of the public flow (api.CheckoutCustomerInput).
type OnlineCustomerInput struct {
	Customer customer.Input
	// TermsVersion is the accepted version (must be the current one: the
	// checkout service checks it); TermsAcceptedAt is set by the service clock.
	TermsVersion  string
	Locale        string
	NotesCustomer string
	// Extras replaces the booking-level extras when ExtrasSet.
	Extras    []ExtraInput
	ExtrasSet bool
	// PromoCode re-prices the booking when not nil ("" removes it).
	PromoCode *string
}

// CreateOnline creates a held booking of channel online with a hold of
// in.HoldTTL (default ARENILE_HOLD_TTL) in tx: price and online policy
// (violations refused, never forced), quota online, booking.created +
// booking.held.
func (s *Service) CreateOnline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, in CreateInput) (*Detail, error) {
	return nil, configerr.ErrNotImplemented
}

// AttachCustomer saves the customer, terms acceptance, marketing consent,
// notes, extras and promo code of a held online booking in tx
// (booking.modified). hold_expired when the hold already elapsed.
func (s *Service) AttachCustomer(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, in OnlineCustomerInput) (*Detail, error) {
	return nil, configerr.ErrNotImplemented
}

// StartPayment moves a held booking to pending_payment and extends its
// hold to holdUntil (max 30 minutes, spec 10.2) in tx. On a booking already
// pending_payment it only extends the hold (payment retry).
func (s *Service) StartPayment(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, holdUntil time.Time) (*Detail, error) {
	return nil, configerr.ErrNotImplemented
}

// ConfirmWithoutPayment confirms a held online booking whose policy needs
// no online payment (manual driver, spec 13.2): confirmed, or
// awaiting_approval with the long hold when approval is required.
func (s *Service) ConfirmWithoutPayment(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string) (*Detail, error) {
	return nil, configerr.ErrNotImplemented
}

// Abandon expires a held or pending_payment online booking and releases its
// resources in tx (booking.expired). Idempotent on an expired booking.
func (s *Service) Abandon(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string) error {
	return configerr.ErrNotImplemented
}

// ModifyAsCustomer applies (or previews, in.DryRun) a modification asked
// from the manage area within the modification limits of the policy
// snapshot (policy_violation otherwise, never forced).
func (s *Service) ModifyAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, bookingID string, expected *int64, in PatchInput) (*ChangeResult, error) {
	return nil, configerr.ErrNotImplemented
}

// CancelAsCustomer cancels (or previews, in.DryRun) from the manage area
// with the refund or credit of the policy snapshot; a money refund is left
// due (the checkout service then refunds through the payment gateway).
func (s *Service) CancelAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, bookingID string, in CancelInput) (*CancelResult, error) {
	return nil, configerr.ErrNotImplemented
}

// DetailIn loads the detail of a booking of es through db (no permission
// check: the caller authenticated a checkout session or a magic link).
func (s *Service) DetailIn(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*Detail, error) {
	b, err := store.GetBooking(ctx, db, es, bookingID)
	if err != nil {
		return nil, notFound(err)
	}
	return s.loadDetail(ctx, db, es, b)
}
