package renewal

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the renewal service, wired by internal/app. Methods
// taking tx run in the caller's transaction and never open their own.

// Bookings creates and drives the held booking of a proposal. Implemented
// by *booking.Service (internal/domain/booking/renewal.go adds ExtendHold,
// ConfirmRenewal, EndRenewal and AcceptRenewalTerms): every booking state
// change goes through the booking service (audit + outbox in tx).
type Bookings interface {
	CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error)
	Quote(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput) (*booking.Quote, error)
	DetailIn(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*booking.Detail, error)
	ExtendHold(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, until time.Time) error
	StartPayment(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, holdUntil time.Time) (*booking.Detail, error)
	ConfirmRenewal(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID, note string) (*booking.Detail, error)
	EndRenewal(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, declined bool, reason string) (*booking.Detail, error)
	AcceptRenewalTerms(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID, version string) error
}

// Checkout resolves the magic link and starts the online payment of a
// renewal. Implemented by *checkout.Service (internal/domain/checkout/
// renewal.go): same link rules as the manage area, amount = deposit or
// balance, held -> pending_payment through the booking service, the
// confirmation only from the provider webhook.
type Checkout interface {
	ResolveManage(ctx context.Context, slug, token string) (*checkout.ManageRef, error)
	PayManagedAmount(ctx context.Context, slug, token string, amount money.Cents, idempotencyKey string) (*checkout.PayOutcome, error)
}

// Notifier queues the proposal emails. Implemented by *notify.Service
// (templates notify.TemplateRenewalProposal / TemplateRenewalReminder; the
// compose step issues the magic link of the proposal booking).
type Notifier interface {
	Enqueue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m notify.Enqueue) (*store.Message, error)
}

// Credits reads the credit balance a customer can spend (optional; nil =
// credit never shown). Implemented by *credit.Service (CreditBalance).
type Credits interface {
	CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error)
}

// CreditApplier spends up to limit of the customer credit on a booking as a
// payment with method credit (redemption row, balance, audit and outbox in
// its own transaction) and returns what it applied (optional; nil = the
// use_credit option is not offered). It is the operation behind POST
// /manage/{token}/apply-credit of the credit builder.
type CreditApplier interface {
	ApplyCreditToBooking(ctx context.Context, es store.EstablishmentScope, bookingID string, limit money.Cents, idempotencyKey string) (money.Cents, error)
}
