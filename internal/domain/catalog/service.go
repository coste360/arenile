package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the catalog package. They wrap the configerr kinds so that the
// HTTP layer maps them to stable problem types; the text after the kind is
// shown to the manager (Italian, source language).
var (
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	ErrComponentNotFound     = fmt.Errorf("%w: component", configerr.ErrNotFound)
	ErrPackNotFound          = fmt.Errorf("%w: pack", configerr.ErrNotFound)
	ErrExtraNotFound         = fmt.Errorf("%w: extra", configerr.ErrNotFound)
	// ErrDuplicateCode: the code is already used by another item of the same
	// kind in the establishment.
	ErrDuplicateCode = fmt.Errorf("%w: codice già usato da un altro elemento dello stabilimento", configerr.ErrConflict)
	// ErrComponentInUse: the component is referenced by packs, extras or
	// pools, so it can be neither deleted nor renamed.
	ErrComponentInUse = fmt.Errorf("%w: componente usato da pack, extra o pool", configerr.ErrConflict)
	// ErrPackCodeInUse: the pack code is listed in the scope of a booking
	// policy, so it cannot be renamed.
	ErrPackCodeInUse = fmt.Errorf("%w: codice del pack usato da una policy di prenotazione", configerr.ErrConflict)
)

// Deps are the dependencies of the catalog service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements the catalog use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil && d.Clock != nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Outbox == nil && d.Clock != nil {
		d.Outbox = event.NewOutbox(d.Clock)
	}
	return &Service{d: d}
}

// WithDB returns a copy of the service running on db (typically the bun.Tx
// of a caller composing several services, e.g. the guided setup). Mutations
// then run in a savepoint of that transaction.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// ChangeData is the payload (version 1) of the config.pricing_changed event
// emitted by pack and extra mutations.
type ChangeData struct {
	// Entity is "pack" or "extra".
	Entity   string `json:"entity"`
	EntityID string `json:"entity_id"`
	Code     string `json:"code"`
	// Change is created | updated | archived | deleted.
	Change string `json:"change"`
}

// Change kinds of ChangeData.
const (
	ChangeCreated  = "created"
	ChangeUpdated  = "updated"
	ChangeArchived = "archived"
	ChangeDeleted  = "deleted"
)

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return es, ErrEstablishmentNotFound
		}
		return es, err
	}
	return es, nil
}

// writeCtx is what a mutation needs inside its transaction.
type writeCtx struct {
	tx  bun.IDB
	es  store.EstablishmentScope
	est *store.Establishment
	p   *auth.Principal
	ip  string
}

// write authorizes a configuration change and runs fn in a transaction.
func (s *Service) write(ctx context.Context, p *auth.Principal, ip, establishmentID string, fn func(ctx context.Context, w *writeCtx) error) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		est, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrEstablishmentNotFound
			}
			return fmt.Errorf("catalog: load establishment: %w", err)
		}
		return fn(ctx, &writeCtx{tx: tx, es: es, est: est, p: p, ip: ip})
	})
}

// record writes the audit entry and, when change is set, the
// config.pricing_changed event, in the transaction of w.
func (s *Service) record(ctx context.Context, w *writeCtx, action, entityType, entityID string, diff any, change *ChangeData) error {
	err := s.d.Audit.Record(ctx, w.tx, audit.Entry{
		EstablishmentID: w.es.ID(), Actor: w.p.Actor(w.ip), Action: action,
		EntityType: entityType, EntityID: entityID, Diff: diff,
	})
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	if change != nil {
		if _, err := s.d.Outbox.Write(ctx, w.tx, event.ConfigPricingChanged, w.es.ID(), 1, change); err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
	}
	return nil
}

// mapWriteErr turns store errors of inserts/updates into catalog errors.
func mapWriteErr(err error, notFound error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrConflict):
		return ErrDuplicateCode
	case errors.Is(err, store.ErrNotFound):
		return notFound
	}
	return fmt.Errorf("catalog: %w", err)
}

func mapReadErr(err error, notFound error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return notFound
	}
	return fmt.Errorf("catalog: %w", err)
}

// lostRace builds the version conflict of an optimistic update that matched
// no row: the row changed (or disappeared) after it was read.
func lostRace(ctx context.Context, reload func(ctx context.Context) (int64, error), notFound error) error {
	v, err := reload(ctx)
	if err != nil {
		return mapReadErr(err, notFound)
	}
	return &configerr.VersionConflictError{Current: v}
}
