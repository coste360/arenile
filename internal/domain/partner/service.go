package partner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of Service.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements partners, customer categories, customer requirement
// verifications and the partner account. Permissions: reads bookings.read
// (statement and payments reports.read), writes establishment.configure
// (partners, categories), cash.operate (partner payments), bookings.write
// (verifications).
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

// Errors of the partner domain besides the sentinels of errors.go.
var (
	// ErrNotFound: unknown or out-of-scope partner, category or payment.
	ErrNotFound = fmt.Errorf("%w: partner", configerr.ErrNotFound)
	// ErrDuplicateCategory: a category with the same code exists.
	ErrDuplicateCategory = fmt.Errorf("%w: a customer category with the same code exists", configerr.ErrConflict)
	// ErrPaymentVoided: the payment was already voided.
	ErrPaymentVoided = fmt.Errorf("%w: payment already voided", configerr.ErrConflict)
)

// Audit entity types and the entity values of event.ConfigChange.
const (
	EntityPartner          = "partner"
	EntityCustomerCategory = "customer_category"
	EntityPartnerPayment   = "partner_payment"
	EntityVerification     = "requirement_verification"
)

// Ref is a short reference to a partner (badges, booking detail).
type Ref struct {
	ID   string
	Name string
	Kind string
}

// Lookup is the port booking and inventory use to resolve a partner inside
// their transaction (scoped by establishment, INV-26). Service implements
// it.
type Lookup interface {
	PartnerRef(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, id string) (*Ref, error)
}

var _ Lookup = (*Service)(nil)

// PartnerRef returns the partner of the establishment (ErrNotFound when
// missing).
func (s *Service) PartnerRef(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, id string) (*Ref, error) {
	p, err := store.GetPartner(ctx, tx, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	return &Ref{ID: p.ID, Name: p.Name, Kind: p.Kind}, nil
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, db bun.IDB, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	es, err := p.Authorize(ctx, db, establishmentID, perm)
	if err != nil {
		return store.EstablishmentScope{}, fmt.Errorf("partner: %w", err)
	}
	return es, nil
}

func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("partner: %w", err)
}

// staleOrMissing is the error of an optimistic update that matched no row.
func staleOrMissing(ctx context.Context, db bun.IDB, get func(context.Context, bun.IDB) (int64, error)) error {
	v, err := get(ctx, db)
	if err != nil {
		return notFound(err)
	}
	return &configerr.VersionConflictError{Current: v}
}

// record writes the audit entry of a change and, when configEvent, the
// config.pricing_changed event (agreements depend on partners and
// categories: the public page and the caches reload them). diff never
// carries contact data.
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, entity, id, kind string, diff any, configEvent bool) error {
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(reqctx.From(ctx).ClientIP),
		Action: entity + "." + kind, EntityType: entity, EntityID: id, Diff: diff,
	}); err != nil {
		return fmt.Errorf("partner: %w", err)
	}
	if !configEvent {
		return nil
	}
	if _, err := s.d.Outbox.Write(ctx, tx, event.ConfigPricingChanged, es.ID(), 1,
		event.ConfigChange{Entity: entity, EntityID: id, Change: kind}); err != nil {
		return fmt.Errorf("partner: %w", err)
	}
	return nil
}

func userOf(p *auth.Principal) *string {
	if p != nil && p.Kind == auth.PrincipalUser && p.UserID != "" {
		u := p.UserID
		return &u
	}
	return nil
}
