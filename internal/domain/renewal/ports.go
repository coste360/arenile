package renewal

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the renewal service, wired by internal/app.

// Bookings creates and drives the held booking of a proposal. Implemented by
// *booking.Service; ExtendHold is added by the renewal builder in
// internal/domain/booking/renewal.go (updates hold_expires_at of a held
// booking and of its hold allocations, audit + booking.modified).
type Bookings interface {
	CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error)
	Confirm(ctx context.Context, p *auth.Principal, establishmentID, id string, in booking.ActionInput) (*booking.Detail, error)
	Cancel(ctx context.Context, p *auth.Principal, establishmentID, id string, in booking.CancelInput) (*booking.CancelResult, error)
	ExtendHold(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, until time.Time) error
}

// Checkout resolves the magic link and starts the online payment of a
// renewal. Implemented by *checkout.Service; PayManagedAmount is added by
// the renewal builder in internal/domain/checkout/renewal.go (same rules as
// ManagePay, amount = deposit or total, held -> pending_payment through the
// booking service, credit applied first when useCredit).
type Checkout interface {
	ResolveManage(ctx context.Context, slug, token string) (store.EstablishmentScope, string, error)
	PayManagedAmount(ctx context.Context, slug, token string, amount money.Cents, useCredit bool, idempotencyKey string) (*checkout.PayOutcome, money.Cents, error)
}

// Notifier queues the proposal emails. Implemented by *notify.Service
// (templates notify.TemplateRenewalProposal / TemplateRenewalReminder).
type Notifier interface {
	Enqueue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m notify.Enqueue) (*store.Message, error)
}
