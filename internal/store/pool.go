package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of pools (mapping area, spec 6.2 / 7.5).

// InsertPool inserts a pool of the establishment proven by es.
func InsertPool(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Pool) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	if p.Version == 0 {
		p.Version = 1
	}
	if p.CapacityOverrides == nil {
		p.CapacityOverrides = []CapacityOverride{}
	}
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert pool", err)
}

// GetPool loads a pool of the establishment.
func GetPool(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Pool, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(Pool)
	err := db.NewSelect().Model(p).Where("pl.id = ?", id).Where("pl.establishment_id = ?", es.ID()).Scan(ctx)
	return p, wrapRead("get pool", err)
}

// ListPools lists the pools of the establishment ordered by id (keyset),
// optionally only those of one zone.
func ListPools(ctx context.Context, db bun.IDB, es EstablishmentScope, zoneID *string, cursor string, limit int) (Page[Pool], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Pool]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[Pool]{}, err
	}
	limit = mappingPageLimit(limit)
	var items []Pool
	q := db.NewSelect().Model(&items).Where("pl.establishment_id = ?", es.ID()).OrderExpr("pl.id ASC").Limit(int64(limit + 1))
	if zoneID != nil {
		q = q.Where("pl.zone_id = ?", *zoneID)
	}
	if after != "" {
		q = q.Where("pl.id > ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[Pool]{}, fmt.Errorf("store: list pools: %w", err)
	}
	return mappingPageOf(items, limit, func(p Pool) string { return p.ID }), nil
}

// ListZonePools returns the pools bound to the given zones, system managed
// first, then by id.
func ListZonePools(ctx context.Context, db bun.IDB, es EstablishmentScope, zoneIDs []string) ([]Pool, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(zoneIDs) == 0 {
		return nil, nil
	}
	var items []Pool
	err := db.NewSelect().Model(&items).Where("pl.establishment_id = ?", es.ID()).
		Where("pl.zone_id IN (?)", bun.List(zoneIDs)).
		OrderExpr("pl.system_managed DESC, pl.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list zone pools: %w", err)
	}
	return items, nil
}

// UpdatePoolVersioned rewrites a pool and sets version to expected+1 only if
// the stored version is still expected; it reports whether a row changed.
func UpdatePoolVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Pool, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	p.EstablishmentID = es.ID()
	p.Version = expected + 1
	if p.CapacityOverrides == nil {
		p.CapacityOverrides = []CapacityOverride{}
	}
	res, err := db.NewUpdate().Model(p).
		Column("zone_id", "component_code", "name", "capacity", "capacity_overrides", "version", "updated_at").
		Where("pl.id = ?", p.ID).Where("pl.establishment_id = ?", es.ID()).Where("pl.version = ?", expected).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update pool", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeletePool deletes a pool of the establishment.
func DeletePool(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewDelete().TableExpr("pools").Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("delete pool", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MappingSyncSystemPool sets the capacity and name of a system managed pool
// and bumps its version, whatever the current version (the implicit pool of
// a `zone` zone is owned by the system, spec 7.5).
func MappingSyncSystemPool(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, capacity int, name I18nText, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	nameJSON, err := MarshalJSONValue(name)
	if err != nil {
		return err
	}
	res, err := db.NewUpdate().TableExpr("pools").
		Set("capacity = ?", capacity).Set("name = ?", nameJSON).
		Set("version = version + 1").Set("updated_at = ?", at).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("system_managed = ?", true).
		Exec(ctx)
	if err != nil {
		return wrapWrite("sync system pool", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
