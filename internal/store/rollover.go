package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository of the season rollover (owner: internal/domain/rollover):
// season_rollovers rows (migration 00008) and the few reads the rollover
// needs on seasons, closure days and season passes. Every function filters
// on the establishment of the scope.

// GetSeasonRollover loads the rollover of sourceSeasonID onto targetYear
// (ErrNotFound when it was never applied).
func GetSeasonRollover(ctx context.Context, db bun.IDB, es EstablishmentScope, sourceSeasonID string, targetYear int) (*SeasonRollover, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	r := new(SeasonRollover)
	err := db.NewSelect().Model(r).
		Where("sro.establishment_id = ?", es.ID()).
		Where("sro.source_season_id = ?", sourceSeasonID).
		Where("sro.target_year = ?", targetYear).
		Scan(ctx)
	return r, wrapRead("get season rollover", err)
}

// InsertSeasonRollover inserts an applied rollover (ErrConflict when the
// same source and year, or the same target season, is already recorded).
func InsertSeasonRollover(ctx context.Context, db bun.IDB, es EstablishmentScope, r *SeasonRollover) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	r.EstablishmentID = es.ID()
	if len(r.Options) == 0 {
		r.Options = JSON(`{}`)
	}
	if len(r.Result) == 0 {
		r.Result = JSON(`{}`)
	}
	_, err := db.NewInsert().Model(r).Exec(ctx)
	return wrapWrite("insert season rollover", err)
}

// ListSeasonRollovers lists the applied rollovers, newest first (ids are
// UUIDv7, so id order is creation order).
func ListSeasonRollovers(ctx context.Context, db bun.IDB, es EstablishmentScope, cursor string, limit int) (Page[SeasonRollover], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[SeasonRollover]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[SeasonRollover]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []SeasonRollover
	q := db.NewSelect().Model(&items).Where("sro.establishment_id = ?", es.ID()).
		OrderExpr("sro.id DESC").Limit(int64(limit + 1))
	if after != "" {
		q = q.Where("sro.id < ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[SeasonRollover]{}, fmt.Errorf("store: list season rollovers: %w", err)
	}
	page := Page[SeasonRollover]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	if page.Items == nil {
		page.Items = []SeasonRollover{}
	}
	return page, nil
}

// RolloverClosureDays returns the closure days of es in [from, to], by date.
func RolloverClosureDays(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]ClosureDay, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []ClosureDay{}
	err := db.NewSelect().Model(&items).
		Where("cd.establishment_id = ?", es.ID()).
		Where("cd.date >= ?", from).Where("cd.date <= ?", to).
		OrderExpr("cd.date ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: rollover closure days: %w", err)
	}
	return items, nil
}

// CountRenewableSeasonPasses counts the season passes of a season that can
// be renewed: confirmed, checked in or already completed.
func CountRenewableSeasonPasses(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	n, err := db.NewSelect().Model((*Booking)(nil)).
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.season_id = ?", seasonID).
		Where("bk.is_season_pass = ?", true).
		Where("bk.status IN (?)", bun.List(RenewableBookingStatuses)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count renewable passes: %w", err)
	}
	return int(n), nil
}

// RenewableBookingStatuses are the statuses of a season pass that can be
// renewed (the pass was sold and not cancelled).
var RenewableBookingStatuses = []string{"confirmed", "checked_in", "completed"}
