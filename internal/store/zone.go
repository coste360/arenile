package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Repository functions of zones (mapping area, spec 5.2 / 6.2).

// InsertZone inserts a zone of the establishment proven by es.
func InsertZone(ctx context.Context, db bun.IDB, es EstablishmentScope, z *Zone) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	z.EstablishmentID = es.ID()
	if z.Version == 0 {
		z.Version = 1
	}
	_, err := db.NewInsert().Model(z).Exec(ctx)
	return wrapWrite("insert zone", err)
}

// GetZone loads a zone of the establishment.
func GetZone(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Zone, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	z := new(Zone)
	err := db.NewSelect().Model(z).Where("z.id = ?", id).Where("z.establishment_id = ?", es.ID()).Scan(ctx)
	return z, wrapRead("get zone", err)
}

// ListZones lists the zones of the establishment ordered by id (keyset),
// optionally only those of one map.
func ListZones(ctx context.Context, db bun.IDB, es EstablishmentScope, mapID *string, cursor string, limit int) (Page[Zone], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Zone]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[Zone]{}, err
	}
	limit = mappingPageLimit(limit)
	var items []Zone
	q := db.NewSelect().Model(&items).Where("z.establishment_id = ?", es.ID()).OrderExpr("z.id ASC").Limit(int64(limit + 1))
	if mapID != nil {
		q = q.Where("z.map_id = ?", *mapID)
	}
	if after != "" {
		q = q.Where("z.id > ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[Zone]{}, fmt.Errorf("store: list zones: %w", err)
	}
	return mappingPageOf(items, limit, func(z Zone) string { return z.ID }), nil
}

// ListZonesByIDs loads the zones of the establishment among ids (unknown or
// foreign ids are simply absent).
func ListZonesByIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]Zone, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var items []Zone
	err := db.NewSelect().Model(&items).Where("z.establishment_id = ?", es.ID()).
		Where("z.id IN (?)", bun.List(ids)).OrderExpr("z.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list zones by id: %w", err)
	}
	return items, nil
}

// ListMapZones returns every zone attached to a map.
func ListMapZones(ctx context.Context, db bun.IDB, es EstablishmentScope, mapID string) ([]Zone, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Zone
	err := db.NewSelect().Model(&items).Where("z.establishment_id = ?", es.ID()).
		Where("z.map_id = ?", mapID).OrderExpr("z.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list map zones: %w", err)
	}
	return items, nil
}

// UpdateZoneVersioned rewrites a zone and sets version to expected+1 only if
// the stored version is still expected; it reports whether a row changed.
func UpdateZoneVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, z *Zone, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	z.EstablishmentID = es.ID()
	z.Version = expected + 1
	res, err := db.NewUpdate().Model(z).
		Column("map_id", "code", "name", "assignment_mode", "online_visible", "online_quota", "price_category",
			"auto_assign", "sort_order", "version", "updated_at").
		Where("z.id = ?", z.ID).Where("z.establishment_id = ?", es.ID()).Where("z.version = ?", expected).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update zone", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteZone deletes a zone of the establishment (its pools cascade).
func DeleteZone(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewDelete().TableExpr("zones").Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("delete zone", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// LockZones takes a write lock on the given zone rows without changing them
// (PostgreSQL row locks; SQLite write transactions are already serialized),
// so that map saves and zone changes touching the same zones are
// serialized: a save that adds spots to a zone and an update that turns the
// zone into `quantity` can then never both commit. Unknown ids are ignored.
// Callers lock zones before any other row of the map (spots, the map
// itself) to keep a stable lock order.
func LockZones(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := db.NewUpdate().Model((*Zone)(nil)).Set("version = version").
		Where("establishment_id = ?", es.ID()).Where("id IN (?)", bun.List(ids)).Exec(ctx)
	return wrapWrite("lock zones", err)
}
