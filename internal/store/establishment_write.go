package store

import (
	"context"

	"github.com/uptrace/bun"
)

// Repository functions of the establishment configuration use cases (M1:
// create, update, archive). See internal/domain/establishment.

// UpdateEstablishmentIfVersion replaces every column of the establishment in
// scope only when its stored version is still expected (optimistic
// concurrency). It reports false when no row matched: the establishment is
// gone or another writer bumped its version first.
func UpdateEstablishmentIfVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, e *Establishment, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	if e.ID != es.ID() {
		return false, ErrNotFound
	}
	normalizeEstablishment(e)
	res, err := db.NewUpdate().Model(e).WherePK().
		Where("organization_id = ?", es.OrganizationID()).
		Where("version = ?", expected).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update establishment", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// EstablishmentSlugTaken reports whether slug is used by an establishment
// other than exceptID (archived ones included: slugs are never reused while
// the row exists, so old public links do not point to a different lido).
func EstablishmentSlugTaken(ctx context.Context, db bun.IDB, slug, exceptID string) (bool, error) {
	q := db.NewSelect().Model((*Establishment)(nil)).Where("e.slug = ?", slug)
	if exceptID != "" {
		q = q.Where("e.id <> ?", exceptID)
	}
	ok, err := q.Exists(ctx)
	if err != nil {
		return false, wrapRead("establishment slug taken", err)
	}
	return ok, nil
}
