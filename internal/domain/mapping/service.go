package mapping

import (
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
)

// Deps are the dependencies of the mapping service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	// Allocations reports future allocations of spots; nil means NoAllocations.
	Allocations AllocationChecker
}

// Service implements the mapping use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Allocations == nil {
		d.Allocations = NoAllocations{}
	}
	return &Service{d: d}
}
