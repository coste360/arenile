package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// ListComponents returns a page of the components of an establishment.
func (s *Service) ListComponents(ctx context.Context, p *auth.Principal, establishmentID, cursor string, limit int) (store.Page[store.Component], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.Component]{}, err
	}
	page, err := store.ListComponents(ctx, s.d.DB, es, cursor, limit)
	if err != nil && !errors.Is(err, store.ErrInvalidCursor) {
		err = fmt.Errorf("catalog: list components: %w", err)
	}
	return page, err
}

// GetComponent returns one component.
func (s *Service) GetComponent(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.Component, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	c, err := store.GetComponent(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapReadErr(err, ErrComponentNotFound)
	}
	return c, nil
}

// checkComponentPool verifies that the pool of a tracked component belongs to
// the establishment and holds inventory of the same component code.
func checkComponentPool(ctx context.Context, w *writeCtx, f componentFields) error {
	if f.PoolID == nil {
		return nil
	}
	pool, err := store.CatalogGetPool(ctx, w.tx, w.es, *f.PoolID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return configerr.Invalid("pool_id", "Pool inesistente in questo stabilimento.")
		}
		return fmt.Errorf("catalog: load pool: %w", err)
	}
	if pool.ComponentCode != f.Code {
		return configerr.Invalid("pool_id", fmt.Sprintf("Il pool contiene il componente %q, non %q.", pool.ComponentCode, f.Code))
	}
	return nil
}

// CreateComponent creates a component (code unique per establishment).
func (s *Service) CreateComponent(ctx context.Context, p *auth.Principal, ip, establishmentID string, in ComponentInput) (*store.Component, error) {
	var out *store.Component
	err := s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		f, ve := validateComponent(in, w.est.DefaultLocale)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if err := checkComponentPool(ctx, w, f); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		c := &store.Component{
			ID: store.NewID(), EstablishmentID: w.es.ID(), Code: f.Code, Name: f.Name,
			Tracked: f.Tracked, PoolID: f.PoolID, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertComponent(ctx, w.tx, w.es, c); err != nil {
			return mapWriteErr(err, ErrComponentNotFound)
		}
		out = c
		return s.record(ctx, w, "component.created", "component", c.ID, map[string]any{"after": c}, nil)
	})
	return out, err
}

// UpdateComponent replaces a component. ifMatch (nil = no precondition) must
// equal the current version. The code of a component used by packs, extras
// or pools cannot change.
func (s *Service) UpdateComponent(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, in ComponentInput) (*store.Component, error) {
	var out *store.Component
	err := s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		cur, err := store.GetComponent(ctx, w.tx, w.es, id)
		if err != nil {
			return mapReadErr(err, ErrComponentNotFound)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		f, ve := validateComponent(in, w.est.DefaultLocale)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if err := checkComponentPool(ctx, w, f); err != nil {
			return err
		}
		if f.Code != cur.Code {
			if err := s.componentInUse(ctx, w, cur.Code); err != nil {
				return err
			}
		}
		before := *cur
		next := *cur
		next.Code, next.Name, next.Tracked, next.PoolID = f.Code, f.Name, f.Tracked, f.PoolID
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdateComponent(ctx, w.tx, w.es, &next, cur.Version)
		if err != nil {
			return mapWriteErr(err, ErrComponentNotFound)
		}
		if !ok {
			return lostRace(ctx, func(ctx context.Context) (int64, error) {
				c, err := store.GetComponent(ctx, w.tx, w.es, id)
				return c.Version, err
			}, ErrComponentNotFound)
		}
		out = &next
		return s.record(ctx, w, "component.updated", "component", id, map[string]any{"before": before, "after": next}, nil)
	})
	return out, err
}

// DeleteComponent deletes a component that no pack, extra or pool uses
// (otherwise ErrComponentInUse: components have no archived state).
func (s *Service) DeleteComponent(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64) error {
	return s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		cur, err := store.GetComponent(ctx, w.tx, w.es, id)
		if err != nil {
			return mapReadErr(err, ErrComponentNotFound)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		if err := s.componentInUse(ctx, w, cur.Code); err != nil {
			return err
		}
		if err := store.DeleteComponent(ctx, w.tx, w.es, id); err != nil {
			return mapWriteErr(err, ErrComponentNotFound)
		}
		return s.record(ctx, w, "component.deleted", "component", id, map[string]any{"before": cur}, nil)
	})
}

// componentInUse returns ErrComponentInUse when code is referenced by the
// composition of a pack, by an extra or by a pool of the establishment.
func (s *Service) componentInUse(ctx context.Context, w *writeCtx, code string) error {
	packs, err := store.ListAllPacks(ctx, w.tx, w.es)
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	for _, pk := range packs {
		if _, ok := pk.Composition[code]; ok {
			return fmt.Errorf("%w (pack %s)", ErrComponentInUse, pk.Code)
		}
	}
	extras, err := store.ListAllExtras(ctx, w.tx, w.es)
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	for _, x := range extras {
		if x.ComponentCode != nil && *x.ComponentCode == code {
			return fmt.Errorf("%w (extra %s)", ErrComponentInUse, x.Code)
		}
	}
	n, err := store.CatalogPoolsUsingComponent(ctx, w.tx, w.es, code)
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w (%d pool)", ErrComponentInUse, n)
	}
	return nil
}

// componentCodes returns the codes of the components of the establishment.
func componentCodes(ctx context.Context, w *writeCtx) (map[string]bool, error) {
	cs, err := store.ListAllComponents(ctx, w.tx, w.es)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	out := make(map[string]bool, len(cs))
	for _, c := range cs {
		out[c.Code] = true
	}
	return out, nil
}
