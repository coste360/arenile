package mapping

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// ListPools returns a page of pools, optionally of one zone. System managed
// pools are included (read-only).
func (s *Service) ListPools(ctx context.Context, p *auth.Principal, establishmentID string, zoneID *string, cursor string, limit int) (store.Page[store.Pool], error) {
	es, err := s.view(ctx, p, establishmentID)
	if err != nil {
		return store.Page[store.Pool]{}, err
	}
	return store.ListPools(ctx, s.d.DB, es, zoneID, cursor, limit)
}

// GetPool returns one pool.
func (s *Service) GetPool(ctx context.Context, p *auth.Principal, establishmentID, poolID string) (*store.Pool, error) {
	es, err := s.view(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	pl, err := store.GetPool(ctx, s.d.DB, es, poolID)
	if err != nil {
		return nil, notFound(err, ErrPoolNotFound)
	}
	return pl, nil
}

// CreatePool creates a quantity pool, optionally bound to a `quantity` zone.
func (s *Service) CreatePool(ctx context.Context, p *auth.Principal, establishmentID string, in PoolInput) (*store.Pool, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validatePoolInput(&in).OrNil(); err != nil {
		return nil, err
	}
	now := s.now()
	pl := &store.Pool{ID: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now}
	applyPoolInput(pl, in)
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		if err := checkPoolZone(ctx, tx, es, in.ZoneID); err != nil {
			return err
		}
		if err := store.InsertPool(ctx, tx, es, pl); err != nil {
			return err
		}
		data := event.ConfigChange{Entity: "pool", EntityID: pl.ID, Change: event.ChangeCreated, Version: pl.Version}
		return s.record(ctx, tx, p, es, "pool.created", data, map[string]any{"after": pl})
	})
	if err != nil {
		return nil, fmt.Errorf("mapping: create pool: %w", err)
	}
	return pl, nil
}

// UpdatePool replaces a pool (expected = optional If-Match). System managed
// pools are read-only.
func (s *Service) UpdatePool(ctx context.Context, p *auth.Principal, establishmentID, poolID string, expected *int64, in PoolInput) (*store.Pool, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validatePoolInput(&in).OrNil(); err != nil {
		return nil, err
	}
	var out *store.Pool
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetPool(ctx, tx, es, poolID)
		if err != nil {
			return notFound(err, ErrPoolNotFound)
		}
		if cur.SystemManaged {
			return ErrSystemPoolReadOnly
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		if err := checkPoolZone(ctx, tx, es, in.ZoneID); err != nil {
			return err
		}
		before := *cur
		next := *cur
		applyPoolInput(&next, in)
		next.UpdatedAt = s.now()
		ok, err := store.UpdatePoolVersioned(ctx, tx, es, &next, cur.Version)
		if err != nil {
			return err
		}
		if !ok {
			latest, err := store.GetPool(ctx, tx, es, poolID)
			if err != nil {
				return notFound(err, ErrPoolNotFound)
			}
			return &configerr.VersionConflictError{Current: latest.Version}
		}
		data := event.ConfigChange{Entity: "pool", EntityID: poolID, Change: event.ChangeUpdated, Version: next.Version}
		if err := s.record(ctx, tx, p, es, "pool.updated", data, map[string]any{"before": before, "after": next}); err != nil {
			return err
		}
		out = &next
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mapping: update pool: %w", err)
	}
	return out, nil
}

// DeletePool deletes a pool; system managed pools are read-only.
func (s *Service) DeletePool(ctx context.Context, p *auth.Principal, establishmentID, poolID string, expected *int64) error {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return err
	}
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetPool(ctx, tx, es, poolID)
		if err != nil {
			return notFound(err, ErrPoolNotFound)
		}
		if cur.SystemManaged {
			return ErrSystemPoolReadOnly
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		if err := store.DeletePool(ctx, tx, es, poolID); err != nil {
			return notFound(err, ErrPoolNotFound)
		}
		data := event.ConfigChange{Entity: "pool", EntityID: poolID, Change: event.ChangeDeleted, Version: cur.Version}
		return s.record(ctx, tx, p, es, "pool.deleted", data, map[string]any{"before": cur})
	})
	if err != nil {
		return fmt.Errorf("mapping: delete pool: %w", err)
	}
	return nil
}

// checkPoolZone verifies that a pool's zone belongs to the establishment and
// is a `quantity` zone (spec 6.2: pools are bound to quantity zones).
func checkPoolZone(ctx context.Context, db bun.IDB, es store.EstablishmentScope, zoneID *string) error {
	if zoneID == nil {
		return nil
	}
	z, err := store.GetZone(ctx, db, es, *zoneID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return configerr.Invalid("zone_id", "Zona inesistente.")
		}
		return err
	}
	if z.AssignmentMode != ModeQuantity {
		return configerr.Invalid("zone_id", "Un pool si può legare solo a una zona a quantità.")
	}
	return nil
}

func applyPoolInput(pl *store.Pool, in PoolInput) {
	pl.ZoneID = in.ZoneID
	pl.ComponentCode = in.ComponentCode
	pl.Name = in.Name
	pl.Capacity = in.Capacity
	pl.CapacityOverrides = in.CapacityOverrides
	if pl.CapacityOverrides == nil {
		pl.CapacityOverrides = []store.CapacityOverride{}
	}
}
