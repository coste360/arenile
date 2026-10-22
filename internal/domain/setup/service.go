package setup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/catalog"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/season"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the setup package. They wrap the configerr kinds so that the
// HTTP layer maps them to stable problem types.
var (
	// ErrAlreadyConfigured: the establishment already has maps or zones and
	// the template would duplicate the inventory (409 conflict).
	ErrAlreadyConfigured = fmt.Errorf("%w: establishment already has maps or zones, apply the template with force or use \"empty\"", configerr.ErrConflict)
	// ErrQuickRulesExist: quick price list rules already exist for the season
	// and replace_existing is false (409 conflict).
	ErrQuickRulesExist = fmt.Errorf("%w: quick price list rules already exist for this season, set replace_existing", configerr.ErrConflict)
	// ErrSeasonNotFound: unknown season or season of another establishment.
	ErrSeasonNotFound = fmt.Errorf("%w: season", configerr.ErrNotFound)
)

// Deps are the dependencies of the setup service. Every field is required
// except Log and the composed services: the setup use cases write through the
// store models in a single transaction (see doc.go), the services are kept
// for the integrator.
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
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Outbox == nil {
		d.Outbox = event.NewOutbox(d.Clock)
	}
	return &Service{d: d}
}

// authorize resolves the establishment in the principal's scope and checks
// perm, mapping "unknown or foreign" to configerr.ErrNotFound.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return es, fmt.Errorf("%w: establishment", configerr.ErrNotFound)
		}
		return es, err
	}
	return es, nil
}

// today returns the current date in the establishment timezone.
func (s *Service) today(e *store.Establishment) calendar.Date {
	loc, err := time.LoadLocation(e.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return calendar.In(s.d.Clock.Now(), loc)
}

// DefaultSeasonYear is the year of the "current or next" season: the current
// year until the default closing day (30 September), the next one afterwards.
func DefaultSeasonYear(today calendar.Date) int {
	if today.After(calendar.New(today.Year(), time.September, 30)) {
		return today.Year() + 1
	}
	return today.Year()
}
