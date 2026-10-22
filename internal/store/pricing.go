package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository functions of the pricing configuration area (spec 6.4): price
// rules, fixed duration prices, promo codes and agreements, plus the
// read/write helpers on other areas' tables that the pricing tools need
// (seasons, rate periods, catalog, zones, spots, closure days). Those helpers
// are prefixed with "Pricing" so that they never clash with the owning
// area's repository functions.
//
// Every function takes an EstablishmentScope and filters on establishment_id.
// Updates are optimistic: they match the row on (id, establishment_id,
// version) and report whether a row was updated, so that the caller can turn
// a lost race into a version conflict.

// PriceRuleFilter narrows ListPriceRules.
type PriceRuleFilter struct {
	SeasonID   string
	Stage      string
	TargetType string
}

// FixedPriceFilter narrows ListFixedPrices.
type FixedPriceFilter struct {
	SeasonID string
	PackID   string
}

// PricingActiveFilter narrows lists by the active flag (nil = all).
type PricingActiveFilter struct {
	Active *bool
}

// pricingKeysetPage runs q (already filtered) with id keyset pagination on alias.
func pricingKeysetPage[T any](ctx context.Context, q *bun.SelectQuery, items *[]T, alias, cursor string, limit int, idOf func(*T) string, op string) (Page[T], error) {
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

// pricingUpdateVersioned writes every column of model (whose Version must already be
// oldVersion+1) when the stored row still has oldVersion.
func pricingUpdateVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, model any, oldVersion int64, op string) (bool, error) {
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

// pricingDeleteScoped deletes the row id of table in scope; ErrNotFound when absent.
func pricingDeleteScoped(ctx context.Context, db bun.IDB, es EstablishmentScope, model any, id, op string) error {
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

func pricingGetScoped(ctx context.Context, db bun.IDB, es EstablishmentScope, model any, alias, id, op string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	err := db.NewSelect().Model(model).
		Where(alias+".id = ?", id).
		Where(alias+".establishment_id = ?", es.ID()).
		Scan(ctx)
	return wrapRead(op, err)
}

// ---- price rules ------------------------------------------------------------

// NextPriceRuleSeq returns max(seq)+1 over the establishment's price rules
// (1 when there are none): the recency tie-break of spec 8.4.
func NextPriceRuleSeq(ctx context.Context, db bun.IDB, es EstablishmentScope) (int64, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var maxSeq *int64
	err := db.NewSelect().TableExpr("price_rules AS pr").ColumnExpr("MAX(pr.seq)").
		Where("pr.establishment_id = ?", es.ID()).Scan(ctx, &maxSeq)
	if err != nil && !isNoRows(err) {
		return 0, fmt.Errorf("store: next price rule seq: %w", err)
	}
	if maxSeq == nil {
		return 1, nil
	}
	return *maxSeq + 1, nil
}

// InsertPriceRule inserts r (EstablishmentID must match es).
func InsertPriceRule(ctx context.Context, db bun.IDB, es EstablishmentScope, r *PriceRule) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	r.EstablishmentID = es.ID()
	if len(r.TargetIDs) == 0 {
		r.TargetIDs = nil
	}
	_, err := db.NewInsert().Model(r).Exec(ctx)
	return wrapWrite("insert price rule", err)
}

// GetPriceRule loads a price rule in scope.
func GetPriceRule(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*PriceRule, error) {
	r := new(PriceRule)
	if err := pricingGetScoped(ctx, db, es, r, "pr", id, "get price rule"); err != nil {
		return nil, err
	}
	return r, nil
}

// UpdatePriceRule writes r if the stored version is still oldVersion.
func UpdatePriceRule(ctx context.Context, db bun.IDB, es EstablishmentScope, r *PriceRule, oldVersion int64) (bool, error) {
	if len(r.TargetIDs) == 0 {
		r.TargetIDs = nil
	}
	return pricingUpdateVersioned(ctx, db, es, r, oldVersion, "update price rule")
}

// DeletePriceRule deletes a price rule in scope.
func DeletePriceRule(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return pricingDeleteScoped(ctx, db, es, (*PriceRule)(nil), id, "delete price rule")
}

// ListPriceRules pages the establishment's price rules ordered by id.
func ListPriceRules(ctx context.Context, db bun.IDB, es EstablishmentScope, f PriceRuleFilter, cursor string, limit int) (Page[PriceRule], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[PriceRule]{}, err
	}
	var items []PriceRule
	q := db.NewSelect().Model(&items).Where("pr.establishment_id = ?", es.ID())
	if f.SeasonID != "" {
		q = q.Where("pr.season_id = ?", f.SeasonID)
	}
	if f.Stage != "" {
		q = q.Where("pr.stage = ?", f.Stage)
	}
	if f.TargetType != "" {
		q = q.Where("pr.target_type = ?", f.TargetType)
	}
	return pricingKeysetPage(ctx, q, &items, "pr", cursor, limit, func(r *PriceRule) string { return r.ID }, "list price rules")
}

// ListSeasonPriceRules returns every price rule of a season ordered by seq.
func ListSeasonPriceRules(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) ([]PriceRule, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []PriceRule{}
	err := db.NewSelect().Model(&items).
		Where("pr.establishment_id = ?", es.ID()).Where("pr.season_id = ?", seasonID).
		OrderExpr("pr.seq ASC, pr.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list season price rules: %w", err)
	}
	return items, nil
}

// CountSeasonPriceRules counts the price rules of a season.
func CountSeasonPriceRules(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	n, err := db.NewSelect().Model((*PriceRule)(nil)).
		Where("pr.establishment_id = ?", es.ID()).Where("pr.season_id = ?", seasonID).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count price rules: %w", err)
	}
	return int(n), nil
}

// ---- fixed duration prices --------------------------------------------------

// InsertFixedPrice inserts a fixed duration price.
func InsertFixedPrice(ctx context.Context, db bun.IDB, es EstablishmentScope, fp *FixedDurationPrice) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	fp.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(fp).Exec(ctx)
	return wrapWrite("insert fixed price", err)
}

// GetFixedPrice loads a fixed price in scope.
func GetFixedPrice(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*FixedDurationPrice, error) {
	fp := new(FixedDurationPrice)
	if err := pricingGetScoped(ctx, db, es, fp, "fp", id, "get fixed price"); err != nil {
		return nil, err
	}
	return fp, nil
}

// UpdateFixedPrice writes fp if the stored version is still oldVersion.
func UpdateFixedPrice(ctx context.Context, db bun.IDB, es EstablishmentScope, fp *FixedDurationPrice, oldVersion int64) (bool, error) {
	return pricingUpdateVersioned(ctx, db, es, fp, oldVersion, "update fixed price")
}

// DeleteFixedPrice deletes a fixed price in scope.
func DeleteFixedPrice(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return pricingDeleteScoped(ctx, db, es, (*FixedDurationPrice)(nil), id, "delete fixed price")
}

// ListFixedPrices pages fixed prices ordered by id.
func ListFixedPrices(ctx context.Context, db bun.IDB, es EstablishmentScope, f FixedPriceFilter, cursor string, limit int) (Page[FixedDurationPrice], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[FixedDurationPrice]{}, err
	}
	var items []FixedDurationPrice
	q := db.NewSelect().Model(&items).Where("fp.establishment_id = ?", es.ID())
	if f.SeasonID != "" {
		q = q.Where("fp.season_id = ?", f.SeasonID)
	}
	if f.PackID != "" {
		q = q.Where("fp.pack_id = ?", f.PackID)
	}
	return pricingKeysetPage(ctx, q, &items, "fp", cursor, limit, func(r *FixedDurationPrice) string { return r.ID }, "list fixed prices")
}

// ListSeasonFixedPrices returns every fixed price of a season.
func ListSeasonFixedPrices(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) ([]FixedDurationPrice, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []FixedDurationPrice{}
	err := db.NewSelect().Model(&items).
		Where("fp.establishment_id = ?", es.ID()).Where("fp.season_id = ?", seasonID).
		OrderExpr("fp.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list season fixed prices: %w", err)
	}
	return items, nil
}

// CountSeasonFixedPrices counts the fixed prices of a season.
func CountSeasonFixedPrices(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	n, err := db.NewSelect().Model((*FixedDurationPrice)(nil)).
		Where("fp.establishment_id = ?", es.ID()).Where("fp.season_id = ?", seasonID).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count fixed prices: %w", err)
	}
	return int(n), nil
}

// ---- promo codes ------------------------------------------------------------

// InsertPromoCode inserts a promo code; a code differing only in case from an
// existing one of the establishment is ErrConflict.
func InsertPromoCode(ctx context.Context, db bun.IDB, es EstablishmentScope, pc *PromoCode) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	pc.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(pc).Exec(ctx)
	return wrapWrite("insert promo code", err)
}

// GetPromoCode loads a promo code in scope.
func GetPromoCode(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*PromoCode, error) {
	pc := new(PromoCode)
	if err := pricingGetScoped(ctx, db, es, pc, "pc", id, "get promo code"); err != nil {
		return nil, err
	}
	return pc, nil
}

// GetPromoCodeByCode loads a promo code by code, ignoring case.
func GetPromoCodeByCode(ctx context.Context, db bun.IDB, es EstablishmentScope, code string) (*PromoCode, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	pc := new(PromoCode)
	err := db.NewSelect().Model(pc).
		Where("pc.establishment_id = ?", es.ID()).
		Where("lower(pc.code) = ?", strings.ToLower(strings.TrimSpace(code))).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get promo code by code", err)
	}
	return pc, nil
}

// UpdatePromoCode writes pc if the stored version is still oldVersion.
func UpdatePromoCode(ctx context.Context, db bun.IDB, es EstablishmentScope, pc *PromoCode, oldVersion int64) (bool, error) {
	return pricingUpdateVersioned(ctx, db, es, pc, oldVersion, "update promo code")
}

// DeletePromoCode deletes a promo code in scope.
func DeletePromoCode(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return pricingDeleteScoped(ctx, db, es, (*PromoCode)(nil), id, "delete promo code")
}

// ListPromoCodes pages promo codes ordered by id.
func ListPromoCodes(ctx context.Context, db bun.IDB, es EstablishmentScope, f PricingActiveFilter, cursor string, limit int) (Page[PromoCode], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[PromoCode]{}, err
	}
	var items []PromoCode
	q := db.NewSelect().Model(&items).Where("pc.establishment_id = ?", es.ID())
	if f.Active != nil {
		q = q.Where("pc.active = ?", *f.Active)
	}
	return pricingKeysetPage(ctx, q, &items, "pc", cursor, limit, func(r *PromoCode) string { return r.ID }, "list promo codes")
}

// ---- agreements -------------------------------------------------------------

// InsertAgreement inserts an agreement.
func InsertAgreement(ctx context.Context, db bun.IDB, es EstablishmentScope, a *Agreement) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	a.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(a).Exec(ctx)
	return wrapWrite("insert agreement", err)
}

// GetAgreement loads an agreement in scope.
func GetAgreement(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Agreement, error) {
	a := new(Agreement)
	if err := pricingGetScoped(ctx, db, es, a, "ag", id, "get agreement"); err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAgreement writes a if the stored version is still oldVersion.
func UpdateAgreement(ctx context.Context, db bun.IDB, es EstablishmentScope, a *Agreement, oldVersion int64) (bool, error) {
	return pricingUpdateVersioned(ctx, db, es, a, oldVersion, "update agreement")
}

// DeleteAgreement deletes an agreement in scope.
func DeleteAgreement(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return pricingDeleteScoped(ctx, db, es, (*Agreement)(nil), id, "delete agreement")
}

// ListAgreements pages agreements ordered by id.
func ListAgreements(ctx context.Context, db bun.IDB, es EstablishmentScope, f PricingActiveFilter, cursor string, limit int) (Page[Agreement], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Agreement]{}, err
	}
	var items []Agreement
	q := db.NewSelect().Model(&items).Where("ag.establishment_id = ?", es.ID())
	if f.Active != nil {
		q = q.Where("ag.active = ?", *f.Active)
	}
	return pricingKeysetPage(ctx, q, &items, "ag", cursor, limit, func(r *Agreement) string { return r.ID }, "list agreements")
}

// ---- reads/writes on other areas used by the pricing tools ------------------

// PricingGetSeason loads a season in scope.
func PricingGetSeason(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Season, error) {
	s := new(Season)
	if err := pricingGetScoped(ctx, db, es, s, "sn", id, "pricing get season"); err != nil {
		return nil, err
	}
	return s, nil
}

// PricingSeasonByYear loads the season of a year (ErrNotFound when absent).
func PricingSeasonByYear(ctx context.Context, db bun.IDB, es EstablishmentScope, year int) (*Season, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(Season)
	err := db.NewSelect().Model(s).Where("sn.establishment_id = ?", es.ID()).Where("sn.year = ?", year).Limit(1).Scan(ctx)
	if err != nil {
		return nil, wrapRead("pricing season by year", err)
	}
	return s, nil
}

// PricingSeasonForDate loads the season whose [opens_on, closes_on] contains d.
func PricingSeasonForDate(ctx context.Context, db bun.IDB, es EstablishmentScope, d calendar.Date) (*Season, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(Season)
	err := db.NewSelect().Model(s).Where("sn.establishment_id = ?", es.ID()).
		Where("sn.opens_on <= ?", d).Where("sn.closes_on >= ?", d).
		OrderExpr("sn.year DESC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, wrapRead("pricing season for date", err)
	}
	return s, nil
}

// PricingInsertSeason inserts a season (used by the season copy).
func PricingInsertSeason(ctx context.Context, db bun.IDB, es EstablishmentScope, s *Season) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	s.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(s).Exec(ctx)
	return wrapWrite("pricing insert season", err)
}

// PricingListRatePeriods returns the rate periods of a season ordered by
// sort_order, code.
func PricingListRatePeriods(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) ([]RatePeriod, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []RatePeriod{}
	err := db.NewSelect().Model(&items).Where("rp.establishment_id = ?", es.ID()).Where("rp.season_id = ?", seasonID).
		OrderExpr("rp.sort_order ASC, rp.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pricing list rate periods: %w", err)
	}
	return items, nil
}

// PricingInsertRatePeriod inserts a rate period (used by the season copy).
func PricingInsertRatePeriod(ctx context.Context, db bun.IDB, es EstablishmentScope, rp *RatePeriod) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	rp.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(rp).Exec(ctx)
	return wrapWrite("pricing insert rate period", err)
}

// PricingListPacks returns the establishment's packs in catalog order.
func PricingListPacks(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Pack, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []Pack{}
	err := db.NewSelect().Model(&items).Where("pa.establishment_id = ?", es.ID()).
		OrderExpr("pa.sort_order ASC, pa.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pricing list packs: %w", err)
	}
	return items, nil
}

// PricingListExtras returns the establishment's extras in catalog order.
func PricingListExtras(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Extra, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []Extra{}
	err := db.NewSelect().Model(&items).Where("ex.establishment_id = ?", es.ID()).
		OrderExpr("ex.sort_order ASC, ex.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pricing list extras: %w", err)
	}
	return items, nil
}

// PricingListZones returns the establishment's zones in display order.
func PricingListZones(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Zone, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []Zone{}
	err := db.NewSelect().Model(&items).Where("z.establishment_id = ?", es.ID()).
		OrderExpr("z.sort_order ASC, z.code ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pricing list zones: %w", err)
	}
	return items, nil
}

// PricingListSpotsByID returns the spots of the establishment among ids.
func PricingListSpotsByID(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []Spot{}
	if len(ids) == 0 {
		return items, nil
	}
	err := db.NewSelect().Model(&items).Where("sp.establishment_id = ?", es.ID()).
		Where("sp.id IN (?)", bun.List(ids)).OrderExpr("sp.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pricing list spots: %w", err)
	}
	return items, nil
}

// PricingListClosureDays returns the closure days in [from, to].
func PricingListClosureDays(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]ClosureDay, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []ClosureDay{}
	err := db.NewSelect().Model(&items).Where("cd.establishment_id = ?", es.ID()).
		Where("cd.date >= ?", from).Where("cd.date <= ?", to).OrderExpr("cd.date ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pricing list closure days: %w", err)
	}
	return items, nil
}
