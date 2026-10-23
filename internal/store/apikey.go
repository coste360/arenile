package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Repository functions of the API key management from the panel (M5A, spec
// 16, 17). Insert, authentication lookup, touch and revoke stay in auth.go.

// GetAPIKey loads an API key of the establishment of es (revoked included).
func GetAPIKey(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*APIKey, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	k := new(APIKey)
	err := db.NewSelect().Model(k).Where("id = ?", id).Where("establishment_id = ?", es.ID()).Scan(ctx)
	return k, wrapRead("get api key", err)
}

// ListEstablishmentAPIKeys lists the API keys of the establishment of es,
// newest first; revoked keys only when includeRevoked.
func ListEstablishmentAPIKeys(ctx context.Context, db bun.IDB, es EstablishmentScope, includeRevoked bool) ([]APIKey, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var ks []APIKey
	q := db.NewSelect().Model(&ks).Where("establishment_id = ?", es.ID()).OrderExpr("id DESC")
	if !includeRevoked {
		q = q.Where("revoked_at IS NULL")
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list api keys: %w", err)
	}
	return ks, nil
}

// UpdateAPIKey writes the given columns of k (plus updated_at) for a
// non-revoked key of the establishment of es; ErrNotFound otherwise.
func UpdateAPIKey(ctx context.Context, db bun.IDB, es EstablishmentScope, k *APIKey, columns ...string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if k.Scopes == nil {
		k.Scopes = []string{}
	}
	res, err := db.NewUpdate().Model(k).Column(append(columns, "updated_at")...).
		WherePK().Where("establishment_id = ?", es.ID()).Where("revoked_at IS NULL").Exec(ctx)
	if err != nil {
		return wrapWrite("update api key", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserNames returns the display names of the given users, by id (missing
// users are absent from the map).
func UserNames(ctx context.Context, db bun.IDB, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   string `bun:"id"`
		Name string `bun:"name"`
	}
	err := db.NewSelect().Model((*User)(nil)).Column("id", "name").Where("id IN (?)", bun.List(ids)).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: user names: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}
