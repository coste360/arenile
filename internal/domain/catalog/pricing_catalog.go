package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// PricingCatalog returns the catalog of an establishment in the shape of the
// pure pricing engine (pricing.Catalog): settings, packs and extras
// (inactive ones included, with Active false), zones and spots. Requires the
// view permission.
func (s *Service) PricingCatalog(ctx context.Context, p *auth.Principal, establishmentID string) (pricing.Catalog, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return pricing.Catalog{}, err
	}
	return LoadPricingCatalog(ctx, s.d.DB, es)
}

// LoadPricingCatalog is PricingCatalog for callers that already hold an
// authorized scope (pricingconfig, booking, jobs), possibly inside their own
// transaction (db may be a bun.Tx).
func LoadPricingCatalog(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (pricing.Catalog, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return pricing.Catalog{}, ErrEstablishmentNotFound
		}
		return pricing.Catalog{}, fmt.Errorf("catalog: pricing catalog: %w", err)
	}
	settings, err := establishment.ParseSettings(est.Settings, est.Currency)
	if err != nil {
		return pricing.Catalog{}, fmt.Errorf("catalog: pricing catalog: %w", err)
	}
	packs, err := store.ListAllPacks(ctx, db, es)
	if err != nil {
		return pricing.Catalog{}, fmt.Errorf("catalog: pricing catalog: %w", err)
	}
	extras, err := store.ListAllExtras(ctx, db, es)
	if err != nil {
		return pricing.Catalog{}, fmt.Errorf("catalog: pricing catalog: %w", err)
	}
	zones, err := store.CatalogListZones(ctx, db, es)
	if err != nil {
		return pricing.Catalog{}, fmt.Errorf("catalog: pricing catalog: %w", err)
	}
	spots, err := store.CatalogListSpots(ctx, db, es)
	if err != nil {
		return pricing.Catalog{}, fmt.Errorf("catalog: pricing catalog: %w", err)
	}
	return BuildPricingCatalog(settings.Pricing, packs, extras, zones, spots), nil
}

// BuildPricingCatalog maps store rows to the engine's catalog (pure).
func BuildPricingCatalog(settings pricing.Settings, packs []store.Pack, extras []store.Extra, zones []store.Zone, spots []store.Spot) pricing.Catalog {
	cat := pricing.Catalog{
		Settings: settings,
		Packs:    make([]pricing.Pack, 0, len(packs)),
		Extras:   make([]pricing.Extra, 0, len(extras)),
		Zones:    make([]pricing.Zone, 0, len(zones)),
		Spots:    make([]pricing.Spot, 0, len(spots)),
	}
	for _, p := range packs {
		cat.Packs = append(cat.Packs, pricing.Pack{ID: p.ID, Code: p.Code, VATRateBP: cloneInt64(p.VATRateBP), Active: p.Active})
	}
	for _, x := range extras {
		cat.Extras = append(cat.Extras, pricing.Extra{
			ID: x.ID, Code: x.Code, PricingUnit: pricing.PricingUnit(x.PricingUnit),
			VATRateBP: cloneInt64(x.VATRateBP), Active: x.Active,
		})
	}
	for _, z := range zones {
		cat.Zones = append(cat.Zones, pricing.Zone{ID: z.ID, Code: z.Code, PriceCategory: z.PriceCategory})
	}
	for _, sp := range spots {
		ps := pricing.Spot{ID: sp.ID, Label: sp.Label, ZoneID: sp.ZoneID}
		if sp.PriceCategoryOverride != nil {
			ps.PriceCategoryOverride = *sp.PriceCategoryOverride
		}
		if len(sp.Attributes) > 0 {
			ps.Attributes = append([]string(nil), sp.Attributes...)
		}
		cat.Spots = append(cat.Spots, ps)
	}
	return cat
}

func cloneInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
