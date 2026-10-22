package store

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// NewEstablishmentDefaults returns an establishment with the defaults of
// spec 6.1 (Europe/Rome, EUR, Italian) ready to be inserted.
func NewEstablishmentDefaults(organizationID, slug, name string, now time.Time) *Establishment {
	return &Establishment{
		ID:                  NewID(),
		OrganizationID:      organizationID,
		Slug:                slug,
		Name:                name,
		Timezone:            "Europe/Rome",
		Currency:            "EUR",
		DefaultLocale:       "it",
		EnabledLocales:      []string{"it"},
		AllowedEmbedOrigins: []string{},
		Theme:               Theme{},
		Settings:            JSON("{}"),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// CreateOrganizationWithEstablishment inserts an organization and one
// establishment; a convenience for CLI seeding and tests.
func CreateOrganizationWithEstablishment(ctx context.Context, db bun.IDB, orgName, slug, estName string, now time.Time) (*Organization, *Establishment, error) {
	org := &Organization{ID: NewID(), Name: orgName, CreatedAt: now, UpdatedAt: now}
	if err := InsertOrganization(ctx, db, org); err != nil {
		return nil, nil, err
	}
	e := NewEstablishmentDefaults(org.ID, slug, estName, now)
	if err := InsertEstablishment(ctx, db, e); err != nil {
		return nil, nil, err
	}
	return org, e, nil
}
