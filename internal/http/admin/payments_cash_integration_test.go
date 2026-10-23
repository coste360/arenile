//go:build integration

package admin_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/app/builtin"
	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

type deskRegistry struct{ c clock.Clock }

func (deskRegistry) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	if code == builtin.ManualCode {
		return builtin.Manual{}, true
	}
	return nil, false
}

func (r deskRegistry) FiscalDriver(code string) (ext.FiscalDriver, bool) {
	if code == builtin.NoneCode {
		return builtin.NoneFiscal{Clock: r.c}, true
	}
	return nil, false
}

type deskEnv struct {
	h       *admin.Handlers
	est     *store.Establishment
	owner   *auth.Principal
	cashier *auth.Principal
	viewer  *auth.Principal
	ownerB  *auth.Principal // owner of another organization
	booking *store.Booking
}

func newDeskEnv(t *testing.T, db *store.DB) *deskEnv {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2027, 7, 15, 8, 0, 0, 0, time.UTC)
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
	if err != nil {
		t.Fatal(err)
	}
	orgB, _, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", now)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(now)
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	cashSvc := cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	fiscalSvc := fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: deskRegistry{c}})
	custSvc := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	paySvc := payment.NewService(payment.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
		Cash: cashSvc, Fiscal: fiscalSvc, Credits: custSvc, Providers: deskRegistry{c},
	})
	mkIn := func(orgID, estID string, role auth.Role) *auth.Principal {
		u := &store.User{ID: store.NewID(), Email: fmt.Sprintf("%s-%s@lido.it", role, store.NewID()), PasswordHash: "x", Name: "U", Locale: "it", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := store.InsertUser(ctx, db, u); err != nil {
			t.Fatal(err)
		}
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: u.ID, Grants: []auth.Grant{{OrganizationID: orgID, EstablishmentID: estID, Role: role}}}
	}
	mk := func(estID string, role auth.Role) *auth.Principal { return mkIn(org.ID, estID, role) }
	cu := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, LastName: "Rossi", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := db.NewInsert().Model(cu).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	day := calendar.MustParse("2027-07-15")
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "A1B2C3", CustomerID: cu.ID, Channel: "desk", Status: "confirmed",
		StartsOn: day, EndsOn: day, Slots: calendar.SlotsFull, Formula: "day", TotalAmount: 5000,
		PriceSnapshot: store.JSON(`{"vat":[{"rate":22,"taxable":4098,"tax":902}]}`), PolicySnapshot: store.JSON("{}"),
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := db.NewInsert().Model(b).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	return &deskEnv{
		h:     &admin.Handlers{Payments: paySvc, Cash: cashSvc, Fiscal: fiscalSvc, Log: slog.New(slog.DiscardHandler)},
		est:   est,
		owner: mk("", auth.RoleOwner), cashier: mk(est.ID, auth.RoleCashier), viewer: mk(est.ID, auth.RoleViewer),
		ownerB:  mkIn(orgB.ID, "", auth.RoleOwner),
		booking: b,
	}
}

func TestPaymentsCashHTTP_DeskDay(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newDeskEnv(t, db)
		est, bk := e.est.ID, e.booking.ID
		pay := func(p *auth.Principal, in api.ManualPaymentInput, key string) (int, map[string]any) {
			params := api.RecordBookingPaymentParams{}
			if key != "" {
				params.IdempotencyKey = &key
			}
			status, _, body := call(t, p, http.MethodPost, in, func(w http.ResponseWriter, r *http.Request) {
				e.h.RecordBookingPayment(w, r, est, bk, params)
			})
			return status, body
		}

		// Cash without a session: 409 cash_session_closed.
		if status, body := pay(e.cashier, api.ManualPaymentInput{Amount: 1000, Method: api.PaymentMethodCash}, ""); status != 409 || body["type"] != "cash_session_closed" {
			t.Fatalf("no session: %d %v", status, body)
		}
		// Open the desk.
		status, hdr, body := call(t, e.cashier, http.MethodPost, api.CashSessionOpen{OpeningFloat: 10000}, func(w http.ResponseWriter, r *http.Request) {
			e.h.OpenCashSession(w, r, est, api.OpenCashSessionParams{})
		})
		if status != 201 || hdr.Get("ETag") != `"1"` || body["status"] != "open" {
			t.Fatalf("open: %d %v", status, body)
		}
		sessionID := body["id"].(string)
		if status, _, body := call(t, e.cashier, http.MethodPost, api.CashSessionOpen{}, func(w http.ResponseWriter, r *http.Request) {
			e.h.OpenCashSession(w, r, est, api.OpenCashSessionParams{})
		}); status != 409 || body["type"] != "cash_session_already_open" {
			t.Fatalf("second open: %d %v", status, body)
		}

		// Payments: viewer forbidden, cash 30.00, then over the balance.
		if status, body := pay(e.viewer, api.ManualPaymentInput{Amount: 1000, Method: api.PaymentMethodCard}, ""); status != 403 {
			t.Fatalf("viewer: %d %v", status, body)
		}
		status, body = pay(e.cashier, api.ManualPaymentInput{Amount: 3000, Method: api.PaymentMethodCash, Note: strp("acconto")}, "k-1")
		if status != 201 || body["cash_session_id"] != sessionID || body["direction"] != "in" || body["provider"] != "manual" {
			t.Fatalf("cash payment: %d %v", status, body)
		}
		firstID := body["id"]
		if status, body := pay(e.cashier, api.ManualPaymentInput{Amount: 3000, Method: api.PaymentMethodCash, Note: strp("acconto")}, "k-1"); status != 201 || body["id"] != firstID {
			t.Fatalf("replay: %d %v", status, body)
		}
		if status, body := pay(e.cashier, api.ManualPaymentInput{Amount: 2001, Method: api.PaymentMethodCard}, ""); status != 400 || body["type"] != "validation_failed" {
			t.Fatalf("over balance: %d %v", status, body)
		}
		if status, _ := pay(e.cashier, api.ManualPaymentInput{Amount: 2000, Method: api.PaymentMethodCard}, ""); status != 201 {
			t.Fatalf("card: %d", status)
		}

		// Refund: cashier forbidden, owner over paid 400, owner ok 201.
		refund := func(p *auth.Principal, in api.RefundInput) (int, map[string]any) {
			status, _, body := call(t, p, http.MethodPost, in, func(w http.ResponseWriter, r *http.Request) {
				e.h.RecordBookingRefund(w, r, est, bk, api.RecordBookingRefundParams{})
			})
			return status, body
		}
		if status, _ := refund(e.cashier, api.RefundInput{Amount: 100, Method: api.PaymentMethodCash}); status != 403 {
			t.Fatalf("cashier refund: %d", status)
		}
		if status, body := refund(e.owner, api.RefundInput{Amount: 5001, Method: api.PaymentMethodCash}); status != 400 {
			t.Fatalf("refund over paid: %d %v", status, body)
		}
		if status, body := refund(e.owner, api.RefundInput{Amount: 500, Method: api.PaymentMethodCash, Reason: strp("ombrellone rotto")}); status != 201 || body["direction"] != "refund" {
			t.Fatalf("refund: %d %v", status, body)
		}

		// Counter sale at 10%.
		rate := int64(1000)
		status, _, body = call(t, e.cashier, http.MethodPost, api.CounterSaleInput{Amount: 1100, Method: api.PaymentMethodCash, Description: "Lettino extra", VatRateBp: &rate},
			func(w http.ResponseWriter, r *http.Request) {
				e.h.RecordCounterSale(w, r, est, api.RecordCounterSaleParams{})
			})
		if status != 201 || body["booking_id"] != nil {
			t.Fatalf("counter sale: %d %v", status, body)
		}

		// Booking ledger and establishment list.
		status, _, body = call(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.ListBookingPayments(w, r, est, bk) })
		if status != 200 || len(body["items"].([]any)) != 3 {
			t.Fatalf("booking payments: %d %v", status, body)
		}
		dir := api.PaymentDirectionIn
		status, _, body = call(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListPayments(w, r, est, api.ListPaymentsParams{Direction: &dir})
		})
		items, _ := body["items"].([]any)
		if status != 200 || len(items) != 3 || items[1].(map[string]any)["booking_code"] != "A1B2C3" {
			t.Fatalf("list payments: %d %v", status, body)
		}

		// Current session: cash expected = 100 + 30 - 5 + 11 = 136.00.
		status, hdr, body = call(t, e.cashier, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetCurrentCashSession(w, r, est, api.GetCurrentCashSessionParams{})
		})
		session := body["session"].(map[string]any)
		if status != 200 || hdr.Get("ETag") != `"1"` || session["expected_totals"].(map[string]any)["cash"] != float64(13600) {
			t.Fatalf("current: %d %v", status, body)
		}
		if tbm := body["totals_by_method"].(map[string]any); tbm["cash"] != float64(3600) || tbm["card"] != float64(2000) {
			t.Fatalf("totals by method: %v", tbm)
		}
		// Movement out, then close with a stale and a good If-Match.
		if status, _, body := call(t, e.cashier, http.MethodPost, api.CashMovementInput{Kind: api.CashMovementInputKindOut, Amount: 3600, Reason: "versamento"},
			func(w http.ResponseWriter, r *http.Request) {
				e.h.AddCashMovement(w, r, est, sessionID, api.AddCashMovementParams{})
			}); status != 201 || body["kind"] != "out" {
			t.Fatalf("movement: %d %v", status, body)
		}
		closeWith := func(ifMatch string) (int, http.Header, map[string]any) {
			return call(t, e.cashier, http.MethodPost, api.CashSessionClose{CountedTotals: api.MethodTotals{"cash": 9950, "card": 2000}},
				func(w http.ResponseWriter, r *http.Request) {
					e.h.CloseCashSession(w, r, est, sessionID, api.CloseCashSessionParams{IfMatch: &ifMatch})
				})
		}
		if status, _, body := closeWith(`"9"`); status != 412 || body["type"] != "version_conflict" {
			t.Fatalf("stale close: %d %v", status, body)
		}
		status, hdr, body = closeWith(`"1"`)
		if status != 200 || hdr.Get("ETag") != `"2"` || body["status"] != "closed" || body["difference"] != float64(-50) {
			t.Fatalf("close: %d %v", status, body)
		}
		if status, _, body := call(t, e.cashier, http.MethodPost, api.CashMovementInput{Kind: api.CashMovementInputKindIn, Amount: 1, Reason: "x"},
			func(w http.ResponseWriter, r *http.Request) {
				e.h.AddCashMovement(w, r, est, sessionID, api.AddCashMovementParams{})
			}); status != 409 || body["type"] != "cash_session_closed" {
			t.Fatalf("movement on closed: %d %v", status, body)
		}
		status, _, body = call(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListCashSessions(w, r, est, api.ListCashSessionsParams{})
		})
		if status != 200 || len(body["items"].([]any)) != 1 {
			t.Fatalf("history: %d %v", status, body)
		}

		// Corrispettivi: cashier forbidden, JSON and CSV for the owner.
		day := calendar.MustParse("2027-07-15")
		params := api.GetCorrispettiviParams{From: day, To: day}
		if status, _, _ := call(t, e.cashier, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.GetCorrispettivi(w, r, est, params) }); status != 403 {
			t.Fatalf("cashier export: %d", status)
		}
		status, _, body = call(t, e.owner, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.GetCorrispettivi(w, r, est, params) })
		if status != 200 || body["grand_total"] != float64(3000+2000-500+1100) || body["group_by"] != "day" {
			t.Fatalf("corrispettivi json: %d %v", status, body)
		}
		csv := api.Csv
		params.Format = &csv
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(context.Background(), e.owner))
		w := httptest.NewRecorder()
		e.h.GetCorrispettivi(w, r, est, params)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") ||
			!strings.Contains(w.Header().Get("Content-Disposition"), "corrispettivi_2027-07-15_2027-07-15.csv") ||
			!strings.Contains(w.Body.String(), "Totale generale;;;;56,00;;") {
			t.Fatalf("csv: %d %v %q", w.Code, w.Header(), w.Body.String())
		}

		status, _, body = call(t, e.owner, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListFiscalDocuments(w, r, est, api.ListFiscalDocumentsParams{})
		})
		if status != 200 || len(body["items"].([]any)) != 4 {
			t.Fatalf("fiscal documents: %d %v", status, body)
		}
		// Cross-tenant: the owner of another organization gets 404 on both
		// fiscal endpoints of this establishment.
		status, _, body = call(t, e.ownerB, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListFiscalDocuments(w, r, est, api.ListFiscalDocumentsParams{})
		})
		if status != 404 || body["type"] != "not_found" {
			t.Fatalf("cross-tenant fiscal documents: %d %v", status, body)
		}
		if status, _, body := call(t, e.ownerB, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.GetCorrispettivi(w, r, est, params) }); status != 404 || body["type"] != "not_found" {
			t.Fatalf("cross-tenant corrispettivi: %d %v", status, body)
		}
	})
}
