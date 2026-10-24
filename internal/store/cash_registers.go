package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Repository of the cash registers (migration 00009, ADR 0019). Owned by
// internal/domain/cash.

// ListCashRegisters returns the registers of es ordered by sort order and
// name; inactive ones only when includeInactive.
func ListCashRegisters(ctx context.Context, db bun.IDB, es EstablishmentScope, includeInactive bool) ([]CashRegister, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []CashRegister{}
	q := db.NewSelect().Model(&items).
		Where("cr.establishment_id = ?", es.ID()).
		OrderExpr("cr.is_default DESC, cr.sort_order ASC, cr.name ASC, cr.id ASC")
	if !includeInactive {
		q = q.Where("cr.active = ?", true)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list cash registers: %w", err)
	}
	return items, nil
}

// GetCashRegister returns a register of es by id.
func GetCashRegister(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*CashRegister, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	r := new(CashRegister)
	err := db.NewSelect().Model(r).
		Where("cr.id = ?", id).
		Where("cr.establishment_id = ?", es.ID()).
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get cash register", err)
	}
	return r, nil
}

// GetCashRegisterByCode returns a register of es by its code.
func GetCashRegisterByCode(ctx context.Context, db bun.IDB, es EstablishmentScope, code string) (*CashRegister, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	r := new(CashRegister)
	err := db.NewSelect().Model(r).
		Where("cr.code = ?", code).
		Where("cr.establishment_id = ?", es.ID()).
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get cash register by code", err)
	}
	return r, nil
}

// InsertCashRegister inserts a register. A duplicate code (or a second
// default register) is ErrConflict.
func InsertCashRegister(ctx context.Context, db bun.IDB, es EstablishmentScope, r *CashRegister) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	r.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(r).Exec(ctx)
	return wrapWrite("insert cash register", err)
}

// InsertCashRegisterIfAbsent inserts r unless a register with the same code
// (or a second default) already exists: used to create the default register
// lazily, safe under concurrency.
func InsertCashRegisterIfAbsent(ctx context.Context, db bun.IDB, es EstablishmentScope, r *CashRegister) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	r.EstablishmentID = es.ID()
	if _, err := db.NewInsert().Model(r).On("CONFLICT DO NOTHING").Exec(ctx); err != nil {
		return fmt.Errorf("store: insert cash register: %w", err)
	}
	return nil
}

// UpdateCashRegister writes name, active, sort order, version and
// updated_at of r when the stored row still has oldVersion. It reports
// whether the row was updated; the UPDATE also takes the row lock, so a
// concurrent session opening on the register waits (and sees the new state).
func UpdateCashRegister(ctx context.Context, db bun.IDB, es EstablishmentScope, r *CashRegister, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model(r).
		Column("name", "active", "sort_order", "version", "updated_at").
		Where("cr.id = ?", r.ID).
		Where("cr.establishment_id = ?", es.ID()).
		Where("cr.version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update cash register", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update cash register: %w", err)
	}
	return n == 1, nil
}

// LockActiveCashRegister takes the row lock of an active register for the
// rest of tx (no-op UPDATE) and reports false when the register does not
// exist or is inactive. Opening a session holds it, so deactivation and
// opening are serialised.
func LockActiveCashRegister(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().TableExpr("cash_registers").
		Set("version = version").
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("active = ?", true).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: lock cash register: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: lock cash register: %w", err)
	}
	return n == 1, nil
}

// DeleteCashRegister deletes a non-default register of es with version.
// Sessions reference registers with ON DELETE RESTRICT: callers check
// CountCashRegisterSessions first. It reports whether a row was deleted.
func DeleteCashRegister(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, version int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewDelete().TableExpr("cash_registers").
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("is_default = ?", false).
		Where("version = ?", version).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: delete cash register: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: delete cash register: %w", err)
	}
	return n == 1, nil
}

// CountCashRegisterSessions counts the sessions (open or closed) of a
// register.
func CountCashRegisterSessions(ctx context.Context, db bun.IDB, es EstablishmentScope, registerID string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	n, err := db.NewSelect().TableExpr("cash_sessions").
		Where("establishment_id = ?", es.ID()).
		Where("register_id = ?", registerID).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count register sessions: %w", err)
	}
	return int(n), nil
}

// OpenCashSessionsByRegister maps register id -> open session id for es.
func OpenCashSessionsByRegister(ctx context.Context, db bun.IDB, es EstablishmentScope) (map[string]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []struct {
		ID         string  `bun:"id"`
		RegisterID *string `bun:"register_id"`
	}
	err := db.NewSelect().TableExpr("cash_sessions").Column("id", "register_id").
		Where("establishment_id = ?", es.ID()).
		Where("closed_at IS NULL").
		Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: open sessions by register: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.RegisterID != nil {
			out[*r.RegisterID] = r.ID
		}
	}
	return out, nil
}

// CashSessionIDsOfRegister returns the ids of every session of a register
// (used to narrow the corrispettivi to one register).
func CashSessionIDsOfRegister(ctx context.Context, db bun.IDB, es EstablishmentScope, registerID string) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	ids := []string{}
	err := db.NewSelect().TableExpr("cash_sessions").Column("id").
		Where("establishment_id = ?", es.ID()).
		Where("register_id = ?", registerID).
		Scan(ctx, &ids)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: register sessions: %w", err)
	}
	return ids, nil
}
