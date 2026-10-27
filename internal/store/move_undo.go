package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Move undo log (spec 30.7, ADR 0028): written by the booking service in the
// transaction of a move, swap or group move; consumed by its undo within
// MoveUndoWindow. The availability itself is never checked here: an undo
// re-allocates through the inventory and the unique keys decide.

// InsertMoveUndo inserts an undo entry of es.
func InsertMoveUndo(ctx context.Context, db bun.IDB, es EstablishmentScope, e *MoveUndoEntry) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	e.EstablishmentID = es.ID()
	if e.BookingIDs == nil {
		e.BookingIDs = []string{}
	}
	if _, err := db.NewInsert().Model(e).Exec(ctx); err != nil {
		return wrapWrite("insert move undo", err)
	}
	return nil
}

// GetMoveUndo returns an undo entry of es (ErrNotFound if missing).
func GetMoveUndo(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*MoveUndoEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	e := new(MoveUndoEntry)
	err := db.NewSelect().Model(e).Where("mul.id = ?", id).Where("mul.establishment_id = ?", es.ID()).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get move undo: %w", err)
	}
	return e, nil
}

// LastMoveUndo returns the most recent entry of userID in es that is not
// undone and not expired at now (ErrNotFound if none).
func LastMoveUndo(ctx context.Context, db bun.IDB, es EstablishmentScope, userID string, now time.Time) (*MoveUndoEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	e := new(MoveUndoEntry)
	err := db.NewSelect().Model(e).
		Where("mul.establishment_id = ?", es.ID()).Where("mul.user_id = ?", userID).
		Where("mul.undone_at IS NULL").Where("mul.expires_at > ?", now.UTC()).
		OrderExpr("mul.created_at DESC, mul.id DESC").Limit(1).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: last move undo: %w", err)
	}
	return e, nil
}

// MarkMoveUndone sets undone_at on an entry still undoable at now (not
// undone, not expired) and reports whether it did: the conditional UPDATE
// lets one of two concurrent undos win.
func MarkMoveUndone(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*MoveUndoEntry)(nil)).Set("undone_at = ?", now.UTC()).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Where("undone_at IS NULL").Where("expires_at > ?", now.UTC()).Exec(ctx)
	if err != nil {
		return false, wrapWrite("mark move undone", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ListActiveMoveUndos returns the entries of es still undoable at now (not
// undone, not expired: a few at most, the window is 5 minutes), newest first.
func ListActiveMoveUndos(ctx context.Context, db bun.IDB, es EstablishmentScope, now time.Time) ([]MoveUndoEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []MoveUndoEntry{}
	err := db.NewSelect().Model(&out).
		Where("mul.establishment_id = ?", es.ID()).Where("mul.undone_at IS NULL").Where("mul.expires_at > ?", now.UTC()).
		OrderExpr("mul.created_at DESC, mul.id DESC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: active move undos: %w", err)
	}
	return out, nil
}
