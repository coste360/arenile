package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of import_jobs (CSV imports, ADR 0019 feature F).
// Owner: internal/domain/importer.

// InsertImportJob inserts j into es.
func InsertImportJob(ctx context.Context, db bun.IDB, es EstablishmentScope, j *ImportJob) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	j.EstablishmentID = es.ID()
	if j.Options == nil {
		j.Options = map[string]any{}
	}
	if _, err := db.NewInsert().Model(j).Exec(ctx); err != nil {
		return fmt.Errorf("store: insert import job: %w", err)
	}
	return nil
}

// GetImportJob returns the import job id of es (ErrNotFound otherwise).
func GetImportJob(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*ImportJob, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	j := new(ImportJob)
	err := db.NewSelect().Model(j).Where("ij.establishment_id = ?", es.ID()).Where("ij.id = ?", id).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get import job: %w", err)
	}
	return j, nil
}

// ListImportJobs returns the import jobs of es, newest first (id keyset:
// ids are UUIDv7, so they sort by creation time). Content is never loaded.
func ListImportJobs(ctx context.Context, db bun.IDB, es EstablishmentScope, cursor string, limit int) (Page[ImportJob], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[ImportJob]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[ImportJob]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []ImportJob
	q := db.NewSelect().Model(&items).ExcludeColumn("content").Where("ij.establishment_id = ?", es.ID())
	if after != "" {
		q = q.Where("ij.id < ?", after)
	}
	if err := q.OrderExpr("ij.id DESC").Limit(int64(limit + 1)).Scan(ctx); err != nil && !isNoRows(err) {
		return Page[ImportJob]{}, fmt.Errorf("store: list import jobs: %w", err)
	}
	page := Page[ImportJob]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	if page.Items == nil {
		page.Items = []ImportJob{}
	}
	return page, nil
}

// UpdateImportJob saves j when its stored version is still oldVersion
// (optimistic update); it reports false when another write won. A unique
// violation (same file already applied) is returned as ErrConflict.
func UpdateImportJob(ctx context.Context, db bun.IDB, es EstablishmentScope, j *ImportJob, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	if j.Options == nil {
		j.Options = map[string]any{}
	}
	res, err := db.NewUpdate().Model(j).
		Column("status", "content", "options", "report", "applied_at", "version", "updated_at").
		Where("ij.establishment_id = ?", es.ID()).Where("ij.id = ?", j.ID).Where("ij.version = ?", oldVersion).
		Exec(ctx)
	if IsUniqueViolation(err) {
		return false, ErrConflict
	}
	if err != nil {
		return false, fmt.Errorf("store: update import job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update import job: %w", err)
	}
	return n == 1, nil
}

// AppliedImportJobID returns the id of the applied import of es with the
// same kind and file hash, "" when none.
func AppliedImportJobID(ctx context.Context, db bun.IDB, es EstablishmentScope, kind, sha string) (string, error) {
	if err := es.mustBeValid(); err != nil {
		return "", err
	}
	var id string
	err := db.NewSelect().Model((*ImportJob)(nil)).Column("ij.id").
		Where("ij.establishment_id = ?", es.ID()).Where("ij.kind = ?", kind).
		Where("ij.sha256 = ?", sha).Where("ij.status = ?", ImportStatusApplied).
		Limit(1).Scan(ctx, &id)
	if isNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: applied import job: %w", err)
	}
	return id, nil
}

// ClearExpiredImportContent removes the uploaded content (personal data) of
// every import job created before cutoff, across establishments (retention
// job): jobs not applied yet become discarded. It returns the rows changed.
func ClearExpiredImportContent(ctx context.Context, db bun.IDB, cutoff, now time.Time) (int64, error) {
	res, err := db.NewUpdate().Model((*ImportJob)(nil)).
		Set("content = NULL").
		Set("status = CASE WHEN status IN (?) THEN ? ELSE status END",
			bun.List([]string{ImportStatusUploaded, ImportStatusValidated, ImportStatusFailed, ImportStatusApplying}), ImportStatusDiscarded).
		Set("version = version + 1").
		Set("updated_at = ?", now).
		Where("content IS NOT NULL").Where("created_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: clear import content: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: clear import content: %w", err)
	}
	return n, nil
}

// ImportActiveSpots returns the active spots of es (id, label, zone),
// used to resolve the spot labels of a season pass import.
func ImportActiveSpots(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []Spot
	err := db.NewSelect().Model(&out).Column("sp.id", "sp.label", "sp.zone_id", "sp.map_id", "sp.active").
		Where("sp.establishment_id = ?", es.ID()).Where("sp.active = ?", true).
		OrderExpr("sp.label ASC, sp.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: import spots: %w", err)
	}
	return out, nil
}

// ImportActivePacks returns the active packs of es (pack codes and names
// of a season pass import).
func ImportActivePacks(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Pack, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []Pack
	err := db.NewSelect().Model(&out).
		Where("pa.establishment_id = ?", es.ID()).Where("pa.active = ?", true).
		OrderExpr("pa.sort_order ASC, pa.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: import packs: %w", err)
	}
	return out, nil
}

// ImportSeasonPassExists reports whether the customer already holds a
// season pass of the season on the spot that is not cancelled (an imported
// row matching it is skipped: idempotent import).
func ImportSeasonPassExists(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID, seasonID, spotID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().TableExpr("bookings AS bk").
		Join("JOIN booking_lines AS bl ON bl.booking_id = bk.id").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.is_season_pass = ?", true).
		Where("bk.customer_id = ?", customerID).Where("bk.season_id = ?", seasonID).
		Where("bl.spot_id = ?", spotID).
		Where("bk.status NOT IN (?)", bun.List([]string{"cancelled", "expired", "rejected", "no_show"})).
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: import season pass exists: %w", err)
	}
	return n > 0, nil
}
