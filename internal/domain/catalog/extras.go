package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// ListExtras returns a page of the extras of an establishment (active nil = all).
func (s *Service) ListExtras(ctx context.Context, p *auth.Principal, establishmentID string, active *bool, cursor string, limit int) (store.Page[store.Extra], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.Extra]{}, err
	}
	page, err := store.ListExtras(ctx, s.d.DB, es, store.CatalogActiveFilter{Active: active}, cursor, limit)
	if err != nil && !errors.Is(err, store.ErrInvalidCursor) {
		err = fmt.Errorf("catalog: list extras: %w", err)
	}
	return page, err
}

// GetExtra returns one extra.
func (s *Service) GetExtra(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.Extra, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	x, err := store.GetExtra(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapReadErr(err, ErrExtraNotFound)
	}
	return x, nil
}

// checkExtraRefs verifies the component code and the allowed packs against
// the establishment's catalog.
func checkExtraRefs(ctx context.Context, w *writeCtx, f extraFields) error {
	ve := &configerr.ValidationError{}
	if f.ComponentCode != nil {
		codes, err := componentCodes(ctx, w)
		if err != nil {
			return err
		}
		if !codes[*f.ComponentCode] {
			ve.Add("component_code", "Componente inesistente: crealo prima nel catalogo.")
		}
	}
	if len(f.AllowedPackIDs) > 0 {
		found, err := store.CatalogExistingPackIDs(ctx, w.tx, w.es, f.AllowedPackIDs)
		if err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
		for i, id := range f.AllowedPackIDs {
			if !slices.Contains(found, id) {
				ve.Add(fmt.Sprintf("allowed_pack_ids[%d]", i), "Pack inesistente in questo stabilimento.")
			}
		}
	}
	return ve.OrNil()
}

func applyExtra(dst *store.Extra, f extraFields) {
	dst.Code, dst.Name, dst.Description = f.Code, f.Name, f.Description
	dst.ComponentCode, dst.PricingUnit, dst.MaxQuantity, dst.AttachTo = f.ComponentCode, f.PricingUnit, f.MaxQuantity, f.AttachTo
	dst.AllowedPackIDs, dst.Channels, dst.SortOrder, dst.VATRateBP = f.AllowedPackIDs, f.Channels, f.SortOrder, f.VATRateBP
	if f.Active != nil {
		dst.Active = *f.Active
	}
}

func extraChange(x *store.Extra, change string) *ChangeData {
	return &ChangeData{Entity: "extra", EntityID: x.ID, Code: x.Code, Change: change}
}

// CreateExtra creates an extra.
func (s *Service) CreateExtra(ctx context.Context, p *auth.Principal, ip, establishmentID string, in ExtraInput) (*store.Extra, error) {
	var out *store.Extra
	err := s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		f, ve := validateExtra(in, w.est.DefaultLocale)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if err := checkExtraRefs(ctx, w, f); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		x := &store.Extra{ID: store.NewID(), EstablishmentID: w.es.ID(), Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
		applyExtra(x, f)
		if err := store.InsertExtra(ctx, w.tx, w.es, x); err != nil {
			return mapWriteErr(err, ErrExtraNotFound)
		}
		out = x
		return s.record(ctx, w, "extra.created", "extra", x.ID, map[string]any{"after": x}, extraChange(x, ChangeCreated))
	})
	return out, err
}

// UpdateExtra replaces an extra. Active nil keeps the current state.
func (s *Service) UpdateExtra(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, in ExtraInput) (*store.Extra, error) {
	var out *store.Extra
	err := s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		cur, err := store.GetExtra(ctx, w.tx, w.es, id)
		if err != nil {
			return mapReadErr(err, ErrExtraNotFound)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		f, ve := validateExtra(in, w.est.DefaultLocale)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if err := checkExtraRefs(ctx, w, f); err != nil {
			return err
		}
		before := *cur
		next := *cur
		applyExtra(&next, f)
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdateExtra(ctx, w.tx, w.es, &next, cur.Version)
		if err != nil {
			return mapWriteErr(err, ErrExtraNotFound)
		}
		if !ok {
			return lostRace(ctx, func(ctx context.Context) (int64, error) {
				x, err := store.GetExtra(ctx, w.tx, w.es, id)
				return x.Version, err
			}, ErrExtraNotFound)
		}
		out = &next
		return s.record(ctx, w, "extra.updated", "extra", id, map[string]any{"before": before, "after": next}, extraChange(&next, ChangeUpdated))
	})
	return out, err
}

// DeleteExtra deletes an extra, or archives it (active = false) when price
// rules target it. It reports whether the extra was archived.
//
// M3 note: once bookings exist, extras used by booking lines must be
// archived too; extend the reference check then.
func (s *Service) DeleteExtra(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64) (archived bool, err error) {
	err = s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		cur, err := store.GetExtra(ctx, w.tx, w.es, id)
		if err != nil {
			return mapReadErr(err, ErrExtraNotFound)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		targets, err := store.CatalogPriceRuleTargets(ctx, w.tx, w.es, "extra")
		if err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
		if !store.CatalogContains(targets, id) {
			if err := store.DeleteExtra(ctx, w.tx, w.es, id); err != nil {
				return mapWriteErr(err, ErrExtraNotFound)
			}
			return s.record(ctx, w, "extra.deleted", "extra", id, map[string]any{"before": cur}, extraChange(cur, ChangeDeleted))
		}
		archived = true
		if !cur.Active {
			return nil
		}
		next := *cur
		next.Active = false
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdateExtra(ctx, w.tx, w.es, &next, cur.Version)
		if err != nil {
			return mapWriteErr(err, ErrExtraNotFound)
		}
		if !ok {
			return lostRace(ctx, func(ctx context.Context) (int64, error) {
				x, err := store.GetExtra(ctx, w.tx, w.es, id)
				return x.Version, err
			}, ErrExtraNotFound)
		}
		return s.record(ctx, w, "extra.archived", "extra", id, map[string]any{"before": cur, "after": next}, extraChange(&next, ChangeArchived))
	})
	return archived, err
}
