package partner

import (
	"context"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
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

// PartnerRef returns the partner of the establishment (configerr.NotFound
// style error when missing). Skeleton: delivered by the partners builder.
func (s *Service) PartnerRef(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, id string) (*Ref, error) {
	var p store.Partner
	if err := tx.NewSelect().Model(&p).Where("pa.establishment_id = ?", es.ID()).Where("pa.id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return &Ref{ID: p.ID, Name: p.Name, Kind: p.Kind}, nil
}
