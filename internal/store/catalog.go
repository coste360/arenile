package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/uptrace/bun"
)

// Repository functions of the catalog area (spec 6.3): components, packs and
// extras, plus the read helpers on other areas' tables that the catalog
// validation needs (pools, zones, price rules, fixed prices, booking
// policies). Those helpers are prefixed with "Catalog" so that they never
// clash with the owning area's repository functions.
//
// Every function takes an EstablishmentScope and filters on establishment_id.
// Updates are optimistic: they match the row on (id, establishment_id,
// version) and report whether a row was updated, so that the caller can turn
// a lost race into a version conflict.

// CatalogActiveFilter narrows pack and extra lists by the active flag
// (nil = all).
type CatalogActiveFilter struct {
	Active *bool
}

// catalogKeysetPage runs q (already filtered) with id keyset pagination on alias.
func catalogKeysetPage[T any](ctx context.Context, q *bun.SelectQuery, items *[]T, alias, cursor string, limit int, idOf func(*T) string, op string) (Page[T], error) {
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[T]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	q = q.OrderExpr(alias + ".id ASC").Limit(int64(limit + 1))
	if after != "" {
		q = q.Where(alias+".id > ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[T]{}, fmt.Errorf("store: %s: %w", op, err)
	}
	page := Page[T]{Items: *items}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = EncodeCursor(idOf(&page.Items[limit-1]))
	}
	if page.Items == nil {
		page.Items = []T{}
	}
	return page, nil
}

func catalogGetScoped(ctx context.Context, db bun.IDB, es EstablishmentScope, model any, alias, id, op string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	err := db.NewSelect().Model(model).
		Where(alias+".id = ?", id).
		Where(alias+".establishment_id = ?", es.ID()).
		Scan(ctx)
	return wrapRead(op, err)
}

// catalogUpdateVersioned writes every column of model (whose Version must
// already be oldVersion+1) when the stored row still has oldVersion.
func catalogUpdateVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, model any, oldVersion int64, op string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model(model).WherePK().
		Where("establishment_id = ?", es.ID()).
		Where("version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite(op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: %s: %w", op, err)
	}
	return n == 1, nil
}

func catalogDeleteScoped(ctx context.Context, db bun.IDB, es EstablishmentScope, model any, id, op string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewDelete().Model(model).Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite(op, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func catalogInsert(ctx context.Context, db bun.IDB, es EstablishmentScope, model any, establishmentID, op string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if establishmentID != es.ID() {
		return fmt.Errorf("store: %s: establishment mismatch", op)
	}
	_, err := db.NewInsert().Model(model).Exec(ctx)
	return wrapWrite(op, err)
}

// ---- components -------------------------------------------------------------

// InsertComponent inserts a component of the establishment proven by es.
func InsertComponent(ctx context.Context, db bun.IDB, es EstablishmentScope, c *Component) error {
	return catalogInsert(ctx, db, es, c, c.EstablishmentID, "insert component")
}

// GetComponent loads a component in scope.
func GetComponent(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Component, error) {
	c := new(Component)
	return c, catalogGetScoped(ctx, db, es, c, "cp", id, "get component")
}

// UpdateComponent updates a component when its stored version is oldVersion.
func UpdateComponent(ctx context.Context, db bun.IDB, es EstablishmentScope, c *Component, oldVersion int64) (bool, error) {
	return catalogUpdateVersioned(ctx, db, es, c, oldVersion, "update component")
}

// DeleteComponent deletes a component in scope.
func DeleteComponent(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return catalogDeleteScoped(ctx, db, es, (*Component)(nil), id, "delete component")
}

// ListComponents lists the components in scope by id (creation order).
func ListComponents(ctx context.Context, db bun.IDB, es EstablishmentScope, cursor string, limit int) (Page[Component], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Component]{}, err
	}
	var items []Component
	q := db.NewSelect().Model(&items).Where("cp.establishment_id = ?", es.ID())
	return catalogKeysetPage(ctx, q, &items, "cp", cursor, limit, func(c *Component) string { return c.ID }, "list components")
}

// ListAllComponents returns every component in scope ordered by code.
func ListAllComponents(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Component, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Component
	err := db.NewSelect().Model(&items).Where("cp.establishment_id = ?", es.ID()).OrderExpr("cp.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list all components: %w", err)
	}
	return items, nil
}

// ---- packs ------------------------------------------------------------------

func catalogNormalizePack(p *Pack) {
	if p.Name == nil {
		p.Name = I18nText{}
	}
	if p.Description == nil {
		p.Description = I18nText{}
	}
	if p.Composition == nil {
		p.Composition = map[string]int{}
	}
	if len(p.RequiresSpotKind) == 0 {
		p.RequiresSpotKind = nil
	}
	if len(p.AllowedZoneIDs) == 0 {
		p.AllowedZoneIDs = nil
	}
	if p.AllowedFormulas == nil {
		p.AllowedFormulas = []string{}
	}
	if p.Channels == nil {
		p.Channels = []string{}
	}
}

// InsertPack inserts a pack of the establishment proven by es.
func InsertPack(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Pack) error {
	catalogNormalizePack(p)
	return catalogInsert(ctx, db, es, p, p.EstablishmentID, "insert pack")
}

// GetPack loads a pack in scope.
func GetPack(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Pack, error) {
	p := new(Pack)
	return p, catalogGetScoped(ctx, db, es, p, "pa", id, "get pack")
}

// UpdatePack updates a pack when its stored version is oldVersion.
func UpdatePack(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Pack, oldVersion int64) (bool, error) {
	catalogNormalizePack(p)
	return catalogUpdateVersioned(ctx, db, es, p, oldVersion, "update pack")
}

// DeletePack deletes a pack in scope.
func DeletePack(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return catalogDeleteScoped(ctx, db, es, (*Pack)(nil), id, "delete pack")
}

// ListPacks lists the packs in scope by id (creation order).
func ListPacks(ctx context.Context, db bun.IDB, es EstablishmentScope, f CatalogActiveFilter, cursor string, limit int) (Page[Pack], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Pack]{}, err
	}
	var items []Pack
	q := db.NewSelect().Model(&items).Where("pa.establishment_id = ?", es.ID())
	if f.Active != nil {
		q = q.Where("pa.active = ?", *f.Active)
	}
	return catalogKeysetPage(ctx, q, &items, "pa", cursor, limit, func(p *Pack) string { return p.ID }, "list packs")
}

// ListAllPacks returns every pack in scope ordered by sort_order, code.
func ListAllPacks(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Pack, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Pack
	err := db.NewSelect().Model(&items).Where("pa.establishment_id = ?", es.ID()).
		OrderExpr("pa.sort_order ASC, pa.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list all packs: %w", err)
	}
	return items, nil
}

// CatalogExistingPackIDs returns the subset of ids that are packs in scope.
func CatalogExistingPackIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]string, error) {
	return catalogExistingIDs(ctx, db, es, "packs", ids, "existing pack ids")
}

// ---- extras -----------------------------------------------------------------

func catalogNormalizeExtra(x *Extra) {
	if x.Name == nil {
		x.Name = I18nText{}
	}
	if x.Description == nil {
		x.Description = I18nText{}
	}
	if len(x.AllowedPackIDs) == 0 {
		x.AllowedPackIDs = nil
	}
	if x.Channels == nil {
		x.Channels = []string{}
	}
}

// InsertExtra inserts an extra of the establishment proven by es.
func InsertExtra(ctx context.Context, db bun.IDB, es EstablishmentScope, x *Extra) error {
	catalogNormalizeExtra(x)
	return catalogInsert(ctx, db, es, x, x.EstablishmentID, "insert extra")
}

// GetExtra loads an extra in scope.
func GetExtra(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Extra, error) {
	x := new(Extra)
	return x, catalogGetScoped(ctx, db, es, x, "ex", id, "get extra")
}

// UpdateExtra updates an extra when its stored version is oldVersion.
func UpdateExtra(ctx context.Context, db bun.IDB, es EstablishmentScope, x *Extra, oldVersion int64) (bool, error) {
	catalogNormalizeExtra(x)
	return catalogUpdateVersioned(ctx, db, es, x, oldVersion, "update extra")
}

// DeleteExtra deletes an extra in scope.
func DeleteExtra(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return catalogDeleteScoped(ctx, db, es, (*Extra)(nil), id, "delete extra")
}

// ListExtras lists the extras in scope by id (creation order).
func ListExtras(ctx context.Context, db bun.IDB, es EstablishmentScope, f CatalogActiveFilter, cursor string, limit int) (Page[Extra], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Extra]{}, err
	}
	var items []Extra
	q := db.NewSelect().Model(&items).Where("ex.establishment_id = ?", es.ID())
	if f.Active != nil {
		q = q.Where("ex.active = ?", *f.Active)
	}
	return catalogKeysetPage(ctx, q, &items, "ex", cursor, limit, func(x *Extra) string { return x.ID }, "list extras")
}

// ListAllExtras returns every extra in scope ordered by sort_order, code.
func ListAllExtras(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Extra, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Extra
	err := db.NewSelect().Model(&items).Where("ex.establishment_id = ?", es.ID()).
		OrderExpr("ex.sort_order ASC, ex.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list all extras: %w", err)
	}
	return items, nil
}

// ---- cross-area helpers -----------------------------------------------------

// catalogExistingIDs returns the subset of ids present in table for the
// establishment (table is a constant chosen by this file, never user input).
func catalogExistingIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, table string, ids []string, op string) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []string{}, nil
	}
	var found []string
	err := db.NewSelect().TableExpr("? AS t", bun.Ident(table)).ColumnExpr("t.id").
		Where("t.establishment_id = ?", es.ID()).
		Where("t.id IN (?)", bun.List(ids)).
		Scan(ctx, &found)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: %s: %w", op, err)
	}
	return found, nil
}

// CatalogExistingZoneIDs returns the subset of ids that are zones in scope.
func CatalogExistingZoneIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]string, error) {
	return catalogExistingIDs(ctx, db, es, "zones", ids, "existing zone ids")
}

// CatalogGetPool loads a pool in scope (to check a tracked component).
func CatalogGetPool(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Pool, error) {
	p := new(Pool)
	return p, catalogGetScoped(ctx, db, es, p, "pl", id, "catalog get pool")
}

// CatalogPoolsUsingComponent counts the pools in scope that hold
// componentCode inventory.
func CatalogPoolsUsingComponent(ctx context.Context, db bun.IDB, es EstablishmentScope, componentCode string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	n, err := db.NewSelect().Model((*Pool)(nil)).
		Where("pl.establishment_id = ?", es.ID()).
		Where("pl.component_code = ?", componentCode).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: catalog pools using component: %w", err)
	}
	return int(n), nil
}

// CatalogPriceRuleTargets returns the target_ids lists of the price rules in
// scope with the given target type (pack | extra). Rules without target_ids
// (= every item) are skipped: they do not name a specific item.
func CatalogPriceRuleTargets(ctx context.Context, db bun.IDB, es EstablishmentScope, targetType string) ([][]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []PriceRule
	err := db.NewSelect().Model(&rows).Column("id", "target_ids").
		Where("pr.establishment_id = ?", es.ID()).
		Where("pr.target_type = ?", targetType).
		Where("pr.target_ids IS NOT NULL").
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: catalog price rule targets: %w", err)
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		if len(r.TargetIDs) > 0 {
			out = append(out, r.TargetIDs)
		}
	}
	return out, nil
}

// CatalogPackHasFixedPrices reports whether fixed duration prices reference
// the pack (they would be cascade-deleted with it).
func CatalogPackHasFixedPrices(ctx context.Context, db bun.IDB, es EstablishmentScope, packID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	ok, err := db.NewSelect().Model((*FixedDurationPrice)(nil)).
		Where("fp.establishment_id = ?", es.ID()).
		Where("fp.pack_id = ?", packID).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("store: catalog pack fixed prices: %w", err)
	}
	return ok, nil
}

// CatalogPolicyScopes returns the raw scope documents of the booking
// policies in scope (policy.Scope JSON; packs may be listed by id or code).
func CatalogPolicyScopes(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]JSON, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []BookingPolicy
	err := db.NewSelect().Model(&rows).Column("id", "scope").
		Where("bp.establishment_id = ?", es.ID()).
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: catalog policy scopes: %w", err)
	}
	out := make([]JSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Scope)
	}
	return out, nil
}

// CatalogListZones returns every zone in scope (pricing view of the catalog).
func CatalogListZones(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Zone, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Zone
	err := db.NewSelect().Model(&items).Where("z.establishment_id = ?", es.ID()).
		OrderExpr("z.sort_order ASC, z.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: catalog list zones: %w", err)
	}
	return items, nil
}

// CatalogListSpots returns every spot in scope (pricing view of the catalog).
func CatalogListSpots(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Spot
	err := db.NewSelect().Model(&items).Where("sp.establishment_id = ?", es.ID()).
		OrderExpr("sp.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: catalog list spots: %w", err)
	}
	return items, nil
}

// CatalogContains reports whether any of lists contains v (helper for
// reference checks on JSON id lists).
func CatalogContains(lists [][]string, v string) bool {
	for _, l := range lists {
		if slices.Contains(l, v) {
			return true
		}
	}
	return false
}
