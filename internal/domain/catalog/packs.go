package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/store"
)

// ListPacks returns a page of the packs of an establishment (active nil = all).
func (s *Service) ListPacks(ctx context.Context, p *auth.Principal, establishmentID string, active *bool, cursor string, limit int) (store.Page[store.Pack], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.Pack]{}, err
	}
	page, err := store.ListPacks(ctx, s.d.DB, es, store.CatalogActiveFilter{Active: active}, cursor, limit)
	if err != nil && !errors.Is(err, store.ErrInvalidCursor) {
		err = fmt.Errorf("catalog: list packs: %w", err)
	}
	return page, err
}

// GetPack returns one pack.
func (s *Service) GetPack(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.Pack, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	pk, err := store.GetPack(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapReadErr(err, ErrPackNotFound)
	}
	return pk, nil
}

// checkPackRefs verifies the composition codes and the allowed zones against
// the establishment's components and zones.
func checkPackRefs(ctx context.Context, w *writeCtx, f packFields) error {
	ve := &configerr.ValidationError{}
	codes, err := componentCodes(ctx, w)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(f.Composition))
	for c := range f.Composition {
		keys = append(keys, c)
	}
	slices.Sort(keys)
	for _, c := range keys {
		if !codes[c] {
			ve.Add("composition."+c, "Componente inesistente: crealo prima nel catalogo.")
		}
	}
	if len(f.AllowedZoneIDs) > 0 {
		found, err := store.CatalogExistingZoneIDs(ctx, w.tx, w.es, f.AllowedZoneIDs)
		if err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
		for i, id := range f.AllowedZoneIDs {
			if !slices.Contains(found, id) {
				ve.Add(fmt.Sprintf("allowed_zone_ids[%d]", i), "Zona inesistente in questo stabilimento.")
			}
		}
	}
	return ve.OrNil()
}

func applyPack(dst *store.Pack, f packFields) {
	dst.Code, dst.Name, dst.Description, dst.Image = f.Code, f.Name, f.Description, f.Image
	dst.Composition = f.Composition
	dst.RequiresSpotKind, dst.AllowedZoneIDs = f.RequiresSpotKind, f.AllowedZoneIDs
	dst.AllowedFormulas, dst.Channels = f.AllowedFormulas, f.Channels
	dst.MaxPerBooking, dst.RequiresApproval, dst.SortOrder = f.MaxPerBooking, f.RequiresApproval, f.SortOrder
	dst.VATRateBP = f.VATRateBP
	if f.Active != nil {
		dst.Active = *f.Active
	}
}

func packChange(pk *store.Pack, change string) *ChangeData {
	return &ChangeData{Entity: "pack", EntityID: pk.ID, Code: pk.Code, Change: change}
}

// CreatePack creates a pack.
func (s *Service) CreatePack(ctx context.Context, p *auth.Principal, ip, establishmentID string, in PackInput) (*store.Pack, error) {
	var out *store.Pack
	err := s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		f, ve := validatePack(in, w.est.DefaultLocale)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if err := checkPackRefs(ctx, w, f); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		pk := &store.Pack{ID: store.NewID(), EstablishmentID: w.es.ID(), Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
		applyPack(pk, f)
		if err := store.InsertPack(ctx, w.tx, w.es, pk); err != nil {
			return mapWriteErr(err, ErrPackNotFound)
		}
		out = pk
		return s.record(ctx, w, "pack.created", "pack", pk.ID, map[string]any{"after": pk}, packChange(pk, ChangeCreated))
	})
	return out, err
}

// UpdatePack replaces a pack. Active nil keeps the current state.
func (s *Service) UpdatePack(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, in PackInput) (*store.Pack, error) {
	var out *store.Pack
	err := s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		cur, err := store.GetPack(ctx, w.tx, w.es, id)
		if err != nil {
			return mapReadErr(err, ErrPackNotFound)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		f, ve := validatePack(in, w.est.DefaultLocale)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if err := checkPackRefs(ctx, w, f); err != nil {
			return err
		}
		if f.Code != cur.Code {
			used, err := packCodeInPolicies(ctx, w, cur.Code)
			if err != nil {
				return err
			}
			if used {
				return ErrPackCodeInUse
			}
		}
		before := *cur
		next := *cur
		applyPack(&next, f)
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdatePack(ctx, w.tx, w.es, &next, cur.Version)
		if err != nil {
			return mapWriteErr(err, ErrPackNotFound)
		}
		if !ok {
			return lostRace(ctx, func(ctx context.Context) (int64, error) {
				x, err := store.GetPack(ctx, w.tx, w.es, id)
				return x.Version, err
			}, ErrPackNotFound)
		}
		out = &next
		return s.record(ctx, w, "pack.updated", "pack", id, map[string]any{"before": before, "after": next}, packChange(&next, ChangeUpdated))
	})
	return out, err
}

// DeletePack deletes a pack, or archives it (active = false) when price
// rules, fixed duration prices, extras or booking policies reference it, so
// that the price list history stays consistent. It reports whether the pack
// was archived instead of deleted.
//
// M3 note: once bookings exist, packs used by booking lines must be archived
// too; extend packReferenced then.
func (s *Service) DeletePack(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64) (archived bool, err error) {
	err = s.write(ctx, p, ip, establishmentID, func(ctx context.Context, w *writeCtx) error {
		cur, err := store.GetPack(ctx, w.tx, w.es, id)
		if err != nil {
			return mapReadErr(err, ErrPackNotFound)
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		refd, err := packReferenced(ctx, w, cur)
		if err != nil {
			return err
		}
		if !refd {
			if err := store.DeletePack(ctx, w.tx, w.es, id); err != nil {
				return mapWriteErr(err, ErrPackNotFound)
			}
			return s.record(ctx, w, "pack.deleted", "pack", id, map[string]any{"before": cur}, packChange(cur, ChangeDeleted))
		}
		archived = true
		if !cur.Active {
			return nil // already archived
		}
		next := *cur
		next.Active = false
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdatePack(ctx, w.tx, w.es, &next, cur.Version)
		if err != nil {
			return mapWriteErr(err, ErrPackNotFound)
		}
		if !ok {
			return lostRace(ctx, func(ctx context.Context) (int64, error) {
				x, err := store.GetPack(ctx, w.tx, w.es, id)
				return x.Version, err
			}, ErrPackNotFound)
		}
		return s.record(ctx, w, "pack.archived", "pack", id, map[string]any{"before": cur, "after": next}, packChange(&next, ChangeArchived))
	})
	return archived, err
}

// packReferenced reports whether price rules, fixed prices, extras or
// booking policies of the establishment reference the pack.
func packReferenced(ctx context.Context, w *writeCtx, pk *store.Pack) (bool, error) {
	targets, err := store.CatalogPriceRuleTargets(ctx, w.tx, w.es, "pack")
	if err != nil {
		return false, fmt.Errorf("catalog: %w", err)
	}
	if store.CatalogContains(targets, pk.ID) {
		return true, nil
	}
	fixed, err := store.CatalogPackHasFixedPrices(ctx, w.tx, w.es, pk.ID)
	if err != nil {
		return false, fmt.Errorf("catalog: %w", err)
	}
	if fixed {
		return true, nil
	}
	extras, err := store.ListAllExtras(ctx, w.tx, w.es)
	if err != nil {
		return false, fmt.Errorf("catalog: %w", err)
	}
	for _, x := range extras {
		if slices.Contains(x.AllowedPackIDs, pk.ID) {
			return true, nil
		}
	}
	return packInPolicies(ctx, w, pk.ID, pk.Code)
}

// packCodeInPolicies reports whether a booking policy scope lists code.
func packCodeInPolicies(ctx context.Context, w *writeCtx, code string) (bool, error) {
	return packInPolicies(ctx, w, "", code)
}

// packInPolicies reports whether a booking policy scope lists the pack by id
// or by code (policy.Scope accepts both).
func packInPolicies(ctx context.Context, w *writeCtx, id, code string) (bool, error) {
	scopes, err := store.CatalogPolicyScopes(ctx, w.tx, w.es)
	if err != nil {
		return false, fmt.Errorf("catalog: %w", err)
	}
	for _, raw := range scopes {
		var sc policy.Scope
		if err := raw.Unmarshal(&sc); err != nil {
			// A malformed scope cannot be interpreted: be conservative.
			return true, nil //nolint:nilerr // conservative: treat as referenced
		}
		for _, v := range sc.Packs {
			if (id != "" && v == id) || (code != "" && v == code) {
				return true, nil
			}
		}
	}
	return false, nil
}
