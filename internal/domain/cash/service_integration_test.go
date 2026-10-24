//go:build integration

package cash_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type bunTx = bun.Tx

var t0 = time.Date(2027, 7, 15, 6, 30, 0, 0, time.UTC) // 08:30 in Rome

type env struct {
	ctx     context.Context
	db      *store.DB
	clock   *clock.Fake
	svc     *cash.Service
	estA    *store.Establishment
	esA     store.EstablishmentScope
	owner   *auth.Principal
	cashier *auth.Principal
	viewer  *auth.Principal
	staff   *auth.Principal
	ownerB  *auth.Principal
	n       atomic.Int64
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", t0)
	if err != nil {
		t.Fatal(err)
	}
	orgB, _, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(t0)
	e := &env{ctx: ctx, db: db, clock: c, estA: estA}
	e.svc = cash.NewService(cash.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)})
	e.owner = e.user(t, orgA.ID, "", auth.RoleOwner)
	e.cashier = e.user(t, orgA.ID, estA.ID, auth.RoleCashier)
	e.viewer = e.user(t, orgA.ID, estA.ID, auth.RoleViewer)
	e.staff = e.user(t, orgA.ID, estA.ID, auth.RoleBeachStaff)
	e.ownerB = e.user(t, orgB.ID, "", auth.RoleOwner)
	if e.esA, err = store.SystemEstablishmentScope(ctx, db, estA.ID); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) user(t *testing.T, orgID, estID string, role auth.Role) *auth.Principal {
	t.Helper()
	u := &store.User{
		ID: store.NewID(), Email: fmt.Sprintf("u%d@lido.it", e.n.Add(1)), PasswordHash: "x", Name: "U",
		Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0,
	}
	if err := store.InsertUser(e.ctx, e.db, u); err != nil {
		t.Fatal(err)
	}
	return &auth.Principal{Kind: auth.PrincipalUser, UserID: u.ID, Grants: []auth.Grant{{OrganizationID: orgID, EstablishmentID: estID, Role: role}}}
}

// payment inserts a succeeded counter-sale payment attached to session.
func (e *env) payment(t *testing.T, sessionID, method, dir string, amount money.Cents) {
	t.Helper()
	taxable, tax := money.VATSplit(amount, 2200)
	p := &store.Payment{
		ID: store.NewID(), Provider: "manual", Method: method, Direction: dir, Amount: amount, Status: store.PaymentSucceeded,
		CashSessionID: &sessionID, VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: amount, Taxable: taxable, Tax: tax}},
		BusinessDate: calendar.MustParse("2027-07-15"), CreatedAt: t0, UpdatedAt: t0,
	}
	if err := store.InsertPayment(e.ctx, e.db, e.esA, p); err != nil {
		t.Fatal(err)
	}
}

func (e *env) count(t *testing.T, table, where string, args ...any) int {
	t.Helper()
	n, err := e.db.NewSelect().TableExpr(table).Where(where, args...).Count(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

// TestCashSession_CloseComputesDifference (INV-25): expected = float +
// session takings +/- movements; difference = counted - expected; the
// closed session refuses movements and payments.
func TestCashSession_CloseComputesDifference(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		cs, err := e.svc.Open(e.ctx, e.cashier, "10.0.0.1", e.estA.ID, cash.OpenInput{OpeningFloat: 10000, Notes: "turno mattina"})
		if err != nil {
			t.Fatal(err)
		}
		if cs.Register != store.DefaultCashRegister || cs.Version != 1 || cs.ExpectedTotals["cash"] != 10000 {
			t.Fatalf("open: %+v", cs)
		}
		if _, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{}); !errors.Is(err, cash.ErrSessionAlreadyOpen) {
			t.Fatalf("second open: %v", err)
		}
		// Another register can be opened in parallel.
		if _, err := e.svc.CreateRegister(e.ctx, e.owner, "", e.estA.ID, cash.RegisterInput{Name: "Bar", Code: ptr("bar")}); err != nil {
			t.Fatal(err)
		}
		bar, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{Register: "bar"})
		if err != nil {
			t.Fatal(err)
		}

		e.payment(t, cs.ID, store.PaymentMethodCash, store.PaymentDirectionIn, 4000)
		e.payment(t, cs.ID, store.PaymentMethodCard, store.PaymentDirectionIn, 6000)
		e.payment(t, cs.ID, store.PaymentMethodCash, store.PaymentDirectionRefund, 1000)
		e.payment(t, bar.ID, store.PaymentMethodCash, store.PaymentDirectionIn, 999)
		if _, err := e.svc.AddMovement(e.ctx, e.cashier, "", e.estA.ID, cs.ID, cash.MovementInput{Kind: store.CashMovementIn, Amount: 2000, Reason: "cambio"}); err != nil {
			t.Fatal(err)
		}
		// Withdrawal above the expected cash (100 + 40 - 10 + 20 = 150.00).
		var ve *configerr.ValidationError
		if _, err := e.svc.AddMovement(e.ctx, e.cashier, "", e.estA.ID, cs.ID, cash.MovementInput{Kind: store.CashMovementOut, Amount: 15001, Reason: "versamento"}); !errors.As(err, &ve) {
			t.Fatalf("withdrawal above cash: %v", err)
		}
		if _, err := e.svc.AddMovement(e.ctx, e.cashier, "", e.estA.ID, cs.ID, cash.MovementInput{Kind: store.CashMovementOut, Amount: 5000, Reason: "versamento banca"}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.AddMovement(e.ctx, e.cashier, "", e.estA.ID, cs.ID, cash.MovementInput{Kind: "x", Amount: 0}); !errors.As(err, &ve) || len(ve.Fields) != 3 {
			t.Fatalf("movement validation: %v", err)
		}

		cur, err := e.svc.Current(e.ctx, e.viewer, e.estA.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		if cur.Session.ExpectedTotals["cash"] != 10000 || cur.Session.ExpectedTotals["card"] != 6000 || len(cur.Payments) != 3 || len(cur.Movements) != 2 {
			t.Fatalf("current: %+v", cur.Session.ExpectedTotals)
		}
		if cur.Summary.Takings["cash"] != 3000 || cur.Summary.ByVAT[0].Gross != 9000 {
			t.Fatalf("summary: %+v", cur.Summary)
		}

		// If-Match mismatch: 412.
		stale := int64(7)
		var vc *configerr.VersionConflictError
		if _, err := e.svc.Close(e.ctx, e.cashier, "", e.estA.ID, cs.ID, &stale, cash.CloseInput{}); !errors.As(err, &vc) || vc.Current != 1 {
			t.Fatalf("stale close: %v", err)
		}
		e.clock.Advance(10 * time.Hour)
		v := int64(1)
		closed, err := e.svc.Close(e.ctx, e.cashier, "", e.estA.ID, cs.ID, &v, cash.CloseInput{
			Counted: store.MethodTotals{"cash": 9800, "card": 6000}, Notes: "manca 2 euro",
		})
		if err != nil {
			t.Fatal(err)
		}
		s := closed.Session
		if s.ClosedAt == nil || s.Version != 2 || s.Difference == nil || *s.Difference != -200 {
			t.Fatalf("closed: %+v", s)
		}
		if s.ExpectedTotals["cash"] != 10000 || s.CountedTotals["cash"] != 9800 || closed.Summary.Differences["cash"] != -200 {
			t.Fatalf("totals: %+v %+v", s.ExpectedTotals, s.CountedTotals)
		}
		if s.Notes != "turno mattina\nmanca 2 euro" {
			t.Fatalf("notes %q", s.Notes)
		}

		// Stored values survive a reload (INV-25 on the persisted row).
		got, err := e.svc.Get(e.ctx, e.owner, e.estA.ID, cs.ID)
		if err != nil {
			t.Fatal(err)
		}
		if *got.Session.Difference != got.Session.CountedTotals.Sum()-got.Session.ExpectedTotals.Sum() {
			t.Fatalf("persisted difference %d", *got.Session.Difference)
		}

		// Closed: movements, second close and payments are refused.
		if _, err := e.svc.AddMovement(e.ctx, e.cashier, "", e.estA.ID, cs.ID, cash.MovementInput{Kind: "in", Amount: 1, Reason: "x"}); !errors.Is(err, cash.ErrSessionClosed) {
			t.Fatalf("movement on closed: %v", err)
		}
		if _, err := e.svc.Close(e.ctx, e.cashier, "", e.estA.ID, cs.ID, nil, cash.CloseInput{}); !errors.Is(err, cash.ErrSessionClosed) {
			t.Fatalf("second close: %v", err)
		}
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bunTx) error {
			_, err := e.svc.SessionForPayment(ctx, tx, e.esA, &cs.ID, store.PaymentMethodCard)
			return err
		})
		if !errors.Is(err, cash.ErrSessionClosed) {
			t.Fatalf("payment on closed session: %v", err)
		}
		// Without a main session, cash fails and card goes without session.
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bunTx) error {
			if _, err := e.svc.SessionForPayment(ctx, tx, e.esA, nil, store.PaymentMethodCash); !errors.Is(err, cash.ErrSessionClosed) {
				return fmt.Errorf("cash: %w", err)
			}
			id, err := e.svc.SessionForPayment(ctx, tx, e.esA, nil, store.PaymentMethodCard)
			if err != nil || id != nil {
				return fmt.Errorf("card: %v %w", id, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// A new session can be opened after closing.
		if _, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{OpeningFloat: 9800}); err != nil {
			t.Fatal(err)
		}

		// Audit + outbox for open (3), movements (2) and close (1).
		if n := e.count(t, "outbox_events", "type = ?", event.CashSessionOpened); n != 3 {
			t.Fatalf("opened events %d", n)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.CashMovementRecorded); n != 2 {
			t.Fatalf("movement events %d", n)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.CashSessionClosed); n != 1 {
			t.Fatalf("closed events %d", n)
		}
		if n := e.count(t, "audit_log", "entity_type = 'cash_session'"); n != 6 {
			t.Fatalf("audit rows %d", n)
		}

		// History: newest first, filters by status, register and dates.
		closedOnly := cash.ListFilter{Status: cash.StatusClosed}
		page, err := e.svc.List(e.ctx, e.viewer, e.estA.ID, closedOnly, "", 10)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != cs.ID {
			t.Fatalf("closed history: %+v %v", page.Items, err)
		}
		all, err := e.svc.List(e.ctx, e.owner, e.estA.ID, cash.ListFilter{}, "", 2)
		if err != nil || len(all.Items) != 2 || all.NextCursor == "" {
			t.Fatalf("history page: %d %v", len(all.Items), err)
		}
		next, err := e.svc.List(e.ctx, e.owner, e.estA.ID, cash.ListFilter{}, all.NextCursor, 2)
		if err != nil || len(next.Items) != 1 || next.Items[0].ID != cs.ID {
			t.Fatalf("history next: %+v %v", next.Items, err)
		}
		d := calendar.MustParse("2027-07-14")
		if p, _ := e.svc.List(e.ctx, e.owner, e.estA.ID, cash.ListFilter{To: &d}, "", 10); len(p.Items) != 0 {
			t.Fatalf("to filter: %d", len(p.Items))
		}
		if p, _ := e.svc.List(e.ctx, e.owner, e.estA.ID, cash.ListFilter{Register: "bar"}, "", 10); len(p.Items) != 1 || p.Items[0].ExpectedTotals["cash"] != 999 {
			t.Fatalf("register filter: %+v", p.Items)
		}
	})
}

// TestCashService_Permissions: cashier operates, viewer reads the history,
// beach staff has no access, another tenant sees nothing.
func TestCashService_Permissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		if _, err := e.svc.Open(e.ctx, e.viewer, "", e.estA.ID, cash.OpenInput{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer open: %v", err)
		}
		cs, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Get(e.ctx, e.viewer, e.estA.ID, cs.ID); err != nil {
			t.Fatalf("viewer read: %v", err)
		}
		if _, err := e.svc.Get(e.ctx, e.staff, e.estA.ID, cs.ID); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("staff read: %v", err)
		}
		if _, err := e.svc.Close(e.ctx, e.viewer, "", e.estA.ID, cs.ID, nil, cash.CloseInput{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer close: %v", err)
		}
		if _, err := e.svc.Get(e.ctx, e.ownerB, e.estA.ID, cs.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		if _, err := e.svc.Get(e.ctx, e.owner, e.estA.ID, store.NewID()); !errors.Is(err, cash.ErrNotFound) {
			t.Fatalf("unknown session: %v", err)
		}
		if _, err := e.svc.Current(e.ctx, e.owner, e.estA.ID, "bar"); !errors.Is(err, cash.ErrNotFound) {
			t.Fatalf("no current: %v", err)
		}
		var ve *configerr.ValidationError
		if _, err := e.svc.Open(e.ctx, e.owner, "", e.estA.ID, cash.OpenInput{Register: "Bar Centrale", OpeningFloat: -1}); !errors.As(err, &ve) || len(ve.Fields) != 2 {
			t.Fatalf("open validation: %v", err)
		}
		if _, err := e.svc.Close(e.ctx, e.owner, "", e.estA.ID, cs.ID, nil, cash.CloseInput{Counted: store.MethodTotals{"bitcoin": 1}}); !errors.As(err, &ve) {
			t.Fatalf("close validation: %v", err)
		}
	})
}

// TestConcurrency_OpenCashSession: 100 parallel openings of the same
// register leave exactly one open session.
func TestConcurrency_OpenCashSession(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		var wg sync.WaitGroup
		var opened, refused atomic.Int32
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{OpeningFloat: 5000})
				switch {
				case err == nil:
					opened.Add(1)
				case errors.Is(err, cash.ErrSessionAlreadyOpen):
					refused.Add(1)
				default:
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if opened.Load() != 1 || refused.Load() != 99 {
			t.Fatalf("opened %d refused %d", opened.Load(), refused.Load())
		}
		if n := e.count(t, "cash_sessions", "establishment_id = ? AND closed_at IS NULL", e.estA.ID); n != 1 {
			t.Fatalf("open sessions %d", n)
		}
	})
}

// TestConcurrency_CloseWhilePaying: payments attached while the session is
// being closed either land before the close (and are in its expected
// totals) or are refused with ErrSessionClosed; INV-25 holds on the
// persisted row.
func TestConcurrency_CloseWhilePaying(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		cs, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var landed atomic.Int64
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if i == 50 {
					if _, err := e.svc.Close(e.ctx, e.cashier, "", e.estA.ID, cs.ID, nil, cash.CloseInput{}); err != nil {
						t.Error(err)
					}
					return
				}
				err := store.RunInTx(e.ctx, db, func(ctx context.Context, tx bunTx) error {
					id, err := e.svc.SessionForPayment(ctx, tx, e.esA, &cs.ID, store.PaymentMethodCash)
					if err != nil {
						return err
					}
					p := &store.Payment{
						ID: store.NewID(), Provider: "manual", Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn,
						Amount: 100, Status: store.PaymentSucceeded, CashSessionID: id, BusinessDate: calendar.MustParse("2027-07-15"),
						CreatedAt: t0, UpdatedAt: t0,
					}
					return store.InsertPayment(ctx, tx, e.esA, p)
				})
				switch {
				case err == nil:
					landed.Add(100)
				case errors.Is(err, cash.ErrSessionClosed):
				default:
					t.Error(err)
				}
			}(i)
		}
		wg.Wait()
		got, err := e.svc.Get(e.ctx, e.owner, e.estA.ID, cs.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Session.ExpectedTotals["cash"] != money.Cents(landed.Load()) || got.Summary.Takings["cash"] != money.Cents(landed.Load()) {
			t.Fatalf("expected %d, takings %d, landed %d", got.Session.ExpectedTotals["cash"], got.Summary.Takings["cash"], landed.Load())
		}
	})
}
