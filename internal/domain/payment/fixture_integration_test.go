//go:build integration

package payment_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app/builtin"
	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// t0 is 2027-07-15 10:00 in Rome (08:00 UTC): business date 2027-07-15.
var t0 = time.Date(2027, 7, 15, 8, 0, 0, 0, time.UTC)

var day0 = calendar.MustParse("2027-07-15")

// registry is the test double of *app.Registry (payment.Providers and
// fiscal.Drivers) with the builtin drivers.
type registry struct{ c clock.Clock }

func (registry) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	if code == builtin.ManualCode {
		return builtin.Manual{}, true
	}
	return nil, false
}

func (r registry) FiscalDriver(code string) (ext.FiscalDriver, bool) {
	if code == builtin.NoneCode {
		return builtin.NoneFiscal{Clock: r.c}, true
	}
	return nil, false
}

// ledger is a store-backed double of booking.Ledger (payment.BookingLedger):
// it locks the booking row with a no-op UPDATE and adds to paid_amount, as
// the real adapter does.
type ledger struct{}

func (ledger) LoadForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, id string) (*store.Booking, error) {
	res, err := tx.NewUpdate().TableExpr("bookings").Set("version = version").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, store.ErrNotFound
	}
	return loadBooking(ctx, tx, es, id)
}

func (ledger) ApplyPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, id string, signed money.Cents) (*store.Booking, error) {
	if _, err := tx.NewUpdate().TableExpr("bookings").
		Set("paid_amount = paid_amount + ?", int64(signed)).Set("version = version + 1").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx); err != nil {
		return nil, err
	}
	return loadBooking(ctx, tx, es, id)
}

func loadBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) (*store.Booking, error) {
	b := new(store.Booking)
	if err := db.NewSelect().Model(b).Where("bk.id = ?", id).Where("bk.establishment_id = ?", es.ID()).Scan(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

// credits is a store-backed double of the customer credit ledger.
type credits struct{}

func (credits) GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error {
	c.EstablishmentID = es.ID()
	_, err := tx.NewInsert().Model(c).Exec(ctx)
	return err
}

func (credits) CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	var total int64
	err := db.NewSelect().TableExpr("customer_credits").ColumnExpr("COALESCE(SUM(amount), 0)").
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).
		Where("(expires_on IS NULL OR expires_on >= ?)", today).Scan(ctx, &total)
	return money.Cents(total), err
}

// confirmer records the manual-driver confirmations.
type confirmer struct {
	mu    sync.Mutex
	calls []string
}

func (c *confirmer) ConfirmFromPayment(ctx context.Context, tx bun.IDB, _ *auth.Principal, es store.EstablishmentScope, bookingID string, _ *store.Payment) error {
	c.mu.Lock()
	c.calls = append(c.calls, bookingID)
	c.mu.Unlock()
	_, err := tx.NewUpdate().TableExpr("bookings").Set("status = 'confirmed'").Set("hold_expires_at = NULL").
		Where("id = ?", bookingID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	return err
}

type env struct {
	ctx     context.Context
	db      *store.DB
	clock   *clock.Fake
	svc     *payment.Service
	cash    *cash.Service
	fiscal  *fiscal.Service
	conf    *confirmer
	estA    *store.Establishment
	estB    *store.Establishment
	esA     store.EstablishmentScope
	owner   *auth.Principal
	cashier *auth.Principal
	viewer  *auth.Principal
	ownerB  *auth.Principal
	custA   *store.Customer
	seq     atomic.Int64
}

func newEnv(t testing.TB, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", t0)
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
	if err != nil {
		t.Fatal(err)
	}
	// Default VAT 22% for payments without a price snapshot.
	estA.Settings = store.JSON(`{"pricing":{"default_vat_rate_bp":2200}}`)
	if _, err := db.NewUpdate().Model(estA).Column("settings").WherePK().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(t0)
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	e := &env{ctx: ctx, db: db, clock: c, estA: estA, estB: estB, conf: &confirmer{}}
	e.cash = cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	e.fiscal = fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: registry{c}})
	e.svc = payment.NewService(payment.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: ledger{}, Cash: e.cash, Fiscal: e.fiscal,
		Credits: credits{}, Providers: registry{c}, Confirmer: e.conf,
	})
	e.owner = e.user(t, orgA.ID, "", auth.RoleOwner)
	e.cashier = e.user(t, orgA.ID, estA.ID, auth.RoleCashier)
	e.viewer = e.user(t, orgA.ID, estA.ID, auth.RoleViewer)
	e.ownerB = e.user(t, orgB.ID, "", auth.RoleOwner)
	if e.esA, err = store.SystemEstablishmentScope(ctx, db, estA.ID); err != nil {
		t.Fatal(err)
	}
	e.custA = e.customer(t, estA.ID)
	return e
}

func (e *env) user(t testing.TB, orgID, estID string, role auth.Role) *auth.Principal {
	t.Helper()
	u := &store.User{
		ID: store.NewID(), Email: fmt.Sprintf("%s-%d@lido.it", role, e.seq.Add(1)), PasswordHash: "x", Name: string(role),
		Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0,
	}
	if err := store.InsertUser(e.ctx, e.db, u); err != nil {
		t.Fatal(err)
	}
	return &auth.Principal{Kind: auth.PrincipalUser, UserID: u.ID, Grants: []auth.Grant{{OrganizationID: orgID, EstablishmentID: estID, Role: role}}}
}

func (e *env) customer(t testing.TB, estID string) *store.Customer {
	t.Helper()
	c := &store.Customer{
		ID: store.NewID(), EstablishmentID: estID, FirstName: "Mario", LastName: "Rossi", Locale: "it",
		Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	if _, err := e.db.NewInsert().Model(c).Exec(e.ctx); err != nil {
		t.Fatal(err)
	}
	return c
}

// snapshot22and10 is a price snapshot of 100.00: 61.00 at 22% and 39.00 at
// 10%, VAT-inclusive.
const snapshot22and10 = `{"total":10000,"vat":[{"rate":22,"taxable":5000,"tax":1100},{"rate":10,"taxable":3545,"tax":355}]}`

// booking inserts a booking of estA (status, total, deposit due).
func (e *env) booking(t testing.TB, status string, total, deposit money.Cents, snapshot string) *store.Booking {
	t.Helper()
	n := e.seq.Add(1)
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: e.estA.ID, Code: fmt.Sprintf("B%06d", n), CustomerID: e.custA.ID,
		Channel: "desk", Status: status, StartsOn: day0, EndsOn: day0, Slots: calendar.SlotsFull, Formula: "day",
		PriceSnapshot: store.JSON(snapshot), TotalAmount: total, DepositDue: deposit, PolicySnapshot: store.JSON("{}"),
		Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	if status == "held" || status == "pending_payment" {
		exp := t0.Add(time.Hour)
		b.HoldExpiresAt = &exp
	}
	if _, err := e.db.NewInsert().Model(b).Exec(e.ctx); err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) reload(t testing.TB, id string) *store.Booking {
	t.Helper()
	b, err := loadBooking(e.ctx, e.db, e.esA, id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) openSession(t testing.TB, float money.Cents) *store.CashSession {
	t.Helper()
	cs, err := e.cash.Open(e.ctx, e.owner, "", e.estA.ID, cash.OpenInput{OpeningFloat: float})
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func (e *env) count(t testing.TB, table, where string, args ...any) int {
	t.Helper()
	n, err := e.db.NewSelect().TableExpr(table).Where(where, args...).Count(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

// assertLedger checks INV-22 for b: paid_amount equals the succeeded
// payments minus the succeeded refunds.
func (e *env) assertLedger(t testing.TB, bookingID string) money.Cents {
	t.Helper()
	b := e.reload(t, bookingID)
	totals, err := store.BookingPaymentTotals(e.ctx, e.db, e.esA, bookingID)
	if err != nil {
		t.Fatal(err)
	}
	if b.PaidAmount != totals.Net() {
		t.Fatalf("INV-22: paid_amount %d != ledger %d (in %d, refunded %d)", b.PaidAmount, totals.Net(), totals.In, totals.Refunded)
	}
	if totals.Refunded > totals.In {
		t.Fatalf("INV-24: refunded %d > paid %d", totals.Refunded, totals.In)
	}
	return b.PaidAmount
}
