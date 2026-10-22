package pricingconfig

import (
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
)

// Deps are the dependencies of the pricing configuration service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements price rules, fixed prices, promo codes, agreements and
// the pricing tools (simulate, matrix, coverage, season copy).
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service { return &Service{d: d} }
