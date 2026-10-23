package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Repository of the customer credit balance (owner: internal/domain/credit,
// migration 00008). The ledger rows themselves are written with
// InsertCustomerCredit (customer.go); every insert goes with
// AddCustomerCreditBalance in the same transaction so that
// customer_credit_balances.balance = SUM(customer_credits.amount).

// LockCustomerCreditBalance makes sure the balance row of a customer exists
// (balance 0 on first use) and locks it for the rest of the transaction
// (no-op UPDATE: row lock on PostgreSQL, the single writer on SQLite). Every
// credit movement of one customer serialises on it, so the FIFO expiry
// computed before a debit cannot race another movement.
func LockCustomerCreditBalance(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	row := &CustomerCreditBalance{EstablishmentID: es.ID(), CustomerID: customerID, Balance: 0, UpdatedAt: now}
	if _, err := db.NewInsert().Model(row).On("CONFLICT (establishment_id, customer_id) DO NOTHING").Exec(ctx); err != nil {
		return wrapWrite("init credit balance", err)
	}
	res, err := db.NewUpdate().Model((*CustomerCreditBalance)(nil)).
		Set("balance = balance").
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: lock credit balance: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddCustomerCreditBalance adds amount to the balance of a customer with a
// conditional UPDATE (balance + amount >= 0) and reports whether it applied.
// false means the movement would make the balance negative: the caller
// rolls back (credit.ErrInsufficient). The row must exist
// (LockCustomerCreditBalance).
func AddCustomerCreditBalance(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, amount money.Cents, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*CustomerCreditBalance)(nil)).
		Set("balance = balance + ?", int64(amount)).
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).
		Where("balance + ? >= 0", int64(amount)).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: update credit balance: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// GetCustomerCreditBalance returns the stored balance of a customer (0 when
// no movement exists yet).
func GetCustomerCreditBalance(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) (money.Cents, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var row CustomerCreditBalance
	err := db.NewSelect().Model(&row).
		Where("ccb.establishment_id = ?", es.ID()).Where("ccb.customer_id = ?", customerID).
		Limit(1).Scan(ctx)
	if isNoRows(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: get credit balance: %w", err)
	}
	return row.Balance, nil
}

// LedgerCustomerCredits returns every movement of a customer in
// chronological order (created_at, id): the input of the FIFO allocation.
func LedgerCustomerCredits(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) ([]CustomerCredit, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []CustomerCredit
	err := db.NewSelect().Model(&items).
		Where("cc.establishment_id = ?", es.ID()).Where("cc.customer_id = ?", customerID).
		OrderExpr("cc.created_at ASC, cc.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer credit ledger: %w", err)
	}
	return items, nil
}

// PageCustomerCredits returns a page of the movements of a customer, newest
// first (keyset on id: UUIDv7, creation order).
func PageCustomerCredits(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID, cursor string, limit int) (Page[CustomerCredit], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[CustomerCredit]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[CustomerCredit]{}, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var items []CustomerCredit
	q := db.NewSelect().Model(&items).
		Where("cc.establishment_id = ?", es.ID()).Where("cc.customer_id = ?", customerID).
		OrderExpr("cc.id DESC").Limit(int64(limit + 1))
	if before != "" {
		q = q.Where("cc.id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[CustomerCredit]{}, fmt.Errorf("store: page customer credits: %w", err)
	}
	page := Page[CustomerCredit]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	if page.Items == nil {
		page.Items = []CustomerCredit{}
	}
	return page, nil
}

// CreditExpiryCandidate is a customer with a grant expired before a date
// that has no expiry row yet and a positive balance.
type CreditExpiryCandidate struct {
	EstablishmentID string `bun:"establishment_id"`
	CustomerID      string `bun:"customer_id"`
}

// Key is the keyset cursor of the candidate (CustomersWithExpiredCredit).
func (c CreditExpiryCandidate) Key() string { return c.EstablishmentID + ":" + c.CustomerID }

// CustomersWithExpiredCredit lists (all establishments) the customers having
// a positive movement with expires_on < before and no expiry row for it,
// and a positive stored balance: a customer whose balance is zero has
// nothing left to expire (every grant was consumed), so a fully used grant
// does not come back on every run. Ordered by (establishment_id,
// customer_id) and resumed after the given key (keyset), so a run walks
// every candidate in pages of limit. The establishment-local "today" is
// applied by the caller per customer; before is the latest possible today
// (UTC+14), so the list is a superset.
func CustomersWithExpiredCredit(ctx context.Context, db bun.IDB, before calendar.Date, after *CreditExpiryCandidate, limit int) ([]CreditExpiryCandidate, error) {
	var out []CreditExpiryCandidate
	q := db.NewSelect().TableExpr("customer_credits AS cc").
		ColumnExpr("DISTINCT cc.establishment_id, cc.customer_id").
		Join("JOIN customer_credit_balances AS ccb ON ccb.establishment_id = cc.establishment_id AND ccb.customer_id = cc.customer_id").
		Where("ccb.balance > 0").
		Where("cc.amount > 0").
		Where("cc.expires_on IS NOT NULL").
		Where("cc.expires_on < ?", before).
		Where("NOT EXISTS (SELECT 1 FROM customer_credits AS x WHERE x.reference_credit_id = cc.id AND x.source = ?)", CreditSourceExpiry).
		OrderExpr("cc.establishment_id, cc.customer_id").
		Limit(int64(limit))
	if after != nil {
		q = q.Where("(cc.establishment_id > ? OR (cc.establishment_id = ? AND cc.customer_id > ?))", after.EstablishmentID, after.EstablishmentID, after.CustomerID)
	}
	err := q.Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customers with expired credit: %w", err)
	}
	return out, nil
}

// BookingCreditNet is what a booking received through the payment method
// credit and has not yet given back as credit: SUM(succeeded incoming
// payments of method credit) - SUM(succeeded refunds of method credit). A
// cancellation refunds this part as credit, never as money (spec 13.4).
func BookingCreditNet(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (money.Cents, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var sum int64
	err := db.NewSelect().Model((*Payment)(nil)).
		ColumnExpr("COALESCE(SUM(CASE WHEN pm.direction = ? THEN pm.amount ELSE -pm.amount END), 0)", PaymentDirectionIn).
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.booking_id = ?", bookingID).
		Where("pm.method = ?", PaymentMethodCredit).
		Where("pm.status = ?", PaymentSucceeded).
		Scan(ctx, &sum)
	if err != nil {
		return 0, fmt.Errorf("store: booking credit net: %w", err)
	}
	return money.Cents(sum), nil
}

// CreditBookingPolicy returns the policy snapshot of a booking of es (the
// credit validity of the credits it causes).
func CreditBookingPolicy(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (JSON, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var b Booking
	err := db.NewSelect().Model(&b).Column("bk.policy_snapshot").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.id = ?", bookingID).Limit(1).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: booking policy: %w", err)
	}
	return b.PolicySnapshot, nil
}
