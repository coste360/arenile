package setup

import (
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/catalog"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/season"
)

// Deps are the dependencies of the setup service. Every field is required
// except Log.
type Deps struct {
	DB      bun.IDB
	Clock   clock.Clock
	Audit   *audit.Writer
	Outbox  *event.Outbox
	Log     *slog.Logger
	Seasons *season.Service
	Mapping *mapping.Service
	Catalog *catalog.Service
	Pricing *pricingconfig.Service
}

// Service implements the guided setup use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service { return &Service{d: d} }
