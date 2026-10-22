package mapping

import (
	"context"
	"fmt"
	"slices"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// Reference kinds of ZoneReference.
const (
	RefPriceRule     = "price_rule"
	RefPromoCode     = "promo_code"
	RefAgreement     = "agreement"
	RefPack          = "pack"
	RefBookingPolicy = "booking_policy"
)

// zoneReferences returns the configuration entities that name the zones in
// zones (matched by id and, for policy scopes, by code), grouped by zone id.
// A document that cannot be decoded counts as a reference of every zone:
// the deletion must be conservative.
func zoneReferences(ctx context.Context, db bun.IDB, es store.EstablishmentScope, zones []store.Zone) (map[string][]ZoneReference, error) {
	if len(zones) == 0 {
		return nil, nil
	}
	docs, err := store.ListZoneReferenceDocs(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("mapping: zone references: %w", err)
	}
	out := make(map[string][]ZoneReference)
	add := func(ref ZoneReference, match func(z *store.Zone) bool) {
		for i := range zones {
			if match(&zones[i]) {
				out[zones[i].ID] = append(out[zones[i].ID], ref)
			}
		}
	}
	byIDs := func(ids []string, ok bool) func(z *store.Zone) bool {
		return func(z *store.Zone) bool { return !ok || slices.Contains(ids, z.ID) }
	}
	conditions := func(raw store.JSON) ([]string, bool) {
		var c pricing.Conditions
		if err := raw.Unmarshal(&c); err != nil {
			return nil, false
		}
		return c.ZoneIDs, true
	}
	for _, r := range docs.PriceRules {
		add(ZoneReference{Kind: RefPriceRule, ID: r.ID, Name: r.Name}, byIDs(conditions(r.Conditions)))
	}
	for _, pc := range docs.PromoCodes {
		add(ZoneReference{Kind: RefPromoCode, ID: pc.ID, Name: pc.Code}, byIDs(conditions(pc.Conditions)))
	}
	for _, a := range docs.Agreements {
		add(ZoneReference{Kind: RefAgreement, ID: a.ID, Name: a.Name}, byIDs(conditions(a.Conditions)))
	}
	for _, pk := range docs.Packs {
		add(ZoneReference{Kind: RefPack, ID: pk.ID, Name: pk.Code}, byIDs(pk.AllowedZoneIDs, true))
	}
	for _, bp := range docs.Policies {
		var sc policy.Scope
		ok := bp.Scope.Unmarshal(&sc) == nil
		add(ZoneReference{Kind: RefBookingPolicy, ID: bp.ID, Name: bp.Name}, func(z *store.Zone) bool {
			return !ok || slices.Contains(sc.Zones, z.ID) || slices.Contains(sc.Zones, z.Code)
		})
	}
	return out, nil
}
