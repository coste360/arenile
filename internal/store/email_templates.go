package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository of the editable customer email texts (email_template_overrides,
// migration 00012, ADR 0022) and of the reminder candidates read by the
// reminder job of internal/domain/notify. Every function is scoped by
// establishment.

// EmailTemplateOverride is the set of texts an establishment changed for one
// customer email kind and locale. Absent row = built-in texts.
type EmailTemplateOverride struct {
	bun.BaseModel `bun:"table:email_template_overrides,alias:eto"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	Kind            string    `bun:"kind,notnull"`
	Locale          string    `bun:"locale,notnull"`
	Subject         string    `bun:"subject,notnull"`
	Opening         string    `bun:"opening,notnull"`
	Closing         string    `bun:"closing,notnull"`
	Signature       string    `bun:"signature,notnull"`
	Version         int64     `bun:"version,notnull"`
	UpdatedByUserID *string   `bun:"updated_by_user_id"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// ListEmailTemplateOverrides returns the overrides of es.
func ListEmailTemplateOverrides(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]EmailTemplateOverride, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []EmailTemplateOverride
	err := db.NewSelect().Model(&out).Where("eto.establishment_id = ?", es.ID()).
		OrderExpr("eto.kind ASC, eto.locale ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list email template overrides: %w", err)
	}
	return out, nil
}

// GetEmailTemplateOverride returns the override of es for kind and locale
// (ErrNotFound when the built-in texts apply).
func GetEmailTemplateOverride(ctx context.Context, db bun.IDB, es EstablishmentScope, kind, locale string) (*EmailTemplateOverride, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	o := new(EmailTemplateOverride)
	err := db.NewSelect().Model(o).Where("eto.establishment_id = ?", es.ID()).
		Where("eto.kind = ?", kind).Where("eto.locale = ?", locale).Scan(ctx)
	return o, wrapRead("get email template override", err)
}

// InsertEmailTemplateOverride inserts the first override of a kind and
// locale; a concurrent insert of the same key fails on the unique index
// (ErrConflict).
func InsertEmailTemplateOverride(ctx context.Context, db bun.IDB, es EstablishmentScope, o *EmailTemplateOverride) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	o.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(o).Exec(ctx)
	return wrapWrite("insert email template override", err)
}

// UpdateEmailTemplateOverride saves o when the stored version is still
// prev (conditional update) and reports whether a row changed.
func UpdateEmailTemplateOverride(ctx context.Context, db bun.IDB, es EstablishmentScope, o *EmailTemplateOverride, prev int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model(o).
		Column("subject", "opening", "closing", "signature", "version", "updated_by_user_id", "updated_at").
		Where("establishment_id = ?", es.ID()).Where("id = ?", o.ID).Where("version = ?", prev).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update email template override", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteEmailTemplateOverride deletes the override of es with id when its
// version is still version and reports whether a row was deleted.
func DeleteEmailTemplateOverride(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, version int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewDelete().Model((*EmailTemplateOverride)(nil)).
		Where("establishment_id = ?", es.ID()).Where("id = ?", id).Where("version = ?", version).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("delete email template override", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ReminderCandidateFilter selects the bookings a reminder may concern.
type ReminderCandidateFilter struct {
	// StartsFrom and StartsTo bound starts_on (inclusive).
	StartsFrom, StartsTo calendar.Date
	// CreatedBefore excludes bookings created at or after this instant
	// (they just received their confirmation).
	CreatedBefore time.Time
	// WithBalance keeps only bookings with total_amount > paid_amount.
	WithBalance bool
	Limit       int
}

// ListReminderCandidates returns the confirmed, non-test bookings of es
// matching f, by start date. The caller re-checks every rule; this is only
// the indexed pre-selection (bookings_establishment_dates_idx).
func ListReminderCandidates(ctx context.Context, db bun.IDB, es EstablishmentScope, f ReminderCandidateFilter) ([]Booking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []Booking
	q := db.NewSelect().Model(&out).
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.status = ?", "confirmed").
		Where("bk.test = ?", false).
		Where("bk.starts_on >= ?", f.StartsFrom).Where("bk.starts_on <= ?", f.StartsTo).
		Where("bk.created_at < ?", f.CreatedBefore)
	if f.WithBalance {
		q = q.Where("bk.total_amount > bk.paid_amount")
	}
	if f.Limit > 0 {
		q = q.Limit(int64(f.Limit))
	}
	err := q.OrderExpr("bk.starts_on ASC, bk.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: reminder candidates: %w", err)
	}
	return out, nil
}
