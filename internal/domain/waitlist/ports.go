package waitlist

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the waitlist service, wired by internal/app.

// Holds creates and releases the offer holds inside the caller's
// transaction. Implemented by *inventory.Service: the waitlist builder adds
// CreateHoldInTx / ReleaseHoldInTx in internal/domain/inventory/waitlist.go
// (the bodies of CreateHold / ReleaseHold without principal and own tx).
type Holds interface {
	CreateHoldInTx(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in inventory.HoldInput) (*inventory.HoldView, error)
	ReleaseHoldInTx(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, holdID string) error
}

// Checkout turns an accepted offer into a held booking with a checkout
// session. Implemented by *checkout.Service: the waitlist builder adds
// CreateFromHold in internal/domain/checkout/waitlist.go (booking created
// through the booking service from the hold, customer attached, session
// token issued like Create).
type Checkout interface {
	CreateFromHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, holdID, customerID, locale string) (*checkout.Session, error)
}

// Notifier queues the waitlist emails (notify.TemplateWaitlistJoined,
// TemplateWaitlistOffer). Implemented by *notify.Service.
type Notifier interface {
	Enqueue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m notify.Enqueue) (*store.Message, error)
}
