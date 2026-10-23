package booking

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// LedgerDeps are the dependencies of Ledger.
type LedgerDeps struct {
	Clock clock.Clock
}

// Ledger is the booking side of the payments ledger (bookings.paid_amount).
// It implements payment.BookingLedger and has no dependency on payment, so
// that payment can depend on it while booking depends on payment.Payments
// (no construction cycle, ADR 0010 §10).
type Ledger struct {
	d LedgerDeps
}

// NewLedger returns a Ledger.
func NewLedger(d LedgerDeps) *Ledger { return &Ledger{d: d} }

// LoadForPayment returns the booking, locked for the rest of tx (no-op
// UPDATE: payments on the same booking serialize), or store.ErrNotFound.
func (l *Ledger) LoadForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) (*store.Booking, error) {
	b, err := store.LockBooking(ctx, tx, es, bookingID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("booking: load for payment: %w", err)
	}
	return b, nil
}

// ApplyPayment adds signed (refunds negative) to paid_amount (INV-22) and
// bumps the version (the ETag of the booking changes with every payment).
func (l *Ledger) ApplyPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, signed money.Cents) (*store.Booking, error) {
	b, err := store.ApplyBookingPayment(ctx, tx, es, bookingID, signed, l.d.Clock.Now())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("booking: apply payment: %w", err)
	}
	return b, nil
}
