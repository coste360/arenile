package mapping

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// ListZones returns a page of zones (optionally of one map) with their
// active spot counts and pools.
func (s *Service) ListZones(ctx context.Context, p *auth.Principal, establishmentID string, mapID *string, cursor string, limit int) (store.Page[ZoneView], error) {
	es, err := s.view(ctx, p, establishmentID)
	if err != nil {
		return store.Page[ZoneView]{}, err
	}
	page, err := store.ListZones(ctx, s.d.DB, es, mapID, cursor, limit)
	if err != nil {
		return store.Page[ZoneView]{}, err
	}
	views, err := s.zoneViews(ctx, s.d.DB, es, page.Items)
	if err != nil {
		return store.Page[ZoneView]{}, err
	}
	return store.Page[ZoneView]{Items: views, NextCursor: page.NextCursor}, nil
}

// GetZone returns one zone.
func (s *Service) GetZone(ctx context.Context, p *auth.Principal, establishmentID, zoneID string) (*ZoneView, error) {
	es, err := s.view(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	return s.loadZoneView(ctx, s.d.DB, es, zoneID)
}

func (s *Service) loadZoneView(ctx context.Context, db bun.IDB, es store.EstablishmentScope, zoneID string) (*ZoneView, error) {
	z, err := store.GetZone(ctx, db, es, zoneID)
	if err != nil {
		return nil, notFound(err, ErrZoneNotFound)
	}
	views, err := s.zoneViews(ctx, db, es, []store.Zone{*z})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

func (s *Service) zoneViews(ctx context.Context, db bun.IDB, es store.EstablishmentScope, zones []store.Zone) ([]ZoneView, error) {
	ids := make([]string, len(zones))
	for i, z := range zones {
		ids[i] = z.ID
	}
	counts, err := store.MappingActiveSpotCountsByZone(ctx, db, es, ids)
	if err != nil {
		return nil, err
	}
	pools, err := store.ListZonePools(ctx, db, es, ids)
	if err != nil {
		return nil, err
	}
	// ListZonePools returns system pools first: the first pool of a zone is
	// its implicit pool, or its first bound pool.
	poolOf := make(map[string]string, len(pools))
	for _, pl := range pools {
		if pl.ZoneID != nil {
			if _, ok := poolOf[*pl.ZoneID]; !ok {
				poolOf[*pl.ZoneID] = pl.ID
			}
		}
	}
	out := make([]ZoneView, len(zones))
	for i, z := range zones {
		out[i] = ZoneView{Zone: z, SpotCount: counts[z.ID]}
		if id, ok := poolOf[z.ID]; ok {
			out[i].PoolID = &id
		}
	}
	return out, nil
}

// CreateZone creates a zone; a `zone` zone gets its implicit pool.
func (s *Service) CreateZone(ctx context.Context, p *auth.Principal, establishmentID string, in ZoneInput) (*ZoneView, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validateZoneInput(&in).OrNil(); err != nil {
		return nil, err
	}
	now := s.now()
	z := &store.Zone{ID: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now}
	applyZoneInput(z, in)
	var view *ZoneView
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		if err := checkZoneMap(ctx, tx, es, in.MapID); err != nil {
			return err
		}
		if err := store.InsertZone(ctx, tx, es, z); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrZoneCodeTaken
			}
			return err
		}
		pools, err := s.syncZonePools(ctx, tx, es, []string{z.ID})
		if err != nil {
			return err
		}
		data := MapChangedData{MapID: deref(z.MapID), EntityType: "zone", EntityID: z.ID, Action: "created", Version: z.Version}
		if err := s.record(ctx, tx, p, es, "zone.created", data, map[string]any{"after": z, "pools_synced": pools}); err != nil {
			return err
		}
		view, err = s.loadZoneView(ctx, tx, es, z.ID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("mapping: create zone: %w", err)
	}
	return view, nil
}

// UpdateZone replaces a zone (expected = optional If-Match). Moving a zone
// with spots to another map, or turning it into a `quantity` zone while it
// has spots, is refused; turning a zone with bound pools into a seat/zone
// zone is refused. The implicit pool follows the new mode in the same
// transaction.
func (s *Service) UpdateZone(ctx context.Context, p *auth.Principal, establishmentID, zoneID string, expected *int64, in ZoneInput) (*ZoneView, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validateZoneInput(&in).OrNil(); err != nil {
		return nil, err
	}
	var view *ZoneView
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetZone(ctx, tx, es, zoneID)
		if err != nil {
			return notFound(err, ErrZoneNotFound)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		if err := checkZoneMap(ctx, tx, es, in.MapID); err != nil {
			return err
		}
		spots, err := store.MappingSpotCountsByZone(ctx, tx, es, []string{zoneID})
		if err != nil {
			return err
		}
		if spots[zoneID] > 0 {
			if !equalPtr(cur.MapID, in.MapID) {
				return fmt.Errorf("%w: move or delete its spots before changing the map", ErrZoneHasSpots)
			}
			if in.AssignmentMode == ModeQuantity {
				return fmt.Errorf("%w: a quantity zone cannot have spots", ErrZoneHasSpots)
			}
		}
		if in.AssignmentMode != ModeQuantity && cur.AssignmentMode == ModeQuantity {
			if n, err := boundPools(ctx, tx, es, zoneID); err != nil {
				return err
			} else if n > 0 {
				return fmt.Errorf("%w: unbind its pools before changing the mode", ErrZoneHasPools)
			}
		}
		before := *cur
		next := *cur
		applyZoneInput(&next, in)
		next.UpdatedAt = s.now()
		ok, err := store.UpdateZoneVersioned(ctx, tx, es, &next, cur.Version)
		if errors.Is(err, store.ErrConflict) {
			return ErrZoneCodeTaken
		}
		if err != nil {
			return err
		}
		if !ok {
			latest, err := store.GetZone(ctx, tx, es, zoneID)
			if err != nil {
				return notFound(err, ErrZoneNotFound)
			}
			return &configerr.VersionConflictError{Current: latest.Version}
		}
		pools, err := s.syncZonePools(ctx, tx, es, []string{zoneID})
		if err != nil {
			return err
		}
		data := MapChangedData{MapID: deref(next.MapID), EntityType: "zone", EntityID: zoneID, Action: "updated", Version: next.Version}
		if err := s.record(ctx, tx, p, es, "zone.updated", data, map[string]any{"before": before, "after": next, "pools_synced": pools}); err != nil {
			return err
		}
		view, err = s.loadZoneView(ctx, tx, es, zoneID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("mapping: update zone: %w", err)
	}
	return view, nil
}

// DeleteZone deletes a zone without spots and without bound (non system)
// pools; its implicit pool goes with it.
func (s *Service) DeleteZone(ctx context.Context, p *auth.Principal, establishmentID, zoneID string, expected *int64) error {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return err
	}
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetZone(ctx, tx, es, zoneID)
		if err != nil {
			return notFound(err, ErrZoneNotFound)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		spots, err := store.MappingSpotCountsByZone(ctx, tx, es, []string{zoneID})
		if err != nil {
			return err
		}
		if spots[zoneID] > 0 {
			return fmt.Errorf("%w: move or delete its %d spots first", ErrZoneHasSpots, spots[zoneID])
		}
		if n, err := boundPools(ctx, tx, es, zoneID); err != nil {
			return err
		} else if n > 0 {
			return fmt.Errorf("%w: delete or unbind its %d pools first", ErrZoneHasPools, n)
		}
		if err := store.DeleteZone(ctx, tx, es, zoneID); err != nil {
			return notFound(err, ErrZoneNotFound)
		}
		data := MapChangedData{MapID: deref(cur.MapID), EntityType: "zone", EntityID: zoneID, Action: "deleted", Version: cur.Version}
		return s.record(ctx, tx, p, es, "zone.deleted", data, map[string]any{"before": cur})
	})
	if err != nil {
		return fmt.Errorf("mapping: delete zone: %w", err)
	}
	return nil
}

// checkZoneMap verifies that a zone's map belongs to the establishment.
func checkZoneMap(ctx context.Context, db bun.IDB, es store.EstablishmentScope, mapID *string) error {
	if mapID == nil {
		return nil
	}
	if _, err := store.GetMap(ctx, db, es, *mapID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return configerr.Invalid("map_id", "Mappa inesistente.")
		}
		return err
	}
	return nil
}

// boundPools counts the non system pools bound to a zone.
func boundPools(ctx context.Context, db bun.IDB, es store.EstablishmentScope, zoneID string) (int, error) {
	pools, err := store.ListZonePools(ctx, db, es, []string{zoneID})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, pl := range pools {
		if !pl.SystemManaged {
			n++
		}
	}
	return n, nil
}

func applyZoneInput(z *store.Zone, in ZoneInput) {
	z.MapID = in.MapID
	z.Code = in.Code
	z.Name = in.Name
	z.AssignmentMode = in.AssignmentMode
	z.OnlineVisible = in.OnlineVisible
	z.OnlineQuota = in.OnlineQuota
	z.PriceCategory = in.PriceCategory
	z.AutoAssign = in.AutoAssign
	z.SortOrder = in.SortOrder
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
