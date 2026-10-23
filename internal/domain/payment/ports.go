package payment

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Ports consumed by the payment service. Implementations live in other
// packages and are wired by internal/app; every method taking tx runs in
// the caller's transaction and must not open its own.

// BookingLedger keeps the booking side of the ledger. Implemented by
// booking.Ledger (stateless, no dependency on payment).
type BookingLedger interface {
	// LoadForPayment returns the booking in es, locked for the rest of tx
	// (no-op UPDATE on PostgreSQL), or store.ErrNotFound.
	LoadForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) (*store.Booking, error)
	// ApplyPayment adds signed (refunds negative) to bookings.paid_amount,
	// bumps the version and returns the updated booking.
	ApplyPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, signed money.Cents) (*store.Booking, error)
}

// CashDesk attaches payments to cash sessions. Implemented by cash.Service.
type CashDesk interface {
	// SessionForPayment returns the session that receives a payment of
	// method: sessionID when given (must be open), otherwise the open
	// session of store.DefaultCashRegister. Method cash without an open
	// session fails with cash.ErrSessionClosed; other methods return nil
	// when no session is open.
	SessionForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, sessionID *string, method string) (*string, error)
}

// Fiscal records the fiscal document of a payment. Implemented by
// fiscal.Service (driver "none" by default).
type Fiscal interface {
	// RecordPayment issues (or tracks) the fiscal document of pm in tx.
	RecordPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, pm *store.Payment) error
}

// Credits is the customer credit ledger (spec 13.4). Implemented by
// customer.Service.
type Credits interface {
	// GrantCredit appends a ledger movement (negative amount = credit used).
	GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error
	// CreditBalance is the available (non-expired) balance on today.
	CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error)
}

// Providers resolves payment drivers by code. Implemented by *app.Registry.
type Providers interface {
	PaymentProvider(code string) (ext.PaymentProvider, bool)
}
