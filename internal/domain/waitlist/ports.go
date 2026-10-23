package waitlist

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the waitlist service, wired by internal/app.

// Holds creates and releases the offer holds inside the caller's
// transaction and reads the advisory availability used to choose the
// resources of an offer. Implemented by *inventory.Service
// (internal/domain/inventory/waitlist.go).
type Holds interface {
	CreateHoldInTx(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in inventory.HoldInput) (*inventory.HoldView, error)
	ReleaseHoldInTx(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, holdID string) error
	OfferAvailability(ctx context.Context, es store.EstablishmentScope, q inventory.AvailabilityQuery) (*inventory.AvailabilityResult, error)
}

// Checkout turns an accepted offer into a held booking with a checkout
// session, in one transaction with HoldSession.Within. Implemented by
// *checkout.Service (internal/domain/checkout/waitlist.go).
type Checkout interface {
	CreateFromHold(ctx context.Context, es store.EstablishmentScope, in checkout.HoldSession) (*checkout.Session, error)
}

// Notifier queues the waitlist emails (notify.TemplateWaitlistJoined,
// TemplateWaitlistOffer) with the personal link encrypted. Implemented by
// *notify.Service (internal/domain/notify/waitlist.go).
type Notifier interface {
	EnqueueWaitlist(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m notify.WaitlistMessage) (*store.Message, error)
}

// Customers creates the customer of a public join. Implemented by
// *customer.Service (ResolveOnline: a new customer per request, never
// matched to an existing one by email, ADR 0013 §3).
type Customers interface {
	ResolveOnline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID, currentID string, in customer.Input) (*store.Customer, error)
}

// Terms returns the terms version a public visitor must accept.
// Implemented by *online.Service.
type Terms interface {
	CurrentTerms(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*store.TermsVersion, error)
}
