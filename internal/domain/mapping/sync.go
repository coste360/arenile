package mapping

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/store"
)

// syncZonePools keeps the implicit pools of the given zones in line with
// spec 7.5, inside the caller's transaction:
//
//   - a `zone` zone has exactly one system_managed pool, with capacity equal
//     to its number of active spots and the zone name;
//   - any other zone has none (a leftover pool is deleted).
//
// It returns the ids of the pools it created, updated or deleted. Unknown
// zone ids (e.g. deleted zones, whose pools cascade) are ignored.
func (s *Service) syncZonePools(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, zoneIDs []string) ([]string, error) {
	zoneIDs = uniqueSorted(zoneIDs)
	if len(zoneIDs) == 0 {
		return nil, nil
	}
	zones, err := store.ListZonesByIDs(ctx, tx, es, zoneIDs)
	if err != nil {
		return nil, err
	}
	counts, err := store.MappingActiveSpotCountsByZone(ctx, tx, es, zoneIDs)
	if err != nil {
		return nil, err
	}
	pools, err := store.ListZonePools(ctx, tx, es, zoneIDs)
	if err != nil {
		return nil, err
	}
	system := make(map[string]*store.Pool, len(pools))
	for i := range pools {
		if pools[i].SystemManaged && pools[i].ZoneID != nil {
			system[*pools[i].ZoneID] = &pools[i]
		}
	}
	now := s.now()
	var touched []string
	for i := range zones {
		z := &zones[i]
		pool := system[z.ID]
		switch {
		case z.AssignmentMode == ModeZone && pool == nil:
			zoneID := z.ID
			np := &store.Pool{
				ID: store.NewID(), ZoneID: &zoneID, ComponentCode: SystemPoolComponentCode,
				Name: maps.Clone(z.Name), Capacity: counts[z.ID], CapacityOverrides: []store.CapacityOverride{},
				SystemManaged: true, Version: 1, CreatedAt: now, UpdatedAt: now,
			}
			if err := store.InsertPool(ctx, tx, es, np); err != nil {
				return nil, fmt.Errorf("mapping: create zone pool: %w", err)
			}
			touched = append(touched, np.ID)
		case z.AssignmentMode == ModeZone:
			if pool.Capacity == counts[z.ID] && maps.Equal(pool.Name, z.Name) {
				continue
			}
			if err := store.MappingSyncSystemPool(ctx, tx, es, pool.ID, counts[z.ID], z.Name, now); err != nil {
				return nil, fmt.Errorf("mapping: update zone pool: %w", err)
			}
			touched = append(touched, pool.ID)
		case pool != nil:
			if err := store.DeletePool(ctx, tx, es, pool.ID); err != nil {
				return nil, fmt.Errorf("mapping: delete zone pool: %w", err)
			}
			touched = append(touched, pool.ID)
		}
	}
	return touched, nil
}

func uniqueSorted(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
