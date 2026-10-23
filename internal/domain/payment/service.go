package payment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of the payment service. Every field is required
// except Log.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Bookings  BookingLedger
	Cash      CashDesk
	Fiscal    Fiscal
	Credits   Credits
	Providers Providers
}

// Service implements the payment use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// RefundAs is how a refund is settled.
type RefundAs string

// Refund settlements.
const (
	RefundAsRefund RefundAs = "refund" // money back (manual refund to complete)
	RefundAsCredit RefundAs = "credit" // customer credit (customer_credits)
)

// RefundRequest is a refund of a booking, from the API (RecordBookingRefund)
// or from a cancellation computed on the policy snapshot (booking service).
type RefundRequest struct {
	BookingID     string
	Amount        money.Cents
	Method        string // store.PaymentMethod*; ignored for RefundAsCredit
	As            RefundAs
	PaymentID     string // refunded payment (optional)
	Reason        string
	CashSessionID *string
	// IdempotencyKey, when set, makes the refund idempotent per establishment.
	IdempotencyKey string
}

// RecordRefund records a refund (or credit) of a booking in tx, applies it to
// paid_amount and writes audit + payment.refunded. The caller (booking
// cancellation, or the payment API use case) has already checked
// auth.PermRefundsIssue. Implements booking.Payments.
func (s *Service) RecordRefund(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, req RefundRequest) (*store.Payment, error) {
	return nil, fmt.Errorf("payment: record refund: %w", configerr.ErrNotImplemented)
}

// ListByBooking returns the payments and refunds of a booking, oldest first.
// Implements booking.Payments.
func (s *Service) ListByBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) ([]store.Payment, error) {
	return nil, fmt.Errorf("payment: list by booking: %w", configerr.ErrNotImplemented)
}
