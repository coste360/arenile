package inventory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// DefaultHoldTTL is the hold duration when Deps.HoldTTL is zero (spec 7.4).
const DefaultHoldTTL = 10 * time.Minute

// Deps are the dependencies of the inventory service. Every field is
// required except Log and HoldTTL.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	// HoldTTL is ARENILE_HOLD_TTL (default DefaultHoldTTL).
	HoldTTL time.Duration
}

// Service implements the DB-backed availability engine.
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

// HoldTTL is the configured hold duration.
func (s *Service) HoldTTL() time.Duration { return s.d.HoldTTL }

// Jobs returns the background jobs of the inventory (registered by the app).
func (s *Service) Jobs() []jobs.Job { return nil }

// ---- transactional ports (booking.Inventory, pass.Inventory, mapping) --------

// Allocate occupies the resources of req in tx. Conflicts fail with
// *availability.ConflictError (ErrSpotUnavailable, ErrPoolExhausted,
// ErrQuotaExceeded) and must roll back the caller's transaction.
func (s *Service) Allocate(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req AllocateRequest) (*Allocation, error) {
	return nil, fmt.Errorf("inventory: allocate: %w", configerr.ErrNotImplemented)
}

// Check reports the conflicts Allocate would hit, without writing (quotes,
// dry runs). Expired holds count as free.
func (s *Service) Check(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req AllocateRequest) ([]availability.Conflict, error) {
	return nil, fmt.Errorf("inventory: check: %w", configerr.ErrNotImplemented)
}

// Release deletes the allocations of a booking (spot rows and pool detail
// rows, decrementing pool_usage) in tx.
func (s *Service) Release(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req ReleaseRequest) (*Allocation, error) {
	return nil, fmt.Errorf("inventory: release: %w", configerr.ErrNotImplemented)
}

// Confirm turns the hold allocations of a booking into stable ones in tx:
// kind hold -> booking (expires_at NULL), pool held -> used. A hold already
// expired fails with availability.ErrHoldExpired.
func (s *Service) Confirm(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error {
	return fmt.Errorf("inventory: confirm: %w", configerr.ErrNotImplemented)
}

// ExtendHold moves expires_at of the hold allocations of a booking (payment
// started, approval requested) in tx.
func (s *Service) ExtendHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, expiresAt time.Time) error {
	return fmt.Errorf("inventory: extend hold: %w", configerr.ErrNotImplemented)
}

// ConvertHold re-points the allocations of a panel hold to a booking in tx
// and marks the hold converted.
func (s *Service) ConvertHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req ConvertHoldRequest) (*Allocation, error) {
	return nil, fmt.Errorf("inventory: convert hold: %w", configerr.ErrNotImplemented)
}

// RemoveSeasonDays deletes the season allocations of the given half-days
// (pass release, spec 7.7) in tx.
func (s *Service) RemoveSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days SeasonDays) error {
	return fmt.Errorf("inventory: remove season days: %w", configerr.ErrNotImplemented)
}

// RestoreSeasonDays recreates the season allocations of the given half-days
// (release revoked) in tx; a half-day sold meanwhile fails with
// *availability.ConflictError{Err: ErrSpotUnavailable}.
func (s *Service) RestoreSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days SeasonDays) error {
	return fmt.Errorf("inventory: restore season days: %w", configerr.ErrNotImplemented)
}

// SpotsWithFutureAllocations implements mapping.AllocationChecker: the spots
// among spotIDs with an allocation on or after from (holds only while not
// expired).
func (s *Service) SpotsWithFutureAllocations(ctx context.Context, db bun.IDB, es store.EstablishmentScope, spotIDs []string, from calendar.Date) ([]string, error) {
	return store.SpotsWithAllocationsFrom(ctx, db, es, spotIDs, from, s.d.Clock.Now())
}
