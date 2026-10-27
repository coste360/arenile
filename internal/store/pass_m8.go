package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// SeasonAllocationsOfLine returns the season allocations of a pass line
// between from and to, by date and slot: the half-days the holder still
// occupies, i.e. neither released nor moved away (releases by range, spec
// 30.8). Owner: internal/domain/pass.
func SeasonAllocationsOfLine(ctx context.Context, db bun.IDB, es EstablishmentScope, lineID string, from, to calendar.Date) ([]SpotAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []SpotAllocation
	err := db.NewSelect().Model(&rows).
		Where("sa.establishment_id = ?", es.ID()).Where("sa.booking_line_id = ?", lineID).
		Where("sa.kind = ?", AllocationSeason).Where("sa.date >= ?", from).Where("sa.date <= ?", to).
		OrderExpr("sa.date, sa.slot").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: season allocations of line: %w", err)
	}
	return rows, nil
}
