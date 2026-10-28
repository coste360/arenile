package store

import (
	"context"
	"fmt"
	"time"

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

// MoveSeasonReleases makes the active, not yet resold releases of a pass
// line between from and to follow the pass onto spotID (the pass was moved
// or swapped from a date, INV-10): a revocation then restores the day on
// the spot the pass occupies on that date, and a resale is tracked there.
// A resold release keeps the spot the buyer occupies.
func MoveSeasonReleases(ctx context.Context, db bun.IDB, es EstablishmentScope, lineID string, from, to calendar.Date, spotID string, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().Model((*SeasonRelease)(nil)).
		Set("spot_id = ?", spotID).Set("updated_at = ?", now.UTC()).
		Where("establishment_id = ?", es.ID()).Where("booking_line_id = ?", lineID).
		Where("date >= ?", from).Where("date <= ?", to).
		Where("revoked_at IS NULL").Where("resold_booking_line_id IS NULL").Where("spot_id <> ?", spotID).
		Exec(ctx)
	return wrapWrite("move season releases", err)
}
