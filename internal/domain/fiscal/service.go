package fiscal

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Drivers resolves fiscal drivers by code. Implemented by *app.Registry.
type Drivers interface {
	FiscalDriver(code string) (ext.FiscalDriver, bool)
}

// Deps are the dependencies of the fiscal service. Every field is required
// except Log.
type Deps struct {
	DB      bun.IDB
	Clock   clock.Clock
	Audit   *audit.Writer
	Outbox  *event.Outbox
	Log     *slog.Logger
	Drivers Drivers
}

// Service implements the fiscal use cases.
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

// RecordPayment implements payment.Fiscal.
func (s *Service) RecordPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, pm *store.Payment) error {
	return fmt.Errorf("fiscal: record payment: %w", configerr.ErrNotImplemented)
}
