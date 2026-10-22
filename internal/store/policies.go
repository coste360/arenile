package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Repository functions of booking policies (spec 9). The establishment
// default policy lives in establishments.settings.default_policy; rows of
// booking_policies are scoped overrides of a season.

// BookingPolicyFilter narrows ListBookingPolicies.
type BookingPolicyFilter struct {
	SeasonID string
}

// InsertBookingPolicy inserts a booking policy.
func InsertBookingPolicy(ctx context.Context, db bun.IDB, es EstablishmentScope, p *BookingPolicy) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert booking policy", err)
}

// GetBookingPolicy loads a booking policy in scope.
func GetBookingPolicy(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*BookingPolicy, error) {
	p := new(BookingPolicy)
	if err := pricingGetScoped(ctx, db, es, p, "bp", id, "get booking policy"); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateBookingPolicy writes p if the stored version is still oldVersion.
func UpdateBookingPolicy(ctx context.Context, db bun.IDB, es EstablishmentScope, p *BookingPolicy, oldVersion int64) (bool, error) {
	return pricingUpdateVersioned(ctx, db, es, p, oldVersion, "update booking policy")
}

// DeleteBookingPolicy deletes a booking policy in scope.
func DeleteBookingPolicy(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return pricingDeleteScoped(ctx, db, es, (*BookingPolicy)(nil), id, "delete booking policy")
}

// ListBookingPolicies pages booking policies ordered by id.
func ListBookingPolicies(ctx context.Context, db bun.IDB, es EstablishmentScope, f BookingPolicyFilter, cursor string, limit int) (Page[BookingPolicy], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[BookingPolicy]{}, err
	}
	var items []BookingPolicy
	q := db.NewSelect().Model(&items).Where("bp.establishment_id = ?", es.ID())
	if f.SeasonID != "" {
		q = q.Where("bp.season_id = ?", f.SeasonID)
	}
	return pricingKeysetPage(ctx, q, &items, "bp", cursor, limit, func(r *BookingPolicy) string { return r.ID }, "list booking policies")
}

// ListSeasonBookingPolicies returns every policy of a season (active and not)
// ordered by priority DESC, id.
func ListSeasonBookingPolicies(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) ([]BookingPolicy, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []BookingPolicy{}
	err := db.NewSelect().Model(&items).
		Where("bp.establishment_id = ?", es.ID()).Where("bp.season_id = ?", seasonID).
		OrderExpr("bp.priority DESC, bp.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list season booking policies: %w", err)
	}
	return items, nil
}
