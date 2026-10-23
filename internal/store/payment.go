package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Repository of the payments ledger (spec 6.7, 13). Owned by
// internal/domain/payment; cash and fiscal read it through the query
// functions below. Every function takes an EstablishmentScope.

// PaymentWithBooking is a payment with the code of its booking (empty for
// counter sales), as listed by the panel.
type PaymentWithBooking struct {
	Payment `bun:",extend"`

	BookingCode *string `bun:"booking_code"`
}

// PaymentFilter narrows ListPayments; zero fields do not filter.
type PaymentFilter struct {
	From          *calendar.Date // business_date >= From
	To            *calendar.Date // business_date <= To
	Method        string
	Direction     string
	CashSessionID string
	BookingID     string
}

// InsertPayment inserts a payments row. A duplicate idempotency key (or
// provider reference) is ErrConflict.
func InsertPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Payment) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	if p.VATBreakdown == nil {
		p.VATBreakdown = []VATShare{}
	}
	if p.Metadata == nil {
		p.Metadata = map[string]any{}
	}
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert payment", err)
}

// GetPayment returns a payment of es.
func GetPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(Payment)
	err := db.NewSelect().Model(p).
		Where("pm.id = ?", id).
		Where("pm.establishment_id = ?", es.ID()).
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get payment", err)
	}
	return p, nil
}

// GetPaymentByIdempotencyKey returns the payment recorded with key in es.
func GetPaymentByIdempotencyKey(ctx context.Context, db bun.IDB, es EstablishmentScope, key string) (*Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(Payment)
	err := db.NewSelect().Model(p).
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.idempotency_key = ?", key).
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get payment by idempotency key", err)
	}
	return p, nil
}

// ListBookingPayments returns every payment and refund of a booking, oldest
// first.
func ListBookingPayments(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []Payment{}
	err := db.NewSelect().Model(&items).
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.booking_id = ?", bookingID).
		OrderExpr("pm.created_at ASC, pm.id ASC").
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list booking payments: %w", err)
	}
	return items, nil
}

// ListPayments lists the payments of es matching f, newest first (id
// descending, UUIDv7), with keyset pagination.
func ListPayments(ctx context.Context, db bun.IDB, es EstablishmentScope, f PaymentFilter, cursor string, limit int) (Page[PaymentWithBooking], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[PaymentWithBooking]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[PaymentWithBooking]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []PaymentWithBooking
	q := db.NewSelect().Model(&items).
		ColumnExpr("pm.*").
		ColumnExpr("bk.code AS booking_code").
		Join("LEFT JOIN bookings AS bk ON bk.id = pm.booking_id AND bk.establishment_id = pm.establishment_id").
		Where("pm.establishment_id = ?", es.ID()).
		OrderExpr("pm.id DESC").
		Limit(int64(limit + 1))
	q = applyPaymentFilter(q, f)
	if before != "" {
		q = q.Where("pm.id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[PaymentWithBooking]{}, fmt.Errorf("store: list payments: %w", err)
	}
	page := Page[PaymentWithBooking]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	if page.Items == nil {
		page.Items = []PaymentWithBooking{}
	}
	return page, nil
}

func applyPaymentFilter(q *bun.SelectQuery, f PaymentFilter) *bun.SelectQuery {
	if f.From != nil {
		q = q.Where("pm.business_date >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("pm.business_date <= ?", *f.To)
	}
	if f.Method != "" {
		q = q.Where("pm.method = ?", f.Method)
	}
	if f.Direction != "" {
		q = q.Where("pm.direction = ?", f.Direction)
	}
	if f.CashSessionID != "" {
		q = q.Where("pm.cash_session_id = ?", f.CashSessionID)
	}
	if f.BookingID != "" {
		q = q.Where("pm.booking_id = ?", f.BookingID)
	}
	return q
}

// PaymentTotals are the succeeded amounts of a ledger slice.
type PaymentTotals struct {
	In       money.Cents // direction in
	Refunded money.Cents // direction refund
}

// Net is In - Refunded (INV-22: equals bookings.paid_amount).
func (t PaymentTotals) Net() money.Cents { return t.In - t.Refunded }

// BookingPaymentTotals sums the succeeded payments and refunds of a booking.
func BookingPaymentTotals(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (PaymentTotals, error) {
	return paymentTotals(ctx, db, es, "pm.booking_id = ?", bookingID)
}

// RefundedOfPayment sums the succeeded refunds that point at paymentID.
func RefundedOfPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, paymentID string) (money.Cents, error) {
	t, err := paymentTotals(ctx, db, es, "pm.refund_of_payment_id = ?", paymentID)
	return t.Refunded, err
}

func paymentTotals(ctx context.Context, db bun.IDB, es EstablishmentScope, where string, arg any) (PaymentTotals, error) {
	if err := es.mustBeValid(); err != nil {
		return PaymentTotals{}, err
	}
	var rows []struct {
		Direction string      `bun:"direction"`
		Total     money.Cents `bun:"total"`
	}
	err := db.NewSelect().TableExpr("payments AS pm").
		ColumnExpr("pm.direction AS direction").
		ColumnExpr("COALESCE(SUM(pm.amount), 0) AS total").
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.status = ?", PaymentSucceeded).
		Where(where, arg).
		GroupExpr("pm.direction").
		Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return PaymentTotals{}, fmt.Errorf("store: payment totals: %w", err)
	}
	var t PaymentTotals
	for _, r := range rows {
		switch r.Direction {
		case PaymentDirectionIn:
			t.In += r.Total
		case PaymentDirectionRefund:
			t.Refunded += r.Total
		}
	}
	return t, nil
}

// ListSucceededPayments returns the succeeded payments of es with
// business_date in [from, to], ordered by business_date and id (the input
// of the corrispettivi export).
func ListSucceededPayments(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []Payment{}
	err := db.NewSelect().Model(&items).
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.status = ?", PaymentSucceeded).
		Where("pm.business_date >= ?", from).
		Where("pm.business_date <= ?", to).
		OrderExpr("pm.business_date ASC, pm.id ASC").
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list succeeded payments: %w", err)
	}
	return items, nil
}

// ListSessionPayments returns the payments attached to a cash session,
// oldest first.
func ListSessionPayments(ctx context.Context, db bun.IDB, es EstablishmentScope, sessionID string) ([]Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []Payment{}
	err := db.NewSelect().Model(&items).
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.cash_session_id = ?", sessionID).
		OrderExpr("pm.id ASC").
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list session payments: %w", err)
	}
	return items, nil
}

// PaymentCustomerExists reports whether customerID is a (non-anonymized)
// customer of es; used by counter sales that name a customer.
func PaymentCustomerExists(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().TableExpr("customers AS cu").
		Where("cu.id = ?", customerID).
		Where("cu.establishment_id = ?", es.ID()).
		Where("cu.anonymized_at IS NULL").
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: customer exists: %w", err)
	}
	return n > 0, nil
}

// PaymentBookingExists reports whether bookingID is a booking of es (the
// payment reads answer 404 for unknown bookings without locking them).
func PaymentBookingExists(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().TableExpr("bookings AS bk").
		Where("bk.id = ?", bookingID).
		Where("bk.establishment_id = ?", es.ID()).
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: booking exists: %w", err)
	}
	return n > 0, nil
}
