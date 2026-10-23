package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository functions of the inventory area (spot_allocations, holds,
// spot_blocks, pool_usage, pool_allocations). Owner: internal/domain/inventory.

// SpotsWithAllocationsFrom returns the spots among spotIDs that have an
// allocation on or after from; hold rows count only while expires_at > now.
func SpotsWithAllocationsFrom(ctx context.Context, db bun.IDB, es EstablishmentScope, spotIDs []string, from calendar.Date, now time.Time) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(spotIDs) == 0 {
		return nil, nil
	}
	var out []string
	err := db.NewSelect().TableExpr("spot_allocations AS sa").ColumnExpr("DISTINCT sa.spot_id").
		Where("sa.establishment_id = ?", es.ID()).
		Where("sa.spot_id IN (?)", bun.List(spotIDs)).
		Where("sa.date >= ?", from).
		Where("(sa.kind <> 'hold' OR sa.expires_at > ?)", now.UTC()).
		OrderExpr("sa.spot_id").
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: spots with allocations: %w", err)
	}
	return out, nil
}
