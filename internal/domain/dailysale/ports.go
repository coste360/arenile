package dailysale

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the daily sale service, wired by internal/app. Every
// method taking tx runs in the sale transaction and never opens its own.

// Bookings creates and quotes the confirmed desk booking of a sale.
// Implemented by *booking.Service.
type Bookings interface {
	CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error)
	Quote(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput) (*booking.Quote, error)
}

// CashSessions resolves the cash session of a payment: required and open
// for cash (cash_session_closed otherwise), nil for card. Implemented by
// *cash.Service.
type CashSessions interface {
	SessionForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, sessionID *string, method string) (*string, error)
}

// PaymentInput is the manual payment of a sale.
type PaymentInput struct {
	BookingID     string
	Amount        money.Cents
	Method        string // store.TicketPaymentCash or store.TicketPaymentCard
	CashSessionID *string
}

// Payments records the manual payment of the sale in tx (manual driver).
type Payments interface {
	RecordManualInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in PaymentInput) (*store.Payment, error)
}

// IssueInput is the ticket of a completed sale.
type IssueInput struct {
	BookingID         string
	Date              calendar.Date
	Kind              string // store.TicketKindSale or store.TicketKindEntry
	ZoneID            string
	NeedsAssignment   bool
	ProvisionalSpotID *string
	HolderName        string
	TotalAmount       money.Cents
	PaymentMethod     string
	PaymentID         *string
	CashSessionID     *string
}

// Tickets issues the numbered ticket in tx (store.NextTicketNumber + insert).
// Implemented by the ticketing service.
type Tickets interface {
	Issue(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in IssueInput) (*store.Ticket, error)
}
