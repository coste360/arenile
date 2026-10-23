package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository of fiscal documents (spec 14.2). Owned by internal/domain/fiscal.

// FiscalDocumentFilter narrows ListFiscalDocuments; zero fields do not filter.
type FiscalDocumentFilter struct {
	From   *calendar.Date
	To     *calendar.Date
	Status string
}

// LockFiscalSequence serializes the allocation of the daily sequence numbers
// of es for the rest of tx: a transaction-scoped advisory lock on
// PostgreSQL; on SQLite write transactions are already serialized
// (BEGIN IMMEDIATE), so it is a no-op there.
func LockFiscalSequence(ctx context.Context, db bun.IDB, es EstablishmentScope, day calendar.Date) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if db.Dialect().Name() != dialect.PG {
		return nil
	}
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(?))", "fiscal:"+es.ID()+":"+day.String()); err != nil {
		return fmt.Errorf("store: lock fiscal sequence: %w", err)
	}
	return nil
}

// CountFiscalDocumentsOn counts the documents of es on business date day
// (the next daily sequence number is the count + 1; call
// LockFiscalSequence first).
func CountFiscalDocumentsOn(ctx context.Context, db bun.IDB, es EstablishmentScope, day calendar.Date) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	n, err := db.NewSelect().TableExpr("fiscal_documents AS fd").
		Where("fd.establishment_id = ?", es.ID()).
		Where("fd.business_date = ?", day).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count fiscal documents: %w", err)
	}
	return int(n), nil
}

// InsertFiscalDocument inserts a fiscal document.
func InsertFiscalDocument(ctx context.Context, db bun.IDB, es EstablishmentScope, d *FiscalDocument) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	d.EstablishmentID = es.ID()
	if d.Payload == nil {
		d.Payload = JSON("{}")
	}
	_, err := db.NewInsert().Model(d).Exec(ctx)
	return wrapWrite("insert fiscal document", err)
}

// UpdateFiscalDocumentOutcome stores the driver outcome (status, number,
// response, error) of a pending document. It reports false when the
// document is no longer pending (issued by a concurrent run).
func UpdateFiscalDocumentOutcome(ctx context.Context, db bun.IDB, es EstablishmentScope, d *FiscalDocument) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model(d).
		Column("status", "document_number", "response", "error", "updated_at").
		Where("fd.id = ?", d.ID).
		Where("fd.establishment_id = ?", es.ID()).
		Where("fd.status = ?", FiscalPending).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: update fiscal document: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update fiscal document: %w", err)
	}
	return n == 1, nil
}

// ListFiscalDocuments lists the documents of es, newest first, with keyset
// pagination on id.
func ListFiscalDocuments(ctx context.Context, db bun.IDB, es EstablishmentScope, f FiscalDocumentFilter, cursor string, limit int) (Page[FiscalDocument], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[FiscalDocument]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[FiscalDocument]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []FiscalDocument
	q := db.NewSelect().Model(&items).
		Where("fd.establishment_id = ?", es.ID()).
		OrderExpr("fd.id DESC").
		Limit(int64(limit + 1))
	if f.From != nil {
		q = q.Where("fd.business_date >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("fd.business_date <= ?", *f.To)
	}
	if f.Status != "" {
		q = q.Where("fd.status = ?", f.Status)
	}
	if before != "" {
		q = q.Where("fd.id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[FiscalDocument]{}, fmt.Errorf("store: list fiscal documents: %w", err)
	}
	page := Page[FiscalDocument]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	if page.Items == nil {
		page.Items = []FiscalDocument{}
	}
	return page, nil
}

// ListPendingFiscalDocuments returns up to limit pending documents of every
// establishment, oldest first. System use only (the issuing job), which
// builds a SystemEstablishmentScope per document.
func ListPendingFiscalDocuments(ctx context.Context, db bun.IDB, limit int) ([]FiscalDocument, error) {
	items := []FiscalDocument{}
	err := db.NewSelect().Model(&items).
		Where("fd.status = ?", FiscalPending).
		OrderExpr("fd.id ASC").
		Limit(int64(limit)).
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list pending fiscal documents: %w", err)
	}
	return items, nil
}
