package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// ---- sessions ----------------------------------------------------------------

// InsertSession inserts a session.
func InsertSession(ctx context.Context, db bun.IDB, s *Session) error {
	_, err := db.NewInsert().Model(s).Exec(ctx)
	return wrapWrite("insert session", err)
}

// GetSessionByTokenHash loads a non-expired session by token hash.
func GetSessionByTokenHash(ctx context.Context, db bun.IDB, tokenHash string, now time.Time) (*Session, error) {
	s := new(Session)
	err := db.NewSelect().Model(s).Where("token_hash = ?", tokenHash).Where("expires_at > ?", now).Scan(ctx)
	return s, wrapRead("get session", err)
}

// TouchSession extends a session's expiry and last-seen time.
func TouchSession(ctx context.Context, db bun.IDB, id string, lastSeen, expiresAt time.Time) error {
	_, err := db.NewUpdate().Model((*Session)(nil)).
		Set("last_seen_at = ?", lastSeen).Set("expires_at = ?", expiresAt).Set("updated_at = ?", lastSeen).
		Where("id = ?", id).Exec(ctx)
	return wrapWrite("touch session", err)
}

// DeleteSessionByTokenHash deletes a session (logout, rotation).
func DeleteSessionByTokenHash(ctx context.Context, db bun.IDB, tokenHash string) error {
	_, err := db.NewDelete().Model((*Session)(nil)).Where("token_hash = ?", tokenHash).Exec(ctx)
	return wrapWrite("delete session", err)
}

// DeleteUserSessions deletes every session of a user (password change, disable).
func DeleteUserSessions(ctx context.Context, db bun.IDB, userID string) error {
	_, err := db.NewDelete().Model((*Session)(nil)).Where("user_id = ?", userID).Exec(ctx)
	return wrapWrite("delete user sessions", err)
}

// DeleteExpiredSessions removes expired sessions and returns how many.
func DeleteExpiredSessions(ctx context.Context, db bun.IDB, now time.Time) (int64, error) {
	res, err := db.NewDelete().Model((*Session)(nil)).Where("expires_at <= ?", now).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("delete expired sessions", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ---- API keys ---------------------------------------------------------------

// InsertAPIKey inserts an API key in the establishment of es.
func InsertAPIKey(ctx context.Context, db bun.IDB, es EstablishmentScope, k *APIKey) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	k.EstablishmentID = es.ID()
	if k.Scopes == nil {
		k.Scopes = []string{}
	}
	_, err := db.NewInsert().Model(k).Exec(ctx)
	return wrapWrite("insert api key", err)
}

// GetActiveAPIKeyByPrefix loads a non-revoked API key by its public prefix.
// Authentication is a system operation: the key itself proves the scope.
func GetActiveAPIKeyByPrefix(ctx context.Context, db bun.IDB, prefix string) (*APIKey, error) {
	k := new(APIKey)
	err := db.NewSelect().Model(k).Where("prefix = ?", prefix).Where("revoked_at IS NULL").Scan(ctx)
	return k, wrapRead("get api key", err)
}

// TouchAPIKey records the last use of an API key.
func TouchAPIKey(ctx context.Context, db bun.IDB, id string, at time.Time) error {
	_, err := db.NewUpdate().Model((*APIKey)(nil)).Set("last_used_at = ?", at).Where("id = ?", id).Exec(ctx)
	return wrapWrite("touch api key", err)
}

// RevokeAPIKey revokes an API key of the establishment of es.
func RevokeAPIKey(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewUpdate().Model((*APIKey)(nil)).
		Set("revoked_at = ?", at).Set("updated_at = ?", at).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("revoked_at IS NULL").Exec(ctx)
	if err != nil {
		return wrapWrite("revoke api key", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAPIKeys lists the API keys of an establishment.
func ListAPIKeys(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]APIKey, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var ks []APIKey
	err := db.NewSelect().Model(&ks).Where("establishment_id = ?", es.ID()).OrderExpr("id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list api keys: %w", err)
	}
	return ks, nil
}

// ---- audit log ---------------------------------------------------------------

// InsertAuditEntry appends an audit entry. There is deliberately no update or
// delete function; the database rejects both with a trigger.
func InsertAuditEntry(ctx context.Context, db bun.IDB, e *AuditEntry) error {
	_, err := db.NewInsert().Model(e).Exec(ctx)
	return wrapWrite("insert audit entry", err)
}

// ListAuditEntries lists the audit entries of an establishment, newest first.
func ListAuditEntries(ctx context.Context, db bun.IDB, es EstablishmentScope, cursor string, limit int) (Page[AuditEntry], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[AuditEntry]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[AuditEntry]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []AuditEntry
	q := db.NewSelect().Model(&items).Where("establishment_id = ?", es.ID()).OrderExpr("id DESC").Limit(int64(limit + 1))
	if before != "" {
		q = q.Where("id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[AuditEntry]{}, fmt.Errorf("store: list audit: %w", err)
	}
	page := Page[AuditEntry]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	return page, nil
}
