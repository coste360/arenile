package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of the mapping area: maps, spots and decorations
// (spec 6.2). Zones live in zone.go and pools in pool.go. Every function
// takes an EstablishmentScope and filters on establishment_id.

// ---- maps -------------------------------------------------------------------

// InsertMap inserts a map of the establishment proven by es.
func InsertMap(ctx context.Context, db bun.IDB, es EstablishmentScope, m *Map) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	m.EstablishmentID = es.ID()
	if m.Version == 0 {
		m.Version = 1
	}
	_, err := db.NewInsert().Model(m).Exec(ctx)
	return wrapWrite("insert map", err)
}

// GetMap loads a map of the establishment.
func GetMap(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Map, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	m := new(Map)
	err := db.NewSelect().Model(m).Where("m.id = ?", id).Where("m.establishment_id = ?", es.ID()).Scan(ctx)
	return m, wrapRead("get map", err)
}

// ListMaps lists the maps of the establishment ordered by id (keyset).
func ListMaps(ctx context.Context, db bun.IDB, es EstablishmentScope, cursor string, limit int) (Page[Map], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Map]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[Map]{}, err
	}
	limit = mappingPageLimit(limit)
	var items []Map
	q := db.NewSelect().Model(&items).Where("m.establishment_id = ?", es.ID()).OrderExpr("m.id ASC").Limit(int64(limit + 1))
	if after != "" {
		q = q.Where("m.id > ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[Map]{}, fmt.Errorf("store: list maps: %w", err)
	}
	return mappingPageOf(items, limit, func(m Map) string { return m.ID }), nil
}

// UpdateMapVersioned rewrites the map columns and sets version to
// expected+1, only if the stored version is still expected. It reports
// whether the row was updated (false: version changed meanwhile or the map
// is gone).
func UpdateMapVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, m *Map, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	m.EstablishmentID = es.ID()
	m.Version = expected + 1
	res, err := db.NewUpdate().Model(m).
		Column("name", "width", "height", "grid_size", "background", "sea_side", "sort_order", "version", "updated_at").
		Where("m.id = ?", m.ID).Where("m.establishment_id = ?", es.ID()).Where("m.version = ?", expected).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update map", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteMap deletes a map (spots and decorations cascade).
func DeleteMap(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewDelete().TableExpr("maps").Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("delete map", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MappingSpotCountsByMap returns the number of spots of each map in mapIDs.
func MappingSpotCountsByMap(ctx context.Context, db bun.IDB, es EstablishmentScope, mapIDs []string) (map[string]int, error) {
	return mappingCountGrouped(ctx, db, es, "map_id", mapIDs, false)
}

// MappingActiveSpotCountsByZone returns the number of active spots of each
// zone in zoneIDs (zones without spots are absent).
func MappingActiveSpotCountsByZone(ctx context.Context, db bun.IDB, es EstablishmentScope, zoneIDs []string) (map[string]int, error) {
	return mappingCountGrouped(ctx, db, es, "zone_id", zoneIDs, true)
}

// MappingSpotCountsByZone returns the number of spots (active or not) of
// each zone in zoneIDs.
func MappingSpotCountsByZone(ctx context.Context, db bun.IDB, es EstablishmentScope, zoneIDs []string) (map[string]int, error) {
	return mappingCountGrouped(ctx, db, es, "zone_id", zoneIDs, false)
}

func mappingCountGrouped(ctx context.Context, db bun.IDB, es EstablishmentScope, col string, ids []string, activeOnly bool) (map[string]int, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		Key string `bun:"k"`
		N   int    `bun:"n"`
	}
	q := db.NewSelect().TableExpr("spots AS sp").
		ColumnExpr("sp."+col+" AS k").ColumnExpr("COUNT(*) AS n").
		Where("sp.establishment_id = ?", es.ID()).
		Where("sp."+col+" IN (?)", bun.List(ids)).
		GroupExpr("sp." + col)
	if activeOnly {
		q = q.Where("sp.active = ?", true)
	}
	if err := q.Scan(ctx, &rows); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: count spots by %s: %w", col, err)
	}
	for _, r := range rows {
		out[r.Key] = r.N
	}
	return out, nil
}

// ---- spots ------------------------------------------------------------------

// ListMapSpots returns every spot of a map ordered by row, number and label.
func ListMapSpots(ctx context.Context, db bun.IDB, es EstablishmentScope, mapID string) ([]Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Spot
	err := db.NewSelect().Model(&items).
		Where("sp.establishment_id = ?", es.ID()).Where("sp.map_id = ?", mapID).
		OrderExpr("sp.row_label ASC, sp.number ASC, sp.label ASC, sp.id ASC").
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list spots: %w", err)
	}
	return items, nil
}

// InsertSpots inserts spots of the establishment (no-op when empty).
func InsertSpots(ctx context.Context, db bun.IDB, es EstablishmentScope, spots []Spot) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(spots) == 0 {
		return nil
	}
	for i := range spots {
		spots[i].EstablishmentID = es.ID()
		mappingNormalizeSpot(&spots[i])
	}
	// Chunked to stay below the SQLite bound parameter limit.
	const chunk = 200
	for start := 0; start < len(spots); start += chunk {
		part := spots[start:min(start+chunk, len(spots))]
		if _, err := db.NewInsert().Model(&part).Exec(ctx); err != nil {
			return wrapWrite("insert spots", err)
		}
	}
	return nil
}

// UpdateSpot rewrites every mutable column of a spot of the establishment.
func UpdateSpot(ctx context.Context, db bun.IDB, es EstablishmentScope, s *Spot) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	s.EstablishmentID = es.ID()
	mappingNormalizeSpot(s)
	res, err := db.NewUpdate().Model(s).
		Column("zone_id", "label", "row_label", "number", "kind", "asset", "x", "y", "rotation", "shape", "capacity",
			"max_extra", "price_category_override", "attributes", "active", "updated_at").
		Where("sp.id = ?", s.ID).Where("sp.establishment_id = ?", es.ID()).Where("sp.map_id = ?", s.MapID).
		Exec(ctx)
	if err != nil {
		return wrapWrite("update spot", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSpotLabel changes only the label of a spot (used to free labels before
// a batch of renames, which would otherwise hit unique(map_id, label)).
func SetSpotLabel(ctx context.Context, db bun.IDB, es EstablishmentScope, mapID, spotID, label string, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().TableExpr("spots").Set("label = ?", label).Set("updated_at = ?", at).
		Where("id = ?", spotID).Where("map_id = ?", mapID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	return wrapWrite("set spot label", err)
}

// DeleteSpots deletes spots of a map by id.
func DeleteSpots(ctx context.Context, db bun.IDB, es EstablishmentScope, mapID string, ids []string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := db.NewDelete().TableExpr("spots").
		Where("establishment_id = ?", es.ID()).Where("map_id = ?", mapID).Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	return wrapWrite("delete spots", err)
}

func mappingNormalizeSpot(s *Spot) {
	if s.Capacity == nil {
		s.Capacity = map[string]int{}
	}
	if s.MaxExtra == nil {
		s.MaxExtra = map[string]int{}
	}
	if s.Attributes == nil {
		s.Attributes = []string{}
	}
}

// ---- decorations ------------------------------------------------------------

// ListMapDecorations returns every decoration of a map ordered by id.
func ListMapDecorations(ctx context.Context, db bun.IDB, es EstablishmentScope, mapID string) ([]MapDecoration, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []MapDecoration
	err := db.NewSelect().Model(&items).
		Where("md.establishment_id = ?", es.ID()).Where("md.map_id = ?", mapID).
		OrderExpr("md.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list decorations: %w", err)
	}
	return items, nil
}

// InsertDecorations inserts decorations of the establishment.
func InsertDecorations(ctx context.Context, db bun.IDB, es EstablishmentScope, ds []MapDecoration) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(ds) == 0 {
		return nil
	}
	for i := range ds {
		ds[i].EstablishmentID = es.ID()
	}
	const chunk = 300
	for start := 0; start < len(ds); start += chunk {
		part := ds[start:min(start+chunk, len(ds))]
		if _, err := db.NewInsert().Model(&part).Exec(ctx); err != nil {
			return wrapWrite("insert decorations", err)
		}
	}
	return nil
}

// UpdateDecoration rewrites a decoration of the establishment.
func UpdateDecoration(ctx context.Context, db bun.IDB, es EstablishmentScope, d *MapDecoration) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	d.EstablishmentID = es.ID()
	res, err := db.NewUpdate().Model(d).
		Column("kind", "asset", "variant", "x", "y", "rotation", "shape", "label", "updated_at").
		Where("md.id = ?", d.ID).Where("md.establishment_id = ?", es.ID()).Where("md.map_id = ?", d.MapID).
		Exec(ctx)
	if err != nil {
		return wrapWrite("update decoration", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDecorations deletes decorations of a map by id.
func DeleteDecorations(ctx context.Context, db bun.IDB, es EstablishmentScope, mapID string, ids []string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := db.NewDelete().TableExpr("map_decorations").
		Where("establishment_id = ?", es.ID()).Where("map_id = ?", mapID).Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	return wrapWrite("delete decorations", err)
}

// ---- pagination helpers of the mapping area ---------------------------------

func mappingPageLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	return limit
}

func mappingPageOf[T any](items []T, limit int, id func(T) string) Page[T] {
	page := Page[T]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(id(page.Items[limit-1]))
	}
	if page.Items == nil {
		page.Items = []T{}
	}
	return page
}
