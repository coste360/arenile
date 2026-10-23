package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/money"
)

// Repository of provider payments and refunds (payment_intents) and of the
// processed provider webhook events (webhook_events), migration 00005.
// Owned by internal/domain/payment/gateway. Every operational function
// takes an EstablishmentScope; ListReconcilableIntents is the only
// installation-wide scan (the reconciliation job, like the hold expiry
// job) and returns references to be re-scoped one by one.

// openIntentStatuses are the statuses of intents still waiting for the
// provider.
var openIntentStatuses = []string{IntentCreated, IntentPending}

// PaymentIntentWithBooking is an intent with the code of its booking and,
// when the ledger row exists, its amount (reconciliation report).
type PaymentIntentWithBooking struct {
	PaymentIntent `bun:",extend"`

	BookingCode   *string      `bun:"booking_code"`
	LedgerAmount  *money.Cents `bun:"ledger_amount"`
	LedgerStatus  *string      `bun:"ledger_status"`
	LedgerPayment *string      `bun:"ledger_payment_id"`
}

// IntentRef identifies an intent across establishments (jobs only).
type IntentRef struct {
	ID              string `bun:"id"`
	EstablishmentID string `bun:"establishment_id"`
}

// InsertPaymentIntent inserts an intent of es. A duplicate (provider,
// external_id) or (establishment, idempotency_key) is ErrConflict.
func InsertPaymentIntent(ctx context.Context, db bun.IDB, es EstablishmentScope, pi *PaymentIntent) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	pi.EstablishmentID = es.ID()
	if pi.RawEventIDs == nil {
		pi.RawEventIDs = []string{}
	}
	_, err := db.NewInsert().Model(pi).Exec(ctx)
	return wrapWrite("insert payment intent", err)
}

// UpdatePaymentIntent writes the given columns of pi (updated_at always).
func UpdatePaymentIntent(ctx context.Context, db bun.IDB, es EstablishmentScope, pi *PaymentIntent, columns ...string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if pi.RawEventIDs == nil {
		pi.RawEventIDs = []string{}
	}
	cols := append([]string{"updated_at"}, columns...)
	res, err := db.NewUpdate().Model(pi).Column(cols...).
		Where("pi.id = ?", pi.ID).Where("pi.establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("update payment intent", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPaymentIntent returns an intent of es.
func GetPaymentIntent(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*PaymentIntent, error) {
	return onePaymentIntent(ctx, db, es, func(q *bun.SelectQuery) *bun.SelectQuery { return q.Where("pi.id = ?", id) })
}

// LockPaymentIntent locks an intent of es for the rest of tx (no-op UPDATE,
// which also takes the SQLite write lock) and returns it. Lock order of the
// gateway: payment intent, then booking.
func LockPaymentIntent(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*PaymentIntent, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	res, err := db.NewUpdate().Model((*PaymentIntent)(nil)).Set("updated_at = updated_at").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: lock payment intent: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return GetPaymentIntent(ctx, db, es, id)
}

// FindPaymentIntentByRef returns the intent of es and provider whose
// external_id (or, failing that, external_ref) is ref.
func FindPaymentIntentByRef(ctx context.Context, db bun.IDB, es EstablishmentScope, provider, ref string) (*PaymentIntent, error) {
	pi, err := onePaymentIntent(ctx, db, es, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("pi.provider = ?", provider).Where("pi.external_id = ?", ref)
	})
	if err == nil || ref == "" {
		return pi, err
	}
	return onePaymentIntent(ctx, db, es, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("pi.provider = ?", provider).Where("pi.external_ref = ?", ref).
			Where("pi.kind = ?", IntentKindPayment).OrderExpr("pi.created_at DESC, pi.id DESC")
	})
}

// GetPaymentIntentByIdempotencyKey returns the intent of es created with key.
func GetPaymentIntentByIdempotencyKey(ctx context.Context, db bun.IDB, es EstablishmentScope, key string) (*PaymentIntent, error) {
	return onePaymentIntent(ctx, db, es, func(q *bun.SelectQuery) *bun.SelectQuery { return q.Where("pi.idempotency_key = ?", key) })
}

// GetPaymentIntentByPaymentID returns the provider payment whose ledger row
// is paymentID.
func GetPaymentIntentByPaymentID(ctx context.Context, db bun.IDB, es EstablishmentScope, paymentID string) (*PaymentIntent, error) {
	return onePaymentIntent(ctx, db, es, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("pi.payment_id = ?", paymentID).Where("pi.kind = ?", IntentKindPayment)
	})
}

// LatestPaymentIntentForBooking returns the newest provider payment of a booking.
func LatestPaymentIntentForBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (*PaymentIntent, error) {
	return onePaymentIntent(ctx, db, es, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("pi.booking_id = ?", bookingID).Where("pi.kind = ?", IntentKindPayment).
			OrderExpr("pi.attempt DESC, pi.created_at DESC, pi.id DESC")
	})
}

func onePaymentIntent(ctx context.Context, db bun.IDB, es EstablishmentScope, where func(*bun.SelectQuery) *bun.SelectQuery) (*PaymentIntent, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	pi := new(PaymentIntent)
	err := where(db.NewSelect().Model(pi).Where("pi.establishment_id = ?", es.ID())).Limit(1).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get payment intent: %w", err)
	}
	return pi, nil
}

// MaxPaymentAttempt is the highest attempt number of the provider payments
// of a booking (0 when none).
func MaxPaymentAttempt(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var n int
	err := db.NewSelect().Model((*PaymentIntent)(nil)).ColumnExpr("COALESCE(MAX(pi.attempt), 0)").
		Where("pi.establishment_id = ?", es.ID()).Where("pi.booking_id = ?", bookingID).
		Where("pi.kind = ?", IntentKindPayment).Scan(ctx, &n)
	if err != nil && !isNoRows(err) {
		return 0, fmt.Errorf("store: max payment attempt: %w", err)
	}
	return n, nil
}

// ListOpenPaymentIntents returns the provider payments of a booking still
// waiting for the provider (created or pending), oldest first.
func ListOpenPaymentIntents(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]PaymentIntent, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []PaymentIntent
	err := db.NewSelect().Model(&items).
		Where("pi.establishment_id = ?", es.ID()).Where("pi.booking_id = ?", bookingID).
		Where("pi.kind = ?", IntentKindPayment).Where("pi.status IN (?)", bun.List(openIntentStatuses)).
		OrderExpr("pi.created_at ASC, pi.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list open payment intents: %w", err)
	}
	return items, nil
}

// ListSucceededPaymentIntents returns the provider payments of a booking
// that succeeded (status succeeded, ledger row written), oldest first.
func ListSucceededPaymentIntents(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]PaymentIntent, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []PaymentIntent
	err := db.NewSelect().Model(&items).
		Where("pi.establishment_id = ?", es.ID()).Where("pi.booking_id = ?", bookingID).
		Where("pi.kind = ?", IntentKindPayment).Where("pi.status = ?", IntentSucceeded).Where("pi.payment_id IS NOT NULL").
		OrderExpr("pi.created_at ASC, pi.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list succeeded payment intents: %w", err)
	}
	return items, nil
}

// ListRefundIntents returns the refund intents of a provider payment,
// oldest first; openOnly keeps those still waiting for the provider.
func ListRefundIntents(ctx context.Context, db bun.IDB, es EstablishmentScope, paymentIntentID string, openOnly bool) ([]PaymentIntent, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []PaymentIntent
	q := db.NewSelect().Model(&items).
		Where("pi.establishment_id = ?", es.ID()).Where("pi.refund_of_intent_id = ?", paymentIntentID).
		Where("pi.kind = ?", IntentKindRefund)
	if openOnly {
		q = q.Where("pi.status IN (?)", bun.List(openIntentStatuses))
	}
	err := q.OrderExpr("pi.created_at ASC, pi.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list refund intents: %w", err)
	}
	return items, nil
}

// SumOpenRefundIntents sums the refunds of es still waiting for the
// provider, of one provider payment (paymentIntentID) or, when empty, of
// one booking.
func SumOpenRefundIntents(ctx context.Context, db bun.IDB, es EstablishmentScope, paymentIntentID, bookingID string) (money.Cents, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var total int64
	q := db.NewSelect().Model((*PaymentIntent)(nil)).ColumnExpr("COALESCE(SUM(pi.amount), 0)").
		Where("pi.establishment_id = ?", es.ID()).Where("pi.kind = ?", IntentKindRefund).
		Where("pi.status IN (?)", bun.List(openIntentStatuses))
	if paymentIntentID != "" {
		q = q.Where("pi.refund_of_intent_id = ?", paymentIntentID)
	} else {
		q = q.Where("pi.booking_id = ?", bookingID)
	}
	if err := q.Scan(ctx, &total); err != nil && !isNoRows(err) {
		return 0, fmt.Errorf("store: sum open refund intents: %w", err)
	}
	return money.Cents(total), nil
}

// ListPaymentIntents returns the intents of es created in [from, to),
// newest first, with the booking code and the ledger row (report).
func ListPaymentIntents(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to time.Time, limit int) ([]PaymentIntentWithBooking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []PaymentIntentWithBooking
	err := db.NewSelect().Model(&items).
		ColumnExpr("pi.*").
		ColumnExpr("bk.code AS booking_code").
		ColumnExpr("pm.amount AS ledger_amount").
		ColumnExpr("pm.status AS ledger_status").
		ColumnExpr("pm.id AS ledger_payment_id").
		Join("LEFT JOIN bookings AS bk ON bk.id = pi.booking_id AND bk.establishment_id = pi.establishment_id").
		Join("LEFT JOIN payments AS pm ON pm.id = pi.payment_id AND pm.establishment_id = pi.establishment_id").
		Where("pi.establishment_id = ?", es.ID()).
		Where("pi.created_at >= ?", from).Where("pi.created_at < ?", to).
		OrderExpr("pi.created_at DESC, pi.id DESC").Limit(int64(limit)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list payment intents: %w", err)
	}
	return items, nil
}

// ListReconcilableIntents returns, across establishments (or of one
// establishment when establishmentID is set), the intents the
// reconciliation must look at: payments and refunds still created or
// pending and older than olderThan, least recently checked first.
func ListReconcilableIntents(ctx context.Context, db bun.IDB, establishmentID string, olderThan time.Time, limit int) ([]IntentRef, error) {
	var refs []IntentRef
	q := db.NewSelect().Model((*PaymentIntent)(nil)).Column("pi.id", "pi.establishment_id").
		Where("pi.status IN (?)", bun.List(openIntentStatuses)).
		Where("pi.created_at < ?", olderThan)
	if establishmentID != "" {
		q = q.Where("pi.establishment_id = ?", establishmentID)
	}
	// Least recently checked first: an intent that keeps failing (provider
	// down, credentials removed) never starves the others.
	err := q.OrderExpr("COALESCE(pi.last_checked_at, pi.created_at) ASC, pi.id ASC").Limit(int64(limit)).Scan(ctx, &refs)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list reconcilable intents: %w", err)
	}
	return refs, nil
}

// InsertWebhookEvent records a provider event of es. A second insert of the
// same (establishment, provider, event_id) is ErrConflict: the event was
// already applied (INV-23). It must run in the transaction that applies the
// event. The key is per establishment: one provider account shared by two
// establishments delivers the same event id to both endpoints.
func InsertWebhookEvent(ctx context.Context, db bun.IDB, es EstablishmentScope, ev *WebhookEvent) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	ev.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(ev).Exec(ctx)
	return wrapWrite("insert webhook event", err)
}

// WebhookEventExists reports whether (provider, eventID) was recorded for es.
func WebhookEventExists(ctx context.Context, db bun.IDB, es EstablishmentScope, provider, eventID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().Model((*WebhookEvent)(nil)).Where("whe.establishment_id = ?", es.ID()).
		Where("whe.provider = ?", provider).Where("whe.event_id = ?", eventID).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: webhook event exists: %w", err)
	}
	return n > 0, nil
}

// GetPaymentByProviderRef returns the ledger row of es written for the
// provider reference ref (unique per provider).
func GetPaymentByProviderRef(ctx context.Context, db bun.IDB, es EstablishmentScope, provider, ref string) (*Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(Payment)
	err := db.NewSelect().Model(p).
		Where("pm.establishment_id = ?", es.ID()).Where("pm.provider = ?", provider).Where("pm.provider_ref = ?", ref).
		Limit(1).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get payment by provider ref: %w", err)
	}
	return p, nil
}
