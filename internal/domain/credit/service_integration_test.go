//go:build integration

package credit_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// t0 is 2027-07-15 10:00 in Rome: business date 2027-07-15.
var t0 = time.Date(2027, 7, 15, 8, 0, 0, 0, time.UTC)

type env struct {
	ctx    context.Context
	db     *store.DB
	clock  *clock.Fake
	svc    *credit.Service
	est    *store.Establishment
	es     store.EstablishmentScope
	owner  *auth.Principal
	viewer *auth.Principal
	ownerB *auth.Principal
	cust   *store.Customer
	seq    atomic.Int64
}

func newEnv(t testing.TB, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", t0)
	if err != nil {
		t.Fatal(err)
	}
	orgB, _, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(t0)
	e := &env{ctx: ctx, db: db, clock: c, est: est}
	e.svc = credit.NewService(credit.Deps{DB: db, Clock: c})
	if e.es, err = store.SystemEstablishmentScope(ctx, db, est.ID); err != nil {
		t.Fatal(err)
	}
	e.owner = e.user(t, org.ID, "", auth.RoleOwner)
	e.viewer = e.user(t, org.ID, est.ID, auth.RoleViewer)
	e.ownerB = e.user(t, orgB.ID, "", auth.RoleOwner)
	e.cust = e.customer(t)
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

func (e *env) customer(t testing.TB) *store.Customer {
	t.Helper()
	c := &store.Customer{
		ID: store.NewID(), EstablishmentID: e.est.ID, FirstName: "Mario", LastName: "Rossi", Locale: "it",
		Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	if _, err := e.db.NewInsert().Model(c).Exec(e.ctx); err != nil {
		t.Fatal(err)
	}
	return c
}

// booking inserts a confirmed booking of the customer with a policy snapshot.
func (e *env) booking(t testing.TB, policy string) *store.Booking {
	t.Helper()
	day := calendar.MustParse("2027-07-20")
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: e.est.ID, Code: fmt.Sprintf("B%06d", e.seq.Add(1)), CustomerID: e.cust.ID,
		Channel: "desk", Status: "confirmed", StartsOn: day, EndsOn: day, Slots: calendar.SlotsFull, Formula: "day",
		PriceSnapshot: store.JSON(`{"total":10000}`), TotalAmount: 10000, PolicySnapshot: store.JSON(policy),
		Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	if _, err := e.db.NewInsert().Model(b).Exec(e.ctx); err != nil {
		t.Fatal(err)
	}
	return b
}

// move writes a movement through the ledger port in its own transaction.
func (e *env) move(t testing.TB, amount money.Cents, source string, mutate ...func(*store.CustomerCredit)) (*store.CustomerCredit, error) {
	t.Helper()
	c := &store.CustomerCredit{CustomerID: e.cust.ID, Amount: amount, Source: source}
	for _, m := range mutate {
		m(c)
	}
	err := store.RunInTx(e.ctx, e.db, func(ctx context.Context, tx bun.Tx) error {
		return e.svc.GrantCredit(ctx, tx, e.es, c)
	})
	return c, err
}

// assertBalance checks the invariant balance = SUM(ledger) >= 0 and the
// stored balance against want.
func (e *env) assertBalance(t testing.TB, want money.Cents) {
	t.Helper()
	var sum int64
	if err := e.db.NewSelect().TableExpr("customer_credits").ColumnExpr("COALESCE(SUM(amount), 0)").
		Where("customer_id = ?", e.cust.ID).Scan(e.ctx, &sum); err != nil {
		t.Fatal(err)
	}
	bal, err := store.GetCustomerCreditBalance(e.ctx, e.db, e.es, e.cust.ID)
	if err != nil {
		t.Fatal(err)
	}
	if int64(bal) != sum || bal < 0 {
		t.Fatalf("balance %d != ledger sum %d (or negative)", bal, sum)
	}
	if bal != want {
		t.Fatalf("balance %d, want %d", bal, want)
	}
}

func (e *env) events(t testing.TB, typ string) int {
	t.Helper()
	n, err := e.db.NewSelect().TableExpr("outbox_events").Where("type = ?", typ).Count(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func TestCreditLedger_GrantSpendExpire(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		exp := calendar.MustParse("2027-07-25")
		g, err := e.svc.Grant(e.ctx, e.owner, e.est.ID, e.cust.ID, credit.GrantInput{Amount: 5000, Source: store.CreditSourceGoodwill, ExpiresOn: &exp, Note: "Pioggia"})
		if err != nil {
			t.Fatal(err)
		}
		if g.CreatedByUserID == nil || *g.CreatedByUserID != e.owner.UserID {
			t.Fatalf("created by %v", g.CreatedByUserID)
		}
		if _, err := e.svc.Grant(e.ctx, e.owner, e.est.ID, e.cust.ID, credit.GrantInput{Amount: 3000, Source: store.CreditSourceManual}); err != nil {
			t.Fatal(err)
		}
		e.assertBalance(t, 8000)
		// Spend 2000: the expiring lot goes first.
		if _, err := e.move(t, -2000, store.CreditSourceRedemption); err != nil {
			t.Fatal(err)
		}
		l, err := e.svc.Ledger(e.ctx, e.viewer, e.est.ID, e.cust.ID, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		if l.Balance != 6000 || l.Available != 6000 || len(l.Expiring) != 1 || l.Expiring[0].Amount != 3000 || len(l.Items.Items) != 3 {
			t.Fatalf("ledger %+v", l)
		}
		// After the expiry date the remaining 3000 are no longer spendable.
		e.clock.Set(t0.AddDate(0, 0, 11))
		avail, err := e.svc.CreditBalance(e.ctx, e.db, e.es, e.cust.ID, calendar.MustParse("2027-07-26"))
		if err != nil || avail != 3000 {
			t.Fatalf("available %d %v", avail, err)
		}
		n, err := e.svc.ExpireAll(e.ctx)
		if err != nil || n != 1 {
			t.Fatalf("expire all %d %v", n, err)
		}
		if n, err := e.svc.ExpireAll(e.ctx); err != nil || n != 0 {
			t.Fatalf("expire all again %d %v", n, err)
		}
		e.assertBalance(t, 3000)
		var row store.CustomerCredit
		if err := db.NewSelect().Model(&row).Where("cc.source = ?", store.CreditSourceExpiry).Scan(e.ctx); err != nil {
			t.Fatal(err)
		}
		if row.Amount != -3000 || row.ReferenceCreditID == nil || *row.ReferenceCreditID != g.ID {
			t.Fatalf("expiry row %+v", row)
		}
		if e.events(t, event.CreditIssued) != 2 || e.events(t, event.CreditConsumed) != 1 || e.events(t, event.CreditExpired) != 1 {
			t.Fatalf("events issued %d consumed %d expired %d", e.events(t, event.CreditIssued), e.events(t, event.CreditConsumed), e.events(t, event.CreditExpired))
		}
	})
}

func TestCreditLedger_NeverNegative(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		if _, err := e.move(t, 1000, store.CreditSourceManual); err != nil {
			t.Fatal(err)
		}
		_, err := e.svc.Grant(e.ctx, e.owner, e.est.ID, e.cust.ID, credit.GrantInput{Amount: -1500, Source: store.CreditSourceAdjustment})
		if !errors.Is(err, credit.ErrInsufficient) {
			t.Fatalf("negative adjustment: %v", err)
		}
		if _, err := e.move(t, -1001, store.CreditSourceRedemption); !errors.Is(err, credit.ErrInsufficient) {
			t.Fatalf("overdraft: %v", err)
		}
		if _, err := e.move(t, -1000, store.CreditSourceRedemption); err != nil {
			t.Fatal(err)
		}
		e.assertBalance(t, 0)
		if n, _ := db.NewSelect().TableExpr("customer_credits").Count(e.ctx); n != 2 {
			t.Fatalf("rows %d (refused movements must not be written)", n)
		}
	})
}

func TestCreditLedger_ReversalCappedAtBalance(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		if _, err := e.move(t, 3000, store.CreditSourceSeasonRelease); err != nil {
			t.Fatal(err)
		}
		if _, err := e.move(t, -2000, store.CreditSourceRedemption); err != nil {
			t.Fatal(err)
		}
		c, err := e.move(t, -3000, store.CreditSourceSeasonRelease)
		if err != nil {
			t.Fatal(err)
		}
		if c.Amount != -1000 {
			t.Fatalf("reversal %d, want capped at -1000", c.Amount)
		}
		e.assertBalance(t, 0)
		// Nothing left: the next reversal writes nothing and does not fail.
		if _, err := e.move(t, -500, store.CreditSourceCancellation); err != nil {
			t.Fatal(err)
		}
		e.assertBalance(t, 0)
	})
}

func TestCreditLedger_PolicyValidity(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, `{"version":1,"rules":{"credit_validity_days":30}}`)
		c, err := e.move(t, 2500, store.CreditSourceCancellation, func(c *store.CustomerCredit) { c.BookingID = &b.ID })
		if err != nil {
			t.Fatal(err)
		}
		if c.ExpiresOn == nil || c.ExpiresOn.String() != "2027-08-14" {
			t.Fatalf("expires on %v, want 2027-08-14", c.ExpiresOn)
		}
		b2 := e.booking(t, `{}`)
		c2, err := e.move(t, 500, store.CreditSourceCancellation, func(c *store.CustomerCredit) { c.BookingID = &b2.ID })
		if err != nil || c2.ExpiresOn != nil {
			t.Fatalf("no validity: %v %v", c2.ExpiresOn, err)
		}
	})
}

func TestCreditLedger_Validation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		past := calendar.MustParse("2027-07-01")
		cases := []credit.GrantInput{
			{Amount: 0, Source: store.CreditSourceGoodwill},
			{Amount: -100, Source: store.CreditSourceGoodwill},
			{Amount: 100, Source: store.CreditSourceRedemption},
			{Amount: 100, Source: store.CreditSourceGoodwill, ExpiresOn: &past},
		}
		for i, in := range cases {
			if _, err := e.svc.Grant(e.ctx, e.owner, e.est.ID, e.cust.ID, in); !errors.Is(err, configerr.ErrValidation) {
				t.Fatalf("case %d: %v", i, err)
			}
		}
		if _, err := e.svc.Grant(e.ctx, e.viewer, e.est.ID, e.cust.ID, credit.GrantInput{Amount: 100, Source: store.CreditSourceGoodwill}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer: %v", err)
		}
		if _, err := e.svc.Ledger(e.ctx, e.ownerB, e.est.ID, e.cust.ID, "", 10); !errors.Is(err, configerr.ErrNotFound) && !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("other organization: %v", err)
		}
		if _, err := e.svc.Grant(e.ctx, e.owner, e.est.ID, store.NewID(), credit.GrantInput{Amount: 100, Source: store.CreditSourceGoodwill}); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("unknown customer: %v", err)
		}
		if _, err := e.move(t, -100, store.CreditSourceExpiry); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("expiry through the port: %v", err)
		}
	})
}

// TestConcurrency_CreditSpend: 100 parallel debits of 1000 on a balance of
// 5000 never overdraw it: exactly five succeed, the others get
// ErrInsufficient, and the stored balance equals the ledger sum (0).
func TestConcurrency_CreditSpend(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		if _, err := e.move(t, 5000, store.CreditSourceGoodwill); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var ok, refused atomic.Int32
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var err error
				for attempt := 0; attempt < 50; attempt++ {
					_, err = e.move(t, -1000, store.CreditSourceRedemption)
					if err == nil || errors.Is(err, credit.ErrInsufficient) || !retryable(err) {
						break
					}
					time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				}
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, credit.ErrInsufficient):
					refused.Add(1)
				default:
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 5 || refused.Load() != 95 {
			t.Fatalf("ok %d refused %d", ok.Load(), refused.Load())
		}
		e.assertBalance(t, 0)
	})
}

// retryable reports a transient lock error of the file-backed SQLite
// database (single writer) under heavy contention.
func retryable(err error) bool {
	msg := err.Error()
	return containsAny(msg, "database is locked", "SQLITE_BUSY", "deadlock detected", "could not serialize")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}


// TestCreditLedger_ExpireAllSkipsConsumedGrants (INV-70): a grant fully
// used before its expiry never gets an expiry row, so more than one batch
// of such customers must not starve the real expiries that sort after
// them; a customer whose expiry cannot be written (legacy drift between the
// aggregate and the ledger) is skipped and reported, never blocks the rest.
func TestCreditLedger_ExpireAllSkipsConsumedGrants(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		exp := calendar.MustParse("2027-07-01")
		mk := func(name string) *store.Customer {
			return &store.Customer{
				ID: store.NewID(), EstablishmentID: e.est.ID, FirstName: name, LastName: "Scaduto", Locale: "it",
				Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		const consumed = 1100 // > expireBatch
		var customers []*store.Customer
		var rows []store.CustomerCredit
		var balances []store.CustomerCreditBalance
		for i := range consumed {
			c := mk(fmt.Sprintf("C%04d", i))
			customers = append(customers, c)
			g := store.CustomerCredit{ID: store.NewID(), EstablishmentID: e.est.ID, CustomerID: c.ID, Amount: 1000, Source: store.CreditSourceGoodwill, ExpiresOn: &exp, CreatedAt: t0.AddDate(0, -2, 0)}
			r := store.CustomerCredit{ID: store.NewID(), EstablishmentID: e.est.ID, CustomerID: c.ID, Amount: -1000, Source: store.CreditSourceRedemption, CreatedAt: t0.AddDate(0, -1, 0)}
			rows = append(rows, g, r)
			balances = append(balances, store.CustomerCreditBalance{EstablishmentID: e.est.ID, CustomerID: c.ID, Balance: 0, UpdatedAt: t0})
		}
		// Drift: a balance of 100 with an expired grant of 500 left in the
		// ledger (written before 00008): the expiry row cannot apply.
		drift := mk("Drift")
		customers = append(customers, drift)
		rows = append(rows, store.CustomerCredit{ID: store.NewID(), EstablishmentID: e.est.ID, CustomerID: drift.ID, Amount: 500, Source: store.CreditSourceManual, ExpiresOn: &exp, CreatedAt: t0.AddDate(0, -2, 0)})
		balances = append(balances, store.CustomerCreditBalance{EstablishmentID: e.est.ID, CustomerID: drift.ID, Balance: 100, UpdatedAt: t0})
		// The real expiry, sorting last: 700 of 1000 left.
		last := mk("Ultimo")
		customers = append(customers, last)
		rows = append(rows,
			store.CustomerCredit{ID: store.NewID(), EstablishmentID: e.est.ID, CustomerID: last.ID, Amount: 1000, Source: store.CreditSourceGoodwill, ExpiresOn: &exp, CreatedAt: t0.AddDate(0, -2, 0)},
			store.CustomerCredit{ID: store.NewID(), EstablishmentID: e.est.ID, CustomerID: last.ID, Amount: -300, Source: store.CreditSourceRedemption, CreatedAt: t0.AddDate(0, -1, 0)},
		)
		balances = append(balances, store.CustomerCreditBalance{EstablishmentID: e.est.ID, CustomerID: last.ID, Balance: 700, UpdatedAt: t0})
		for _, batch := range [][]*store.Customer{customers[:600], customers[600:]} {
			if _, err := db.NewInsert().Model(&batch).Exec(e.ctx); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < len(rows); i += 500 {
			chunk := rows[i:min(i+500, len(rows))]
			if _, err := db.NewInsert().Model(&chunk).Exec(e.ctx); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < len(balances); i += 500 {
			chunk := balances[i:min(i+500, len(balances))]
			if _, err := db.NewInsert().Model(&chunk).Exec(e.ctx); err != nil {
				t.Fatal(err)
			}
		}
		n, err := e.svc.ExpireAll(e.ctx)
		if n != 1 || err == nil || !errors.Is(err, credit.ErrInsufficient) || !strings.Contains(err.Error(), "1 customers failed") {
			t.Fatalf("expire all: touched %d, err %v", n, err)
		}
		var row store.CustomerCredit
		if err := db.NewSelect().Model(&row).Where("cc.source = ?", store.CreditSourceExpiry).Where("cc.customer_id = ?", last.ID).Scan(e.ctx); err != nil || row.Amount != -700 {
			t.Fatalf("expiry of the last customer: %+v %v", row, err)
		}
		if n := e.count(t, store.CreditSourceExpiry); n != 1 {
			t.Fatalf("expiry rows %d", n)
		}
		// Only the drifted customer is reported again; nothing else changes.
		if n, err := e.svc.ExpireAll(e.ctx); n != 0 || err == nil {
			t.Fatalf("second run: touched %d, err %v", n, err)
		}
	})
}

func (e *env) count(t testing.TB, source string) int {
	t.Helper()
	n, err := e.db.NewSelect().TableExpr("customer_credits").Where("source = ?", source).Count(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}
