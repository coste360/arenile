package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/money"
)

// Repository of test_mode_settings and of the bulk purge of test rows
// (spec 15.4, migration 00009, ADR 0019). Owner: internal/domain/testmode.
// No row = test mode off.

// GetTestModeSettings returns the test-mode row of es (ErrNotFound when
// test mode was never switched on).
func GetTestModeSettings(ctx context.Context, db bun.IDB, es EstablishmentScope) (*TestModeSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := new(TestModeSettings)
	err := db.NewSelect().Model(row).Where("tms.establishment_id = ?", es.ID()).Scan(ctx)
	return row, wrapRead("get test mode settings", err)
}

// TestModeEnabled reports whether test mode is on for es. Called inside the
// transaction that creates a booking or a payment, so the flag copied on the
// row is the one of the moment of the creation.
func TestModeEnabled(ctx context.Context, db bun.IDB, es EstablishmentScope) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	var n int
	err := db.NewSelect().TableExpr("test_mode_settings").ColumnExpr("COUNT(*)").
		Where("establishment_id = ?", es.ID()).Where("enabled = ?", true).Scan(ctx, &n)
	if err != nil && !isNoRows(err) {
		return false, fmt.Errorf("store: test mode enabled: %w", err)
	}
	return n > 0, nil
}

// BookingIsTest reports the test flag of a booking of es (false when the
// booking does not exist).
func BookingIsTest(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	var n int
	err := db.NewSelect().TableExpr("bookings").ColumnExpr("COUNT(*)").
		Where("establishment_id = ?", es.ID()).Where("id = ?", bookingID).Where("test = ?", true).Scan(ctx, &n)
	if err != nil && !isNoRows(err) {
		return false, fmt.Errorf("store: booking test flag: %w", err)
	}
	return n > 0, nil
}

// EnsureTestModeSettings returns the row of es, inserting it switched off
// (version 1) on first use; concurrent first uses converge on one row.
func EnsureTestModeSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, now time.Time) (*TestModeSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := &TestModeSettings{EstablishmentID: es.ID(), Version: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	if _, err := db.NewInsert().Model(row).On("CONFLICT (establishment_id) DO NOTHING").Exec(ctx); err != nil {
		return nil, wrapWrite("ensure test mode settings", err)
	}
	return GetTestModeSettings(ctx, db, es)
}

// UpdateTestModeSettingsVersioned writes the switch columns of row when the
// stored version is still oldVersion and sets row.Version to oldVersion+1.
// It reports false when no row matched (concurrent change).
func UpdateTestModeSettingsVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, row *TestModeSettings, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	row.Version = oldVersion + 1
	res, err := db.NewUpdate().Model(row).
		Column("enabled", "public_enabled", "enabled_at", "enabled_by_user_id", "version", "updated_at").
		Where("establishment_id = ?", es.ID()).Where("version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		row.Version = oldVersion
		return false, wrapWrite("update test mode settings", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update test mode settings: %w", err)
	}
	if n != 1 {
		row.Version = oldVersion
		return false, nil
	}
	return true, nil
}

// TestRowCounts are the test rows of an establishment.
type TestRowCounts struct {
	Bookings int64
	Payments int64
}

// CountTestRows counts the test bookings and test payments of es.
func CountTestRows(ctx context.Context, db bun.IDB, es EstablishmentScope) (TestRowCounts, error) {
	if err := es.mustBeValid(); err != nil {
		return TestRowCounts{}, err
	}
	var out TestRowCounts
	if err := db.NewSelect().TableExpr("bookings").ColumnExpr("COUNT(*)").
		Where("establishment_id = ?", es.ID()).Where("test = ?", true).Scan(ctx, &out.Bookings); err != nil && !isNoRows(err) {
		return out, fmt.Errorf("store: count test bookings: %w", err)
	}
	if err := db.NewSelect().TableExpr("payments").ColumnExpr("COUNT(*)").
		Where("establishment_id = ?", es.ID()).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("test = ?", true).WhereOr("booking_id IN (?)", testBookingIDs(db, es))
		}).Scan(ctx, &out.Payments); err != nil && !isNoRows(err) {
		return out, fmt.Errorf("store: count test payments: %w", err)
	}
	return out, nil
}

// ListTestBookingIDs returns the ids of the test bookings of es.
func ListTestBookingIDs(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	ids := []string{}
	err := db.NewSelect().TableExpr("bookings").Column("id").
		Where("establishment_id = ?", es.ID()).Where("test = ?", true).
		OrderExpr("id ASC").Scan(ctx, &ids)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list test bookings: %w", err)
	}
	return ids, nil
}

func testBookingIDs(db bun.IDB, es EstablishmentScope) *bun.SelectQuery {
	return db.NewSelect().TableExpr("bookings").Column("id").
		Where("establishment_id = ?", es.ID()).Where("test = ?", true)
}

// DeleteTestRows deletes, in the caller's transaction, every test row of es:
// the test bookings with their lines, extras, allocations (by cascade),
// checkout sessions and magic links, the payments and payment intents,
// refund requests, fiscal documents, renewal proposals and message log rows
// that belong to them, the counter sales taken in test mode, and the credit
// ledger rows they generated (the credit balances of the customers touched
// are recomputed from the remaining ledger, never below zero). audit_log
// rows stay. The caller must release the pool counters of the test bookings
// first (inventory.Release): the cascade on pool_allocations does not touch
// pool_usage. Never touches a row with test = false that does not belong to
// a test booking.
func DeleteTestRows(ctx context.Context, tx bun.IDB, es EstablishmentScope, bookingIDs []string, now time.Time) (TestRowCounts, error) {
	if err := es.mustBeValid(); err != nil {
		return TestRowCounts{}, err
	}
	estID := es.ID()
	// The bookings are the listed ones (whose allocations the caller
	// released) that are still test rows: a test booking created meanwhile
	// by a concurrent transaction is left for the next purge.
	// Never an empty IN list: the nil UUID (a valid uuid literal for
	// PostgreSQL, unlike "") names no row.
	ids := append([]string{"00000000-0000-0000-0000-000000000000"}, bookingIDs...)
	bookings := func() *bun.SelectQuery {
		return testBookingIDs(tx, es).Where("id IN (?)", bun.List(ids))
	}
	payments := func() *bun.SelectQuery {
		return tx.NewSelect().TableExpr("payments").Column("id").
			Where("establishment_id = ?", estID).
			WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("booking_id IN (?)", bookings()).
					WhereOr("test = ? AND booking_id IS NULL", true)
			})
	}
	var counts TestRowCounts
	if err := tx.NewSelect().TableExpr("(?) AS b", bookings()).ColumnExpr("COUNT(*)").Scan(ctx, &counts.Bookings); err != nil && !isNoRows(err) {
		return counts, fmt.Errorf("store: purge test rows: count bookings: %w", err)
	}
	if err := tx.NewSelect().TableExpr("(?) AS p", payments()).ColumnExpr("COUNT(*)").Scan(ctx, &counts.Payments); err != nil && !isNoRows(err) {
		return counts, fmt.Errorf("store: purge test rows: count payments: %w", err)
	}
	if counts.Bookings == 0 && counts.Payments == 0 {
		return counts, nil
	}

	// Credit ledger rows generated by test bookings or test payments.
	var customers []string
	if err := tx.NewSelect().TableExpr("customer_credits").ColumnExpr("DISTINCT customer_id").
		Where("establishment_id = ?", estID).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("booking_id IN (?)", bookings()).WhereOr("payment_id IN (?)", payments())
		}).Scan(ctx, &customers); err != nil && !isNoRows(err) {
		return counts, fmt.Errorf("store: purge test rows: credit customers: %w", err)
	}
	creditIDs := func() *bun.SelectQuery {
		return tx.NewSelect().TableExpr("customer_credits").Column("id").
			Where("establishment_id = ?", estID).
			WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("booking_id IN (?)", bookings()).WhereOr("payment_id IN (?)", payments())
			})
	}
	// Expiry rows of the deleted grants first (CHECK: an expiry row always
	// references its grant).
	if _, err := tx.NewDelete().TableExpr("customer_credits").
		Where("establishment_id = ?", estID).Where("source = ?", "expiry").
		Where("reference_credit_id IN (?)", creditIDs()).Exec(ctx); err != nil {
		return counts, fmt.Errorf("store: purge test rows: credit expiries: %w", err)
	}
	if _, err := tx.NewDelete().TableExpr("customer_credits").
		Where("id IN (?)", creditIDs()).Exec(ctx); err != nil {
		return counts, fmt.Errorf("store: purge test rows: credits: %w", err)
	}

	steps := []struct {
		name string
		q    *bun.DeleteQuery
	}{
		{"refund requests", tx.NewDelete().TableExpr("refund_requests").
			Where("establishment_id = ?", estID).Where("booking_id IN (?)", bookings())},
		{"fiscal documents", tx.NewDelete().TableExpr("fiscal_documents").
			Where("establishment_id = ?", estID).Where("payment_id IN (?)", payments())},
		{"payments", tx.NewDelete().TableExpr("payments").
			Where("id IN (?)", payments())},
		{"payment intents", tx.NewDelete().TableExpr("payment_intents").
			Where("establishment_id = ?", estID).Where("booking_id IN (?)", bookings())},
		{"renewal proposals", tx.NewDelete().TableExpr("renewal_proposals").
			Where("establishment_id = ?", estID).
			WhereGroup(" AND ", func(q *bun.DeleteQuery) *bun.DeleteQuery {
				return q.Where("booking_id IN (?)", bookings()).WhereOr("source_booking_id IN (?)", bookings())
			})},
		{"message log", tx.NewDelete().TableExpr("message_log").
			Where("establishment_id = ?", estID).Where("booking_id IN (?)", bookings())},
		{"bookings", tx.NewDelete().TableExpr("bookings").
			Where("establishment_id = ?", estID).Where("test = ?", true).Where("id IN (?)", bun.List(ids))},
	}
	for _, s := range steps {
		if _, err := s.q.Exec(ctx); err != nil {
			return counts, fmt.Errorf("store: purge test rows: %s: %w", s.name, err)
		}
	}

	for _, c := range customers {
		if err := recomputeCreditBalance(ctx, tx, es, c, now); err != nil {
			return counts, err
		}
	}
	return counts, nil
}

// recomputeCreditBalance rewrites the stored balance of a customer as the
// sum of the remaining ledger rows. A negative sum (a test grant that a real
// redemption consumed) gets a compensating adjustment row, like migration
// 00008 did, so that the invariant balance = SUM(amount) >= 0 holds.
func recomputeCreditBalance(ctx context.Context, tx bun.IDB, es EstablishmentScope, customerID string, now time.Time) error {
	var sum int64
	if err := tx.NewSelect().TableExpr("customer_credits").ColumnExpr("COALESCE(SUM(amount), 0)").
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).Scan(ctx, &sum); err != nil && !isNoRows(err) {
		return fmt.Errorf("store: purge test rows: credit sum: %w", err)
	}
	if sum < 0 {
		row := &CustomerCredit{
			ID: NewID(), EstablishmentID: es.ID(), CustomerID: customerID, Amount: money.Cents(-sum),
			Source: "adjustment", Note: "Rettifica automatica: eliminazione delle prenotazioni di prova", CreatedAt: now.UTC(),
		}
		if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
			return fmt.Errorf("store: purge test rows: credit adjustment: %w", err)
		}
		sum = 0
	}
	if _, err := tx.NewUpdate().TableExpr("customer_credit_balances").
		Set("balance = ?", sum).Set("updated_at = ?", now.UTC()).
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).Exec(ctx); err != nil {
		return fmt.Errorf("store: purge test rows: credit balance: %w", err)
	}
	return nil
}

// TestCreditShortfall counts the customers whose credit ledger would go
// negative without the credit rows generated by the test rows of es: a
// credit granted by a test booking (or a test payment) that a real booking
// already spent. The purge refuses to run in that case (INV-76): deleting
// the grant would leave real money paid with credit that never existed.
func TestCreditShortfall(ctx context.Context, db bun.IDB, es EstablishmentScope) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	estID := es.ID()
	payments := db.NewSelect().TableExpr("payments").Column("id").
		Where("establishment_id = ?", estID).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("booking_id IN (?)", testBookingIDs(db, es)).WhereOr("test = ? AND booking_id IS NULL", true)
		})
	testCredits := func() *bun.SelectQuery {
		return db.NewSelect().TableExpr("customer_credits").Column("id").
			Where("establishment_id = ?", estID).
			WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("booking_id IN (?)", testBookingIDs(db, es)).WhereOr("payment_id IN (?)", payments)
			})
	}
	customers := db.NewSelect().TableExpr("customer_credits").ColumnExpr("DISTINCT customer_id").
		Where("establishment_id = ?", estID).Where("id IN (?)", testCredits())
	remaining := db.NewSelect().TableExpr("customer_credits AS c").Column("c.customer_id").
		Where("c.establishment_id = ?", estID).
		Where("c.customer_id IN (?)", customers).
		Where("c.id NOT IN (?)", testCredits()).
		Where("c.reference_credit_id IS NULL OR c.reference_credit_id NOT IN (?)", testCredits()).
		GroupExpr("c.customer_id").Having("SUM(c.amount) < 0")
	var n int
	if err := db.NewSelect().TableExpr("(?) AS s", remaining).ColumnExpr("COUNT(*)").Scan(ctx, &n); err != nil && !isNoRows(err) {
		return 0, fmt.Errorf("store: test credit shortfall: %w", err)
	}
	return n, nil
}

// IsTestEventSubject reports whether the booking or the payment an event
// is about (ids from the event payload, empty = none) is a test-mode row of
// the establishment. Used by the outbox when an event is written, to flag
// the row and keep it out of the outgoing webhooks (spec 15.4, INV-74).
func IsTestEventSubject(ctx context.Context, db bun.IDB, establishmentID, bookingID, paymentID string) (bool, error) {
	// A subject id that is not a UUID cannot name a row (and PostgreSQL
	// would refuse the comparison with the uuid column, aborting the
	// dispatcher transaction): treat it as absent.
	if !looksLikeUUID(bookingID) {
		bookingID = ""
	}
	if !looksLikeUUID(paymentID) {
		paymentID = ""
	}
	if establishmentID == "" || (bookingID == "" && paymentID == "") {
		return false, nil
	}
	var n int
	if bookingID != "" {
		if err := db.NewSelect().TableExpr("bookings").ColumnExpr("COUNT(*)").
			Where("establishment_id = ?", establishmentID).Where("id = ?", bookingID).Where("test = ?", true).
			Scan(ctx, &n); err != nil && !isNoRows(err) {
			return false, fmt.Errorf("store: test event subject: %w", err)
		}
		if n > 0 {
			return true, nil
		}
	}
	if paymentID != "" {
		if err := db.NewSelect().TableExpr("payments").ColumnExpr("COUNT(*)").
			Where("establishment_id = ?", establishmentID).Where("id = ?", paymentID).Where("test = ?", true).
			Scan(ctx, &n); err != nil && !isNoRows(err) {
			return false, fmt.Errorf("store: test event subject: %w", err)
		}
	}
	return n > 0, nil
}

// looksLikeUUID reports whether s parses as a UUID.
func looksLikeUUID(s string) bool { return uuid.Validate(s) == nil }
