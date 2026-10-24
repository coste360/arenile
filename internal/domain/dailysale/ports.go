package dailysale

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the daily sale service, wired by internal/app. Every
// method taking tx runs in the sale transaction and never opens its own
// (nested work becomes a savepoint of tx).

// Bookings creates and quotes the confirmed desk booking of a sale.
// Implemented by *booking.Service.
type Bookings interface {
	CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error)
	Quote(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput) (*booking.Quote, error)
}

// PaymentInput is the manual payment of a sale.
type PaymentInput struct {
	BookingID     string
	Amount        money.Cents
	Method        string // store.TicketPaymentCash or store.TicketPaymentCard
	CashSessionID *string
	Note          string
}

// Payments records the manual payment of the sale in tx (manual driver):
// cash needs an open cash session (cash_session_closed otherwise), card
// attaches to the open one when there is one. Implemented by
// PaymentRecorder over *payment.Service.
type Payments interface {
	RecordManualInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in PaymentInput) (*store.Payment, error)
}

// Tickets issues the numbered ticket in tx. Implemented by *ticketing.Service.
type Tickets interface {
	Issue(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in ticketing.IssueInput) (*store.Ticket, error)
}

// Customers resolves or creates the walk-in customer. Implemented by
// *customer.Service.
type Customers interface {
	Resolve(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, customerID string, in *customer.Input) (*store.Customer, error)
}

// Availability reads the free spots of a zone and the residual capacity of
// zone and quantity zones. Implemented by *inventory.Service.
type Availability interface {
	GetAvailability(ctx context.Context, p *auth.Principal, establishmentID string, q inventory.AvailabilityQuery) (*inventory.AvailabilityResult, error)
}
