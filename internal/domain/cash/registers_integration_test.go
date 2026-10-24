//go:build integration

package cash_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func ptr[T any](v T) *T { return &v }

// TestCashRegisters_CRUD: the default register is created lazily, codes are
// derived from the name and unique, the default register cannot be
// deactivated or deleted, a register with an open session cannot be
// deactivated, a used register cannot be deleted; configure permission.
func TestCashRegisters_CRUD(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		regs, err := e.svc.ListRegisters(e.ctx, e.cashier, e.estA.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(regs) != 1 || regs[0].Code != store.DefaultCashRegister || !regs[0].IsDefault || regs[0].Name != store.DefaultCashRegisterName {
			t.Fatalf("default register: %+v", regs)
		}
		def := regs[0]
		// Listing again does not create a second default.
		if again, _ := e.svc.ListRegisters(e.ctx, e.owner, e.estA.ID, true); len(again) != 1 {
			t.Fatalf("registers after second list: %d", len(again))
		}

		if _, err := e.svc.CreateRegister(e.ctx, e.cashier, "", e.estA.ID, cash.RegisterInput{Name: "Bar"}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier create: %v", err)
		}
		bar, err := e.svc.CreateRegister(e.ctx, e.owner, "", e.estA.ID, cash.RegisterInput{Name: "Bar Spiaggia è"})
		if err != nil {
			t.Fatal(err)
		}
		if bar.Code != "bar-spiaggia-e" || !bar.Active || bar.IsDefault || bar.Version != 1 {
			t.Fatalf("bar: %+v", bar.CashRegister)
		}
		bar2, err := e.svc.CreateRegister(e.ctx, e.owner, "", e.estA.ID, cash.RegisterInput{Name: "Bar spiaggia E"})
		if err != nil || bar2.Code != "bar-spiaggia-e-2" {
			t.Fatalf("derived code suffix: %+v %v", bar2, err)
		}
		if _, err := e.svc.CreateRegister(e.ctx, e.owner, "", e.estA.ID, cash.RegisterInput{Name: "X", Code: ptr("bar-spiaggia-e")}); !errors.Is(err, cash.ErrRegisterCodeTaken) {
			t.Fatalf("explicit duplicate code: %v", err)
		}
		var ve *configerr.ValidationError
		if _, err := e.svc.CreateRegister(e.ctx, e.owner, "", e.estA.ID, cash.RegisterInput{Name: " ", Code: ptr("Bad Code"), SortOrder: ptr(-1)}); !errors.As(err, &ve) || len(ve.Fields) != 3 {
			t.Fatalf("create validation: %v", err)
		}

		// Default register: never deactivated, never deleted.
		if _, err := e.svc.UpdateRegister(e.ctx, e.owner, "", e.estA.ID, def.ID, nil, cash.RegisterInput{Name: "Cassa", Active: ptr(false)}); !errors.Is(err, cash.ErrDefaultRegister) {
			t.Fatalf("deactivate default: %v", err)
		}
		if err := e.svc.DeleteRegister(e.ctx, e.owner, "", e.estA.ID, def.ID, nil); !errors.Is(err, cash.ErrDefaultRegister) {
			t.Fatalf("delete default: %v", err)
		}
		renamed, err := e.svc.UpdateRegister(e.ctx, e.owner, "", e.estA.ID, def.ID, ptr(int64(1)), cash.RegisterInput{Name: "Cassa ingresso", SortOrder: ptr(5)})
		if err != nil || renamed.Name != "Cassa ingresso" || renamed.Version != 2 || renamed.Code != store.DefaultCashRegister {
			t.Fatalf("rename default: %+v %v", renamed, err)
		}
		var vc *configerr.VersionConflictError
		if _, err := e.svc.UpdateRegister(e.ctx, e.owner, "", e.estA.ID, def.ID, ptr(int64(1)), cash.RegisterInput{Name: "x"}); !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale update: %v", err)
		}

		// Open session on bar: cannot deactivate, cannot delete.
		cs, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{RegisterID: bar.ID, OpeningFloat: 1000})
		if err != nil {
			t.Fatal(err)
		}
		if cs.Register != bar.Code || cs.RegisterID == nil || *cs.RegisterID != bar.ID || cs.RegisterName != bar.Name {
			t.Fatalf("session register: %+v", cs)
		}
		if _, err := e.svc.UpdateRegister(e.ctx, e.owner, "", e.estA.ID, bar.ID, nil, cash.RegisterInput{Name: "Bar", Active: ptr(false)}); !errors.Is(err, cash.ErrSessionAlreadyOpen) {
			t.Fatalf("deactivate with open session: %v", err)
		}
		got, err := e.svc.GetRegister(e.ctx, e.viewer, e.estA.ID, bar.ID)
		if err != nil || !got.Active || got.OpenSessionID == nil || *got.OpenSessionID != cs.ID {
			t.Fatalf("bar after refused deactivation: %+v %v", got, err)
		}
		if err := e.svc.DeleteRegister(e.ctx, e.owner, "", e.estA.ID, bar.ID, nil); !errors.Is(err, cash.ErrRegisterHasSessions) {
			t.Fatalf("delete used: %v", err)
		}
		if _, err := e.svc.Close(e.ctx, e.cashier, "", e.estA.ID, cs.ID, nil, cash.CloseInput{Counted: store.MethodTotals{"cash": 1000}}); err != nil {
			t.Fatal(err)
		}
		off, err := e.svc.UpdateRegister(e.ctx, e.owner, "", e.estA.ID, bar.ID, nil, cash.RegisterInput{Name: "Bar", Active: ptr(false)})
		if err != nil || off.Active {
			t.Fatalf("deactivate closed: %+v %v", off, err)
		}
		// Inactive register: cannot be opened, hidden from the default list.
		if _, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{RegisterID: bar.ID}); !errors.As(err, &ve) {
			t.Fatalf("open inactive: %v", err)
		}
		if _, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{RegisterID: store.NewID()}); !errors.As(err, &ve) {
			t.Fatalf("open unknown: %v", err)
		}
		if _, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{Register: "nope"}); !errors.As(err, &ve) {
			t.Fatalf("open unknown code: %v", err)
		}
		if l, _ := e.svc.ListRegisters(e.ctx, e.cashier, e.estA.ID, false); len(l) != 2 {
			t.Fatalf("active registers %d", len(l))
		}
		if l, _ := e.svc.ListRegisters(e.ctx, e.cashier, e.estA.ID, true); len(l) != 3 {
			t.Fatalf("all registers %d", len(l))
		}
		// Unused register: deleted.
		if err := e.svc.DeleteRegister(e.ctx, e.owner, "", e.estA.ID, bar2.ID, ptr(int64(1))); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.GetRegister(e.ctx, e.owner, e.estA.ID, bar2.ID); !errors.Is(err, cash.ErrRegisterNotFound) {
			t.Fatalf("deleted register: %v", err)
		}
		// Another tenant sees nothing.
		if _, err := e.svc.GetRegister(e.ctx, e.ownerB, e.estA.ID, bar.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		if n := e.count(t, "audit_log", "entity_type = 'cash_register'"); n != 5 {
			t.Fatalf("register audit rows %d", n)
		}
	})
}

// TestCashRegisters_TwoOpenSessionsReconcile (INV-73): two registers are
// open at the same time; the database refuses a second open session on the
// same register even when written directly; the Z totals of each register
// only hold its own payments and the per-register corrispettivi add up to
// the total of the day.
func TestCashRegisters_TwoOpenSessionsReconcile(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		bar, err := e.svc.CreateRegister(e.ctx, e.owner, "", e.estA.ID, cash.RegisterInput{Name: "Bar"})
		if err != nil {
			t.Fatal(err)
		}
		mainS, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{OpeningFloat: 10000})
		if err != nil {
			t.Fatal(err)
		}
		barS, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{RegisterID: bar.ID, OpeningFloat: 5000})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, cash.OpenInput{RegisterID: bar.ID}); !errors.Is(err, cash.ErrSessionAlreadyOpen) {
			t.Fatalf("second open on bar: %v", err)
		}
		// DB constraint, bypassing the service: same register id, another code string.
		dup := &store.CashSession{
			ID: store.NewID(), Register: "bar-alias", RegisterID: &bar.ID, OpenedAt: t0, ExpectedTotals: store.MethodTotals{},
			Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		if err := store.InsertCashSession(e.ctx, db, e.esA, dup); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("db constraint: %v", err)
		}

		e.payment(t, mainS.ID, store.PaymentMethodCash, store.PaymentDirectionIn, 4000)
		e.payment(t, mainS.ID, store.PaymentMethodCard, store.PaymentDirectionIn, 6000)
		e.payment(t, barS.ID, store.PaymentMethodCash, store.PaymentDirectionIn, 1250)
		e.payment(t, barS.ID, store.PaymentMethodCash, store.PaymentDirectionRefund, 250)
		// A test-mode payment (INV-74) never reaches the Z.
		test := &store.Payment{
			ID: store.NewID(), Provider: "manual", Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn, Amount: 99999,
			Status: store.PaymentSucceeded, CashSessionID: &barS.ID, Test: true, BusinessDate: calendar.MustParse("2027-07-15"),
			CreatedAt: t0, UpdatedAt: t0,
		}
		if err := store.InsertPayment(e.ctx, db, e.esA, test); err != nil {
			t.Fatal(err)
		}

		cur, err := e.svc.CurrentOfRegister(e.ctx, e.viewer, e.estA.ID, bar.ID)
		if err != nil || cur.Session.ID != barS.ID || cur.Session.RegisterName != "Bar" {
			t.Fatalf("current of bar: %+v %v", cur, err)
		}
		if _, err := e.svc.CurrentOfRegister(e.ctx, e.viewer, e.estA.ID, store.NewID()); !errors.Is(err, cash.ErrNotFound) {
			t.Fatalf("current of unknown: %v", err)
		}

		e.clock.Advance(9 * time.Hour)
		mainZ, err := e.svc.Close(e.ctx, e.cashier, "", e.estA.ID, mainS.ID, nil, cash.CloseInput{Counted: store.MethodTotals{"cash": 14000, "card": 6000}})
		if err != nil {
			t.Fatal(err)
		}
		barZ, err := e.svc.Close(e.ctx, e.cashier, "", e.estA.ID, barS.ID, nil, cash.CloseInput{Counted: store.MethodTotals{"cash": 5900}})
		if err != nil {
			t.Fatal(err)
		}
		if mainZ.Summary.Takings.Sum() != 10000 || mainZ.Session.ExpectedTotals["cash"] != 14000 || *mainZ.Session.Difference != 0 {
			t.Fatalf("main Z: %+v", mainZ.Summary)
		}
		if barZ.Summary.Takings.Sum() != 1000 || barZ.Session.ExpectedTotals["cash"] != 6000 || *barZ.Session.Difference != -100 {
			t.Fatalf("bar Z: takings %+v expected %+v diff %d", barZ.Summary.Takings, barZ.Session.ExpectedTotals, *barZ.Session.Difference)
		}

		// History per register.
		if p, _ := e.svc.List(e.ctx, e.viewer, e.estA.ID, cash.ListFilter{RegisterID: bar.ID}, "", 10); len(p.Items) != 1 || p.Items[0].RegisterName != "Bar" {
			t.Fatalf("history of bar: %+v", p.Items)
		}

		// Printable Z data: names, establishment and summary.
		zd, err := e.svc.ZReport(e.ctx, e.viewer, e.estA.ID, barS.ID)
		if err != nil {
			t.Fatal(err)
		}
		if zd.Detail.Session.RegisterName != "Bar" || zd.OpenedByName != "U" || zd.ClosedByName != "U" || zd.Establishment.ID != e.estA.ID {
			t.Fatalf("z data: %+v", zd)
		}
		if _, err := e.svc.ZReport(e.ctx, e.staff, e.estA.ID, barS.ID); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("staff z: %v", err)
		}

		// Corrispettivi per register reconcile with the total of the day.
		fs := fiscal.NewService(fiscal.Deps{DB: db, Clock: e.clock, Audit: audit.NewWriter(e.clock), Outbox: event.NewOutbox(e.clock)})
		day := calendar.MustParse("2027-07-15")
		total := func(registerID string) money.Cents {
			t.Helper()
			rep, err := fs.Corrispettivi(e.ctx, e.owner, e.estA.ID, fiscal.CorrispettiviQuery{From: day, To: day, RegisterID: registerID})
			if err != nil {
				t.Fatal(err)
			}
			return rep.GrandTotal
		}
		all, m, b := total(""), total(defaultRegisterID(t, e)), total(bar.ID)
		if m != 10000 || b != 1000 || all != m+b {
			t.Fatalf("corrispettivi all %d main %d bar %d", all, m, b)
		}
		if _, err := fs.Corrispettivi(e.ctx, e.owner, e.estA.ID, fiscal.CorrispettiviQuery{From: day, To: day, RegisterID: store.NewID()}); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("unknown register: %v", err)
		}
	})
}

// defaultRegisterID resolves the id of the default register.
func defaultRegisterID(t *testing.T, e *env) string {
	t.Helper()
	r, err := store.GetCashRegisterByCode(e.ctx, e.db, e.esA, store.DefaultCashRegister)
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

// TestConcurrency_OpenSessionPerRegister (INV-73): 100 parallel openings on
// each of two registers leave exactly one open session per register.
func TestConcurrency_OpenSessionPerRegister(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		bar, err := e.svc.CreateRegister(e.ctx, e.owner, "", e.estA.ID, cash.RegisterInput{Name: "Bar"})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var opened, refused atomic.Int32
		for i := 0; i < 200; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				in := cash.OpenInput{OpeningFloat: 1000}
				if i%2 == 1 {
					in.RegisterID = bar.ID
				}
				_, err := e.svc.Open(e.ctx, e.cashier, "", e.estA.ID, in)
				switch {
				case err == nil:
					opened.Add(1)
				case errors.Is(err, cash.ErrSessionAlreadyOpen):
					refused.Add(1)
				default:
					t.Error(err)
				}
			}(i)
		}
		wg.Wait()
		if opened.Load() != 2 || refused.Load() != 198 {
			t.Fatalf("opened %d refused %d", opened.Load(), refused.Load())
		}
		if n := e.count(t, "cash_sessions", "establishment_id = ? AND closed_at IS NULL AND register_id = ?", e.estA.ID, bar.ID); n != 1 {
			t.Fatalf("open bar sessions %d", n)
		}
		if n := e.count(t, "cash_sessions", "establishment_id = ? AND closed_at IS NULL", e.estA.ID); n != 2 {
			t.Fatalf("open sessions %d", n)
		}
		if n := e.count(t, "cash_registers", "establishment_id = ? AND is_default = ?", e.estA.ID, true); n != 1 {
			t.Fatalf("default registers %d", n)
		}
	})
}
