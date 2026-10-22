package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// ZoneReferenceDocs are the configuration rows of an establishment that may
// name zones inside their JSON documents: price rules, promo codes and
// agreements (pricing.Conditions.zone_ids), packs (allowed_zone_ids) and
// booking policies (policy.Scope.zones, by id or code). The store does not
// interpret the documents: the mapping service decodes them to refuse the
// deletion of a referenced zone.
type ZoneReferenceDocs struct {
	PriceRules []PriceRule     // id, name, conditions
	PromoCodes []PromoCode     // id, code, conditions
	Agreements []Agreement     // id, name, conditions
	Packs      []Pack          // id, code, allowed_zone_ids
	Policies   []BookingPolicy // id, name, scope
}

// ListZoneReferenceDocs loads the documents of ZoneReferenceDocs in scope.
func ListZoneReferenceDocs(ctx context.Context, db bun.IDB, es EstablishmentScope) (*ZoneReferenceDocs, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := &ZoneReferenceDocs{}
	load := func(dst any, alias string, cols ...string) error {
		err := db.NewSelect().Model(dst).Column(cols...).Where(alias+".establishment_id = ?", es.ID()).
			OrderExpr(alias + ".id ASC").Scan(ctx)
		if err != nil && !isNoRows(err) {
			return fmt.Errorf("store: list zone references (%s): %w", alias, err)
		}
		return nil
	}
	if err := load(&out.PriceRules, "pr", "id", "name", "conditions"); err != nil {
		return nil, err
	}
	if err := load(&out.PromoCodes, "pc", "id", "code", "conditions"); err != nil {
		return nil, err
	}
	if err := load(&out.Agreements, "ag", "id", "name", "conditions"); err != nil {
		return nil, err
	}
	if err := load(&out.Packs, "pa", "id", "code", "allowed_zone_ids"); err != nil {
		return nil, err
	}
	if err := load(&out.Policies, "bp", "id", "name", "scope"); err != nil {
		return nil, err
	}
	return out, nil
}
