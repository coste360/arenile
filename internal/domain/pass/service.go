package pass

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of the pass service. Every field is required
// except Log.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Bookings  Bookings
	Inventory Inventory
	Credits   Credits
}

// Service implements the season pass use cases.
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

// ResaleDeps are the dependencies of Resale.
type ResaleDeps struct {
	Clock   clock.Clock
	Credits Credits
	Log     *slog.Logger
}

// Resale marks released season half-days as resold. It implements
// booking.Releases and has no dependency on the booking service.
type Resale struct {
	d ResaleDeps
}

// NewResale returns a Resale.
func NewResale(d ResaleDeps) *Resale {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Resale{d: d}
}

// MarkResold is called by the booking service, in its transaction, after a
// booking line has been allocated on spotID for the given half-days: every
// active (non revoked, not yet resold) release of those half-days gets
// resold_booking_line_id = lineID and, when the pass policy grants
// season_release_credit, a credit to the pass holder. Half-days without a
// release are ignored.
func (r *Resale) MarkResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineID, spotID string, dates []calendar.Date, slots calendar.Slots) error {
	return fmt.Errorf("pass: mark resold: %w", configerr.ErrNotImplemented)
}

// UnmarkResold clears resold_booking_line_id (and reverses the credit, if
// any) when the resale line releases its allocations (cancel, move).
func (r *Resale) UnmarkResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineID string) error {
	return fmt.Errorf("pass: unmark resold: %w", configerr.ErrNotImplemented)
}
