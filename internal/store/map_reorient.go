package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository function of the map sea-side change (M5A, spec 11). Owner:
// internal/domain/mapping (ReorientMap).

// FutureAllocations summarises the allocations of some spots from a date on.
type FutureAllocations struct {
	// Allocations counts the distinct owners (booking line or block) of
	// booking, season and block rows: one multi-day booking line is one
	// allocation, not one per day.
	Allocations int
	// SpotIDs are the spots with at least one such row, sorted.
	SpotIDs []string
}

// FutureAllocationsOf returns the booking, season and block allocations of
// spotIDs on or after from. Holds are transient and left out: they never
// survive long enough to be affected by a layout change.
func FutureAllocationsOf(ctx context.Context, db bun.IDB, es EstablishmentScope, spotIDs []string, from calendar.Date) (FutureAllocations, error) {
	var out FutureAllocations
	if err := es.mustBeValid(); err != nil {
		return out, err
	}
	if len(spotIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		SpotID string  `bun:"spot_id"`
		LineID *string `bun:"booking_line_id"`
		Block  *string `bun:"block_id"`
	}
	err := db.NewSelect().TableExpr("spot_allocations AS sa").
		ColumnExpr("DISTINCT sa.spot_id, sa.booking_line_id, sa.block_id").
		Where("sa.establishment_id = ?", es.ID()).
		Where("sa.spot_id IN (?)", bun.List(spotIDs)).
		Where("sa.date >= ?", from).
		Where("sa.kind IN ('booking', 'season', 'block')").
		OrderExpr("sa.spot_id").
		Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return out, fmt.Errorf("store: future allocations: %w", err)
	}
	owners := map[string]bool{}
	for _, r := range rows {
		switch {
		case r.LineID != nil:
			owners["l:"+*r.LineID] = true
		case r.Block != nil:
			owners["b:"+*r.Block] = true
		}
		if n := len(out.SpotIDs); n == 0 || out.SpotIDs[n-1] != r.SpotID {
			out.SpotIDs = append(out.SpotIDs, r.SpotID)
		}
	}
	out.Allocations = len(owners)
	return out, nil
}
