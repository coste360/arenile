package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of panel holds (spec 7.4, ADR 0010 §3). Owner:
// internal/domain/inventory.

// InsertHold inserts a hold of es.
func InsertHold(ctx context.Context, db bun.IDB, es EstablishmentScope, h *Hold) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	h.EstablishmentID = es.ID()
	if h.Items == nil {
		h.Items = []HoldItem{}
	}
	_, err := db.NewInsert().Model(h).Exec(ctx)
	return wrapWrite("insert hold", err)
}

// GetHold loads a hold of es.
func GetHold(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Hold, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	h := new(Hold)
	err := db.NewSelect().Model(h).Where("ho.id = ?", id).Where("ho.establishment_id = ?", es.ID()).Scan(ctx)
	return h, wrapRead("get hold", err)
}

// CloseHold marks an open hold released (convertedBookingID empty) or
// converted, only if it is still open; it reports whether it changed, so
// that concurrent closers (user, sweeper, conversion) act exactly once.
func CloseHold(ctx context.Context, db bun.IDB, es EstablishmentScope, id, convertedBookingID string, at time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	q := db.NewUpdate().TableExpr("holds").Set("updated_at = ?", at).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Where("released_at IS NULL").Where("converted_booking_id IS NULL")
	if convertedBookingID != "" {
		q = q.Set("converted_booking_id = ?", convertedBookingID)
	} else {
		q = q.Set("released_at = ?", at)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return false, wrapWrite("close hold", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ExpiredHold identifies an open hold past its expiry (sweeper input).
type ExpiredHold struct {
	ID              string `bun:"id"`
	EstablishmentID string `bun:"establishment_id"`
}

// ListExpiredOpenHolds returns up to limit open holds of every
// establishment with expires_at <= now, oldest first. System use only (the
// sweeper job), hence no EstablishmentScope.
func ListExpiredOpenHolds(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]ExpiredHold, error) {
	var out []ExpiredHold
	err := db.NewSelect().TableExpr("holds").ColumnExpr("id, establishment_id").
		Where("released_at IS NULL").Where("converted_booking_id IS NULL").
		Where("expires_at <= ?", now.UTC()).
		OrderExpr("expires_at ASC, id ASC").Limit(int64(limit)).Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list expired holds: %w", err)
	}
	return out, nil
}
