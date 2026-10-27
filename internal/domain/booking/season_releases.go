package booking

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
)

// Season passes moved, swapped or put back (INV-10): the half-days the
// holder released stay free. The allocation of the moved segments skips
// them (inventory.AllocateRequest.Skip) and the release rows follow the
// pass onto the spot of each segment, so a revocation restores the day
// where the pass is on that date and a resale is tracked there.

// seasonSkips returns the active releases of line lineID of the season pass
// b between from and to as half-days to skip (nil for other bookings).
func seasonSkips(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking, lineID string, from, to calendar.Date) ([]availability.DateSlot, error) {
	if !b.IsSeasonPass || to.Before(from) {
		return nil, nil
	}
	rels, err := store.ActiveSeasonReleases(ctx, db, es, lineID, calendar.Range(from, to))
	if err != nil {
		return nil, err
	}
	out := make([]availability.DateSlot, 0, len(rels))
	for _, r := range rels {
		out = append(out, availability.DateSlot{Date: r.Date, Slot: r.Slot})
	}
	return out, nil
}

// moveSeasonReleases makes the releases of a season pass line follow its
// new segments (no-op for other bookings).
func (s *Service) moveSeasonReleases(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, kind inventory.Kind, lineID string, segs []SegmentInput) error {
	if kind != inventory.KindSeason {
		return nil
	}
	now := s.now()
	for _, sg := range segs {
		if err := store.MoveSeasonReleases(ctx, tx, es, lineID, sg.StartsOn, sg.EndsOn, sg.SpotID, now); err != nil {
			return err
		}
	}
	return nil
}
