package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/money"
)

// Models of the cash desk (spec 14.1). Ownership: internal/domain/cash
// (repository in internal/store/cash.go).

// DefaultCashRegister is the register used when none is given.
const DefaultCashRegister = "main"

// MethodTotals are amounts per payment method (keys = Payment*Method values).
type MethodTotals map[string]money.Cents

// Sum is the total over every method.
func (t MethodTotals) Sum() money.Cents {
	var s money.Cents
	for _, v := range t {
		s += v
	}
	return s
}

// CashSession is one opening of a register. At most one session per
// (establishment, register) has ClosedAt nil (unique partial index).
// ExpectedTotals = opening float (cash) + session takings by method +/-
// cash movements; on close CountedTotals and Difference = counted - expected
// are stored (INV-25).
type CashSession struct {
	bun.BaseModel `bun:"table:cash_sessions,alias:cs"`

	ID              string `bun:"id,pk"`
	EstablishmentID string `bun:"establishment_id,notnull"`
	Register        string `bun:"register,notnull"`
	// RegisterID is the cash register (00009); NULL only for rows written
	// by code that predates registers (backfilled by the migration).
	RegisterID     *string      `bun:"register_id"`
	OpenedBy       *string      `bun:"opened_by"`
	ClosedBy       *string      `bun:"closed_by"`
	OpenedAt       time.Time    `bun:"opened_at,notnull"`
	ClosedAt       *time.Time   `bun:"closed_at"`
	OpeningFloat   money.Cents  `bun:"opening_float,notnull"`
	ExpectedTotals MethodTotals `bun:"expected_totals,type:json,notnull"`
	CountedTotals  MethodTotals `bun:"counted_totals,type:json,nullzero"`
	Difference     *money.Cents `bun:"difference"`
	Notes          string       `bun:"notes,notnull"`
	Version        int64        `bun:"version,notnull"`
	CreatedAt      time.Time    `bun:"created_at,notnull"`
	UpdatedAt      time.Time    `bun:"updated_at,notnull"`
}

// Cash movement kinds.
const (
	CashMovementIn  = "in"
	CashMovementOut = "out"
)

// CashMovement is a cash deposit (in) or withdrawal (out) of a session.
type CashMovement struct {
	bun.BaseModel `bun:"table:cash_movements,alias:cm"`

	ID              string      `bun:"id,pk"`
	EstablishmentID string      `bun:"establishment_id,notnull"`
	CashSessionID   string      `bun:"cash_session_id,notnull"`
	Kind            string      `bun:"kind,notnull"`
	Amount          money.Cents `bun:"amount,notnull"`
	Reason          string      `bun:"reason,notnull"`
	CreatedByUserID *string     `bun:"created_by_user_id"`
	CreatedAt       time.Time   `bun:"created_at,notnull"`
}
