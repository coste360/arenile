package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository functions of manual spot blocks (allocations kind = block).
// Owner: internal/domain/inventory.

// InsertSpotBlock inserts a block of es.
func InsertSpotBlock(ctx context.Context, db bun.IDB, es EstablishmentScope, b *SpotBlock) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	b.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(b).Exec(ctx)
	return wrapWrite("insert spot block", err)
}

// GetSpotBlock loads a block of es.
func GetSpotBlock(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*SpotBlock, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	b := new(SpotBlock)
	err := db.NewSelect().Model(b).Where("sb.id = ?", id).Where("sb.establishment_id = ?", es.ID()).Scan(ctx)
	return b, wrapRead("get spot block", err)
}

// DeleteSpotBlock deletes a block of es (its allocations must be deleted
// first by the caller, to report them; the FK cascade is a safety net).
func DeleteSpotBlock(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewDelete().TableExpr("spot_blocks").Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("delete spot block", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SpotBlockFilter narrows ListSpotBlocks; zero values are open bounds.
type SpotBlockFilter struct {
	From   calendar.Date // blocks ending on or after From
	To     calendar.Date // blocks starting on or before To
	SpotID string
}

// ListSpotBlocks lists the blocks of es by id (keyset).
func ListSpotBlocks(ctx context.Context, db bun.IDB, es EstablishmentScope, f SpotBlockFilter, cursor string, limit int) (Page[SpotBlock], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[SpotBlock]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[SpotBlock]{}, err
	}
	limit = mappingPageLimit(limit)
	var items []SpotBlock
	q := db.NewSelect().Model(&items).Where("sb.establishment_id = ?", es.ID()).OrderExpr("sb.id ASC").Limit(int64(limit + 1))
	if !f.From.IsZero() {
		q = q.Where("sb.ends_on >= ?", f.From)
	}
	if !f.To.IsZero() {
		q = q.Where("sb.starts_on <= ?", f.To)
	}
	if f.SpotID != "" {
		q = q.Where("sb.spot_id = ?", f.SpotID)
	}
	if after != "" {
		q = q.Where("sb.id > ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[SpotBlock]{}, fmt.Errorf("store: list spot blocks: %w", err)
	}
	return mappingPageOf(items, limit, func(b SpotBlock) string { return b.ID }), nil
}

// SpotBlocksByID loads the blocks with the given ids (operations view).
func SpotBlocksByID(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]SpotBlock, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var items []SpotBlock
	err := db.NewSelect().Model(&items).Where("sb.establishment_id = ?", es.ID()).Where("sb.id IN (?)", bun.List(ids)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: spot blocks by id: %w", err)
	}
	return items, nil
}
