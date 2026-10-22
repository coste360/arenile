package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/store"
)

// Demo data identifiers.
const (
	DemoOrganizationName = "Lido Demo"
	DemoSlug             = "lido-demo"
)

// SeedResult reports what SeedDemo did.
type SeedResult struct {
	OrganizationID  string
	EstablishmentID string
	Created         bool
}

// SeedDemo creates the demo organization and establishment. It is
// idempotent: when the demo establishment already exists nothing changes.
// Later milestones extend it with inventory, catalog and prices.
func SeedDemo(ctx context.Context, db bun.IDB, c clock.Clock) (*SeedResult, error) {
	res := &SeedResult{}
	err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		if e, err := store.GetEstablishmentBySlug(ctx, tx, DemoSlug); err == nil {
			res.OrganizationID, res.EstablishmentID = e.OrganizationID, e.ID
			return nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		now := c.Now()
		org, err := store.FindOrganizationByName(ctx, tx, DemoOrganizationName)
		if errors.Is(err, store.ErrNotFound) {
			vat := "IT00000000000"
			org = &store.Organization{ID: store.NewID(), Name: DemoOrganizationName, VATNumber: &vat, CreatedAt: now, UpdatedAt: now}
			if err := store.InsertOrganization(ctx, tx, org); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		e := store.NewEstablishmentDefaults(org.ID, DemoSlug, "Lido Demo", now)
		e.EnabledLocales = []string{"it", "en", "de", "fr"}
		e.ContactEmail = "info@lido-demo.example"
		e.ContactPhone = "+39 0771 000000"
		e.Address = store.Address{Street: "Lungomare Caboto 1", City: "Gaeta", Province: "LT", PostalCode: "04024", Country: "IT"}
		e.Geo = &store.Geo{Lat: 41.2152, Lon: 13.5697}
		e.BookingEnabled = true
		showPrices, hideHeader := true, true
		e.Theme = store.Theme{
			PrimaryColor: "#0E6BA8", AccentColor: "#F2B134", FontFamily: "Inter", Radius: "md",
			ShowPricesOnMap: &showPrices, HideHeaderInEmbed: &hideHeader,
			CustomTexts: map[string]map[string]string{
				"it": {"intro": "Prenota il tuo ombrellone al Lido Demo."},
				"en": {"intro": "Book your beach umbrella at Lido Demo."},
			},
		}
		if err := store.InsertEstablishment(ctx, tx, e); err != nil {
			return err
		}
		res.OrganizationID, res.EstablishmentID, res.Created = org.ID, e.ID, true
		return audit.NewWriter(c).Record(ctx, tx, audit.Entry{
			EstablishmentID: e.ID, Actor: audit.SystemActor, Action: "seed.demo", EntityType: "establishment", EntityID: e.ID,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("app: seed demo: %w", err)
	}
	return res, nil
}
