package booking

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the booking service, wired by internal/app (see m3.go).
// Every method taking tx runs in the booking transaction and never opens its
// own (see the unit of work note in doc.go).

// Inventory occupies and frees resources. Implemented by inventory.Service.
type Inventory interface {
	// Allocate occupies the resources of req; conflicts are
	// *availability.ConflictError and abort the transaction.
	Allocate(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req inventory.AllocateRequest) (*inventory.Allocation, error)
	// Check lists the conflicts Allocate would hit, without writing.
	Check(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req inventory.AllocateRequest) ([]availability.Conflict, error)
	// Release frees the allocations of a booking (all, some lines, or from a date).
	Release(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req inventory.ReleaseRequest) (*inventory.Allocation, error)
	// Confirm turns hold allocations into stable ones (fails with
	// availability.ErrHoldExpired when the hold already expired).
	Confirm(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error
	// ExtendHold moves the expiry of the hold allocations of a booking.
	ExtendHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, expiresAt time.Time) error
	// ConvertHold re-points the allocations of a panel hold to the booking.
	ConvertHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req inventory.ConvertHoldRequest) (*inventory.Allocation, error)
}

// Pricer prices a request and evaluates the booking policy inside tx.
// Implemented by *pricingconfig.Service (Quote: Simulate without the
// permission check, reading through db).
type Pricer interface {
	Quote(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in pricingconfig.SimulateInput) (*pricingconfig.SimulateResult, error)
}

// Customers resolves the customer of a booking. Implemented by customer.Service.
type Customers interface {
	// Resolve returns customerID (in es) or creates in (inline customer) in tx.
	Resolve(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, customerID string, in *customer.Input) (*store.Customer, error)
}

// Payments records refunds of cancellations and lists the ledger of a
// booking. Implemented by payment.Service.
type Payments interface {
	RecordRefund(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, req payment.RefundRequest) (*store.Payment, error)
	ListByBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) ([]store.Payment, error)
}

// Releases tracks resales of days released by season pass holders.
// Implemented by pass.Resale.
type Releases interface {
	// MarkResold is called after a line has been allocated on spotID.
	MarkResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineID, spotID string, dates []calendar.Date, slots calendar.Slots) error
	// UnmarkResold is called when a line releases its allocations.
	UnmarkResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineID string) error
}
