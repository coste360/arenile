package ticketing

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/store"
)

// Ports consumed by the ticketing service, wired by internal/app. Methods
// taking tx run in the caller's transaction and never open their own.

// Bookings performs every state change of the ticket's booking (audit log
// and outbox event in the same transaction). Implemented by *booking.Service.
type Bookings interface {
	// Move places a line on a spot (assignment, reassignment, move); a
	// taken spot is a conflict error (409, unique allocation key).
	Move(ctx context.Context, p *auth.Principal, establishmentID, id string, in booking.MoveInput) (*booking.ChangeResult, error)
	// Cancel cancels the booking of a voided ticket and frees its resources.
	Cancel(ctx context.Context, p *auth.Principal, establishmentID, id string, in booking.CancelInput) (*booking.CancelResult, error)
}

// Refunds records the refund of a voided ticket through the payments
// ledger. Implemented by *payment.Service.
type Refunds interface {
	RecordRefund(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, req payment.RefundRequest) (*store.Payment, error)
}
