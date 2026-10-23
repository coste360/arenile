package inventory

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
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

// ---- transactional ports (booking.Inventory, pass.Inventory, mapping) --------

// SpotsWithFutureAllocations implements mapping.AllocationChecker: the spots
// among spotIDs with an allocation on or after from (holds only while not
// expired).
func (s *Service) SpotsWithFutureAllocations(ctx context.Context, db bun.IDB, es store.EstablishmentScope, spotIDs []string, from calendar.Date) ([]string, error) {
	return store.SpotsWithAllocationsFrom(ctx, db, es, spotIDs, from, s.d.Clock.Now())
}
