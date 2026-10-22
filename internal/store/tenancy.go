package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

func wrapWrite(op string, err error) error {
	if err == nil {
		return nil
	}
	if IsUniqueViolation(err) {
		return fmt.Errorf("store: %s: %w", op, ErrConflict)
	}
	return fmt.Errorf("store: %s: %w", op, err)
}

func wrapRead(op string, err error) error {
	if err == nil {
		return nil
	}
	if isNoRows(err) {
		return ErrNotFound
	}
	return fmt.Errorf("store: %s: %w", op, err)
}

// ---- organizations ----------------------------------------------------------

// InsertOrganization inserts an organization.
func InsertOrganization(ctx context.Context, db bun.IDB, o *Organization) error {
	_, err := db.NewInsert().Model(o).Exec(ctx)
	return wrapWrite("insert organization", err)
}

// GetOrganization loads an organization by id.
func GetOrganization(ctx context.Context, db bun.IDB, id string) (*Organization, error) {
	o := new(Organization)
	err := db.NewSelect().Model(o).Where("id = ?", id).Scan(ctx)
	return o, wrapRead("get organization", err)
}

// FindOrganizationByName loads the oldest organization with the given name.
func FindOrganizationByName(ctx context.Context, db bun.IDB, name string) (*Organization, error) {
	o := new(Organization)
	err := db.NewSelect().Model(o).Where("name = ?", name).OrderExpr("id ASC").Limit(1).Scan(ctx)
	return o, wrapRead("find organization", err)
}

// ---- establishments ---------------------------------------------------------

// InsertEstablishment inserts an establishment.
func InsertEstablishment(ctx context.Context, db bun.IDB, e *Establishment) error {
	normalizeEstablishment(e)
	_, err := db.NewInsert().Model(e).Exec(ctx)
	return wrapWrite("insert establishment", err)
}

// UpdateEstablishment updates every column of an establishment in scope.
func UpdateEstablishment(ctx context.Context, db bun.IDB, es EstablishmentScope, e *Establishment) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if e.ID != es.ID() {
		return ErrNotFound
	}
	normalizeEstablishment(e)
	res, err := db.NewUpdate().Model(e).WherePK().Where("organization_id = ?", es.OrganizationID()).Exec(ctx)
	if err != nil {
		return wrapWrite("update establishment", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func normalizeEstablishment(e *Establishment) {
	if e.EnabledLocales == nil {
		e.EnabledLocales = []string{}
	}
	if e.AllowedEmbedOrigins == nil {
		e.AllowedEmbedOrigins = []string{}
	}
	if e.Settings == nil {
		e.Settings = JSON("{}")
	}
	if e.Version == 0 {
		e.Version = 1
	}
}

// GetEstablishment loads the establishment proven by es.
func GetEstablishment(ctx context.Context, db bun.IDB, es EstablishmentScope) (*Establishment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	e := new(Establishment)
	err := db.NewSelect().Model(e).Where("id = ?", es.ID()).Scan(ctx)
	return e, wrapRead("get establishment", err)
}

// GetEstablishmentBySlug loads an establishment by its public slug. It is
// meant for the public API, which only exposes public fields.
func GetEstablishmentBySlug(ctx context.Context, db bun.IDB, slug string) (*Establishment, error) {
	e := new(Establishment)
	err := db.NewSelect().Model(e).Where("slug = ?", strings.ToLower(slug)).Scan(ctx)
	return e, wrapRead("get establishment by slug", err)
}

// Page is a cursor-paginated result.
type Page[T any] struct {
	Items      []T
	NextCursor string
}

// EncodeCursor makes an opaque cursor from the last id of a page.
func EncodeCursor(lastID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("id:" + lastID))
}

// ErrInvalidCursor is returned for malformed cursors.
var ErrInvalidCursor = errors.New("store: invalid cursor")

// DecodeCursor reverses EncodeCursor.
func DecodeCursor(c string) (string, error) {
	if c == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil || !strings.HasPrefix(string(b), "id:") || len(b) < 4 || len(b) > 64 {
		return "", ErrInvalidCursor
	}
	return string(b[3:]), nil
}

// ListEstablishments lists the non-archived establishments in scope, ordered
// by id (UUIDv7, i.e. creation order), with keyset pagination.
func ListEstablishments(ctx context.Context, db bun.IDB, scope Scope, cursor string, limit int) (Page[Establishment], error) {
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[Establishment]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []Establishment
	q := db.NewSelect().Model(&items).
		Where("e.archived_at IS NULL").
		OrderExpr("e.id ASC").
		Limit(int64(limit + 1))
	q = scope.applyToEstablishments(q)
	if after != "" {
		q = q.Where("e.id > ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[Establishment]{}, fmt.Errorf("store: list establishments: %w", err)
	}
	page := Page[Establishment]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	return page, nil
}

// ---- users and memberships --------------------------------------------------

// NormalizeEmail lowercases and trims an email address.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// InsertUser inserts a user (email is normalized).
func InsertUser(ctx context.Context, db bun.IDB, u *User) error {
	u.Email = NormalizeEmail(u.Email)
	_, err := db.NewInsert().Model(u).Exec(ctx)
	return wrapWrite("insert user", err)
}

// HasUsers reports whether at least one user exists (first-owner bootstrap).
func HasUsers(ctx context.Context, db bun.IDB) (bool, error) {
	ok, err := db.NewSelect().Model((*User)(nil)).Exists(ctx)
	return ok, wrapRead("has users", err)
}

// GetUser loads a user by id.
func GetUser(ctx context.Context, db bun.IDB, id string) (*User, error) {
	u := new(User)
	err := db.NewSelect().Model(u).Where("id = ?", id).Scan(ctx)
	return u, wrapRead("get user", err)
}

// GetUserByEmail loads a user by (normalized) email.
func GetUserByEmail(ctx context.Context, db bun.IDB, email string) (*User, error) {
	u := new(User)
	err := db.NewSelect().Model(u).Where("email = ?", NormalizeEmail(email)).Scan(ctx)
	return u, wrapRead("get user by email", err)
}

// UpdateUserLogin records a successful login.
func UpdateUserLogin(ctx context.Context, db bun.IDB, userID string, at time.Time) error {
	_, err := db.NewUpdate().Model((*User)(nil)).
		Set("last_login_at = ?", at).Set("updated_at = ?", at).
		Where("id = ?", userID).Exec(ctx)
	return wrapWrite("update user login", err)
}

// UpdateUserPasswordHash replaces the password hash (e.g. rehash on login).
func UpdateUserPasswordHash(ctx context.Context, db bun.IDB, userID, hash string, at time.Time) error {
	_, err := db.NewUpdate().Model((*User)(nil)).
		Set("password_hash = ?", hash).Set("updated_at = ?", at).
		Where("id = ?", userID).Exec(ctx)
	return wrapWrite("update user password", err)
}

// SetUserTOTP stores an encrypted TOTP secret (pending confirmation when
// confirmedAt is nil).
func SetUserTOTP(ctx context.Context, db bun.IDB, userID string, encSecret *string, confirmedAt *time.Time, at time.Time) error {
	_, err := db.NewUpdate().Model((*User)(nil)).
		Set("totp_secret = ?", encSecret).
		Set("totp_confirmed_at = ?", confirmedAt).
		Set("totp_last_step = 0").
		Set("updated_at = ?", at).
		Where("id = ?", userID).Exec(ctx)
	return wrapWrite("set user totp", err)
}

// AdvanceUserTOTPStep atomically records the last accepted TOTP time step;
// it reports false if step was already used (replay).
func AdvanceUserTOTPStep(ctx context.Context, db bun.IDB, userID string, step int64, at time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*User)(nil)).
		Set("totp_last_step = ?", step).Set("updated_at = ?", at).
		Where("id = ?", userID).Where("totp_last_step < ?", step).Exec(ctx)
	if err != nil {
		return false, wrapWrite("advance totp step", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// InsertMembership inserts a membership.
func InsertMembership(ctx context.Context, db bun.IDB, m *Membership) error {
	_, err := db.NewInsert().Model(m).Exec(ctx)
	return wrapWrite("insert membership", err)
}

// ListUserMemberships returns all memberships of a user.
func ListUserMemberships(ctx context.Context, db bun.IDB, userID string) ([]Membership, error) {
	var ms []Membership
	err := db.NewSelect().Model(&ms).Where("user_id = ?", userID).OrderExpr("id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list memberships: %w", err)
	}
	return ms, nil
}
