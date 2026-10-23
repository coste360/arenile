package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository of the cash desk (spec 14.1). Owned by internal/domain/cash.

// CashSessionFilter narrows ListCashSessions; zero fields do not filter.
type CashSessionFilter struct {
	// Open: nil = all, true = open only, false = closed only.
	Open     *bool
	Register string
	// OpenedFrom / OpenedBefore bound opened_at: [OpenedFrom, OpenedBefore).
	OpenedFrom   *time.Time
	OpenedBefore *time.Time
}

// InsertCashSession inserts a session. A second open session on the same
// register violates cash_sessions_open_uidx and is ErrConflict.
func InsertCashSession(ctx context.Context, db bun.IDB, es EstablishmentScope, s *CashSession) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	s.EstablishmentID = es.ID()
	if s.ExpectedTotals == nil {
		s.ExpectedTotals = MethodTotals{}
	}
	_, err := db.NewInsert().Model(s).Exec(ctx)
	return wrapWrite("insert cash session", err)
}

// GetCashSession returns a session of es.
func GetCashSession(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*CashSession, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(CashSession)
	err := db.NewSelect().Model(s).
		Where("cs.id = ?", id).
		Where("cs.establishment_id = ?", es.ID()).
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get cash session", err)
	}
	return s, nil
}

// GetOpenCashSession returns the open session of register, or ErrNotFound.
func GetOpenCashSession(ctx context.Context, db bun.IDB, es EstablishmentScope, register string) (*CashSession, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(CashSession)
	err := db.NewSelect().Model(s).
		Where("cs.establishment_id = ?", es.ID()).
		Where("cs.register = ?", register).
		Where("cs.closed_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get open cash session", err)
	}
	return s, nil
}

// LockOpenCashSession takes the row lock of an open session for the rest of
// tx (no-op UPDATE: a row lock on PostgreSQL, the write lock on SQLite), so
// that a concurrent close waits for the payment or movement being recorded.
// It reports false when the session does not exist or is closed.
func LockOpenCashSession(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().TableExpr("cash_sessions").
		Set("version = version").
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("closed_at IS NULL").
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: lock cash session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: lock cash session: %w", err)
	}
	return n == 1, nil
}

// CloseCashSession writes the closing columns of s (ClosedAt, ClosedBy,
// ExpectedTotals, CountedTotals, Difference, Notes, Version = oldVersion+1)
// when the stored row is still open with oldVersion. It reports whether the
// row was updated.
func CloseCashSession(ctx context.Context, db bun.IDB, es EstablishmentScope, s *CashSession, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model(s).
		Column("closed_at", "closed_by", "expected_totals", "counted_totals", "difference", "notes", "version", "updated_at").
		Where("cs.id = ?", s.ID).
		Where("cs.establishment_id = ?", es.ID()).
		Where("cs.closed_at IS NULL").
		Where("cs.version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: close cash session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: close cash session: %w", err)
	}
	return n == 1, nil
}

// ListCashSessions lists the sessions of es matching f, newest first, with
// keyset pagination on id (UUIDv7).
func ListCashSessions(ctx context.Context, db bun.IDB, es EstablishmentScope, f CashSessionFilter, cursor string, limit int) (Page[CashSession], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[CashSession]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[CashSession]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []CashSession
	q := db.NewSelect().Model(&items).
		Where("cs.establishment_id = ?", es.ID()).
		OrderExpr("cs.id DESC").
		Limit(int64(limit + 1))
	if f.Open != nil {
		if *f.Open {
			q = q.Where("cs.closed_at IS NULL")
		} else {
			q = q.Where("cs.closed_at IS NOT NULL")
		}
	}
	if f.Register != "" {
		q = q.Where("cs.register = ?", f.Register)
	}
	if f.OpenedFrom != nil {
		q = q.Where("cs.opened_at >= ?", f.OpenedFrom.UTC())
	}
	if f.OpenedBefore != nil {
		q = q.Where("cs.opened_at < ?", f.OpenedBefore.UTC())
	}
	if before != "" {
		q = q.Where("cs.id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[CashSession]{}, fmt.Errorf("store: list cash sessions: %w", err)
	}
	page := Page[CashSession]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	if page.Items == nil {
		page.Items = []CashSession{}
	}
	return page, nil
}

// InsertCashMovement inserts a cash movement.
func InsertCashMovement(ctx context.Context, db bun.IDB, es EstablishmentScope, m *CashMovement) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	m.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(m).Exec(ctx)
	return wrapWrite("insert cash movement", err)
}

// ListCashMovements returns the movements of a session, oldest first.
func ListCashMovements(ctx context.Context, db bun.IDB, es EstablishmentScope, sessionID string) ([]CashMovement, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []CashMovement{}
	err := db.NewSelect().Model(&items).
		Where("cm.establishment_id = ?", es.ID()).
		Where("cm.cash_session_id = ?", sessionID).
		OrderExpr("cm.id ASC").
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list cash movements: %w", err)
	}
	return items, nil
}
