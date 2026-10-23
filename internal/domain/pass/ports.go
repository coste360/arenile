package pass

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the pass service, wired by internal/app.

// Bookings creates the booking behind a season pass. Implemented by
// booking.Service.
type Bookings interface {
	// CreateInTx creates a booking in tx (pricing, policy, allocation with
	// in.AllocationKind = inventory.KindSeason, audit, outbox) and returns it.
	CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error)
}

// Inventory removes and restores season allocations. Implemented by
// inventory.Service.
type Inventory interface {
	RemoveSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days inventory.SeasonDays) error
	RestoreSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days inventory.SeasonDays) error
}

// Credits grants season release credits. Implemented by customer.Service.
type Credits interface {
	GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error
}
