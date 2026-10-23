package booking

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// DefaultHoldTTL is the booking hold duration when Deps.HoldTTL is zero.
const DefaultHoldTTL = 10 * time.Minute

// Deps are the dependencies of the booking service. Every field is required
// except Log and HoldTTL.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Inventory Inventory
	Pricer    Pricer
	Customers Customers
	Payments  Payments
	Releases  Releases
	// HoldTTL is ARENILE_HOLD_TTL (default DefaultHoldTTL).
	HoldTTL time.Duration
}

// Service implements the booking use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.HoldTTL <= 0 {
		d.HoldTTL = DefaultHoldTTL
	}
	return &Service{d: d}
}

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// Jobs returns the background jobs of the booking service (expire booking
// holds every 30 s, complete past bookings daily); registered by the app.
func (s *Service) Jobs() []jobs.Job { return nil }

// CreateInTx creates a booking inside the caller's transaction (used by the
// pass service for season passes and by Create). It authorizes
// auth.PermBookingsWrite, resolves the customer, prices and validates the
// policy (Pricer), allocates (Inventory), writes the rows, the audit log and
// event.BookingCreated (+ BookingConfirmed / BookingHeld). Implements
// pass.Bookings.
func (s *Service) CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in CreateInput) (*Detail, error) {
	return nil, fmt.Errorf("booking: create: %w", configerr.ErrNotImplemented)
}

// LedgerDeps are the dependencies of Ledger.
type LedgerDeps struct {
	Clock clock.Clock
}

// Ledger is the booking side of the payments ledger (bookings.paid_amount).
// It implements payment.BookingLedger and has no dependency on payment.
type Ledger struct {
	d LedgerDeps
}

// NewLedger returns a Ledger.
func NewLedger(d LedgerDeps) *Ledger { return &Ledger{d: d} }

// LoadForPayment returns the booking, locked for the rest of tx.
func (l *Ledger) LoadForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) (*store.Booking, error) {
	return nil, fmt.Errorf("booking: load for payment: %w", configerr.ErrNotImplemented)
}

// ApplyPayment adds signed to paid_amount (INV-22) and bumps the version.
func (l *Ledger) ApplyPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, signed money.Cents) (*store.Booking, error) {
	return nil, fmt.Errorf("booking: apply payment: %w", configerr.ErrNotImplemented)
}
