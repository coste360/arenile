package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/money"
)

// Repository of the guaranteed refunds (refund_requests, ADR 0019 feature
// B). Ownership: internal/domain/refund.

// openRefundRequestStatuses are the statuses of a request whose money has
// not reached the customer yet (failed included: it waits for a retry).
var openRefundRequestStatuses = []string{RefundRequestPending, RefundRequestProcessing, RefundRequestFailed}

// RefundRequestWithBooking is a refund request with its booking code (list).
type RefundRequestWithBooking struct {
	RefundRequest `bun:",extend"`

	BookingCode *string `bun:"booking_code"`
}

// RefundRequestFilter filters ListRefundRequests.
type RefundRequestFilter struct {
	Status    string
	BookingID string
	// Cursor is the id of the last row of the previous page (newest first).
	Cursor string
	Limit  int
}

// InsertRefundRequest inserts a request of es. A second live request for
// the same (booking, payment intent) or the same idempotency key is
// ErrConflict (refund_requests_booking_intent_uidx, refund_requests_idempotency_uidx).
func InsertRefundRequest(ctx context.Context, db bun.IDB, es EstablishmentScope, rr *RefundRequest) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	rr.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(rr).Exec(ctx)
	return wrapWrite("insert refund request", err)
}

// GetRefundRequest returns a request of es.
func GetRefundRequest(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*RefundRequest, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	rr := new(RefundRequest)
	err := db.NewSelect().Model(rr).Where("rr.id = ?", id).Where("rr.establishment_id = ?", es.ID()).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get refund request", err)
	}
	return rr, nil
}

// UpdateRefundRequest writes the given columns of rr (updated_at and
// version always) only when the stored version is still expected
// (optimistic claim): ErrConflict when another worker changed it first,
// ErrNotFound when it does not exist. rr.Version is incremented on success.
func UpdateRefundRequest(ctx context.Context, db bun.IDB, es EstablishmentScope, rr *RefundRequest, expected int64, columns ...string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	rr.Version = expected + 1
	cols := append([]string{"updated_at", "version"}, columns...)
	res, err := db.NewUpdate().Model(rr).Column(cols...).
		Where("rr.id = ?", rr.ID).Where("rr.establishment_id = ?", es.ID()).Where("rr.version = ?", expected).Exec(ctx)
	if err != nil {
		rr.Version = expected
		return wrapWrite("update refund request", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		rr.Version = expected
		if _, gerr := GetRefundRequest(ctx, db, es, rr.ID); gerr != nil {
			return gerr
		}
		return ErrConflict
	}
	return nil
}

// ListBookingRefundRequests returns the requests of a booking, oldest first.
func ListBookingRefundRequests(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]RefundRequest, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []RefundRequest
	err := db.NewSelect().Model(&items).Where("rr.establishment_id = ?", es.ID()).Where("rr.booking_id = ?", bookingID).
		OrderExpr("rr.created_at ASC, rr.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list booking refund requests: %w", err)
	}
	return items, nil
}

// ListRefundRequests returns a page of the requests of es, newest first,
// with the booking code.
func ListRefundRequests(ctx context.Context, db bun.IDB, es EstablishmentScope, f RefundRequestFilter) (*Page[RefundRequestWithBooking], error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var items []RefundRequestWithBooking
	q := db.NewSelect().Model(&items).ColumnExpr("rr.*").ColumnExpr("bk.code AS booking_code").
		Join("LEFT JOIN bookings AS bk ON bk.id = rr.booking_id AND bk.establishment_id = rr.establishment_id").
		Where("rr.establishment_id = ?", es.ID())
	if f.Status != "" {
		q = q.Where("rr.status = ?", f.Status)
	}
	if f.BookingID != "" {
		q = q.Where("rr.booking_id = ?", f.BookingID)
	}
	if f.Cursor != "" {
		q = q.Where("rr.id < ?", f.Cursor)
	}
	if err := q.OrderExpr("rr.id DESC").Limit(int64(limit + 1)).Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list refund requests: %w", err)
	}
	page := &Page[RefundRequestWithBooking]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	return page, nil
}

// ListDueRefundRequests returns the pending or processing requests whose
// next_attempt_at is due, across establishments (system job), oldest due
// first so a request that keeps failing never starves the others.
func ListDueRefundRequests(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]IntentRef, error) {
	var refs []IntentRef
	err := db.NewSelect().Model((*RefundRequest)(nil)).Column("rr.id", "rr.establishment_id").
		Where("rr.status IN (?)", bun.List([]string{RefundRequestPending, RefundRequestProcessing})).
		Where("rr.next_attempt_at <= ?", now).
		OrderExpr("rr.next_attempt_at ASC, rr.id ASC").Limit(int64(limit)).Scan(ctx, &refs)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list due refund requests: %w", err)
	}
	return refs, nil
}

// SumOpenRefundRequests is the amount of the open (pending, processing,
// failed) requests of a booking or, with paymentIntentID, of one provider
// payment.
func SumOpenRefundRequests(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID, paymentIntentID string) (money.Cents, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var total int64
	q := db.NewSelect().Model((*RefundRequest)(nil)).ColumnExpr("COALESCE(SUM(rr.amount), 0)").
		Where("rr.establishment_id = ?", es.ID()).Where("rr.status IN (?)", bun.List(openRefundRequestStatuses))
	if paymentIntentID != "" {
		q = q.Where("rr.payment_intent_id = ?", paymentIntentID)
	} else {
		q = q.Where("rr.booking_id = ?", bookingID)
	}
	if err := q.Scan(ctx, &total); err != nil && !isNoRows(err) {
		return 0, fmt.Errorf("store: sum open refund requests: %w", err)
	}
	return money.Cents(total), nil
}

// HasOpenRefundRequest reports whether a booking has a refund request
// whose money has not reached the customer yet.
func HasOpenRefundRequest(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().Model((*RefundRequest)(nil)).
		Where("rr.establishment_id = ?", es.ID()).Where("rr.booking_id = ?", bookingID).
		Where("rr.status IN (?)", bun.List(openRefundRequestStatuses)).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: open refund requests: %w", err)
	}
	return n > 0, nil
}

// InsertRefundFailedNotification records the in-panel notification of a
// refund request that exhausted its attempts. The dedupe key makes it
// idempotent: a second insert for the same request and attempt count is
// ignored (reports false).
func InsertRefundFailedNotification(ctx context.Context, db bun.IDB, es EstablishmentScope, n *AdminNotification) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n.EstablishmentID = es.ID()
	res, err := db.NewInsert().Model(n).On("CONFLICT (establishment_id, dedupe_key) DO NOTHING").Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: insert refund notification: %w", err)
	}
	k, _ := res.RowsAffected()
	return k > 0, nil
}
