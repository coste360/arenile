//go:build integration

package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/printout"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestCashRegistersHTTP_TwoRegistersAndZReport: register CRUD with ETag /
// If-Match, two sessions open on two registers, register name on the
// session, the printable Z (HTML with its CSP) for both paper sizes.
func TestCashRegistersHTTP_TwoRegistersAndZReport(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newDeskEnv(t, db)
		est := e.est.ID

		status, _, body := call(t, e.cashier, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListCashRegisters(w, r, est, api.ListCashRegistersParams{})
		})
		items, _ := body["items"].([]any)
		if status != 200 || len(items) != 1 || items[0].(map[string]any)["code"] != "main" {
			t.Fatalf("list: %d %v", status, body)
		}
		if status, _, _ := call(t, e.cashier, http.MethodPost, api.CashRegisterInput{Name: "Bar"}, func(w http.ResponseWriter, r *http.Request) {
			e.h.CreateCashRegister(w, r, est, api.CreateCashRegisterParams{})
		}); status != 403 {
			t.Fatalf("cashier create: %d", status)
		}
		status, hdr, body := call(t, e.owner, http.MethodPost, api.CashRegisterInput{Name: "Bar"}, func(w http.ResponseWriter, r *http.Request) {
			e.h.CreateCashRegister(w, r, est, api.CreateCashRegisterParams{})
		})
		if status != 201 || hdr.Get("ETag") != `"1"` || body["code"] != "bar" {
			t.Fatalf("create: %d %v", status, body)
		}
		barID := body["id"].(string)
		if status, _, body := call(t, e.owner, http.MethodPost, api.CashRegisterInput{Name: "Bar 2", Code: strp("bar")}, func(w http.ResponseWriter, r *http.Request) {
			e.h.CreateCashRegister(w, r, est, api.CreateCashRegisterParams{})
		}); status != 409 || body["type"] != "conflict" {
			t.Fatalf("duplicate code: %d %v", status, body)
		}
		stale := `"9"`
		if status, _, body := call(t, e.owner, http.MethodPut, api.CashRegisterInput{Name: "Bar centrale"}, func(w http.ResponseWriter, r *http.Request) {
			e.h.UpdateCashRegister(w, r, est, barID, api.UpdateCashRegisterParams{IfMatch: &stale})
		}); status != 412 || body["type"] != "version_conflict" {
			t.Fatalf("stale update: %d %v", status, body)
		}
		v1 := `"1"`
		status, hdr, body = call(t, e.owner, http.MethodPut, api.CashRegisterInput{Name: "Bar centrale", Code: strp("ignored")}, func(w http.ResponseWriter, r *http.Request) {
			e.h.UpdateCashRegister(w, r, est, barID, api.UpdateCashRegisterParams{IfMatch: &v1})
		})
		if status != 200 || hdr.Get("ETag") != `"2"` || body["name"] != "Bar centrale" || body["code"] != "bar" {
			t.Fatalf("update: %d %v", status, body)
		}

		// Two sessions at once.
		status, _, body = call(t, e.cashier, http.MethodPost, api.CashSessionOpen{OpeningFloat: 10000}, func(w http.ResponseWriter, r *http.Request) {
			e.h.OpenCashSession(w, r, est, api.OpenCashSessionParams{})
		})
		if status != 201 || body["register_name"] != store.DefaultCashRegisterName {
			t.Fatalf("open main: %d %v", status, body)
		}
		mainID := body["id"].(string)
		status, _, body = call(t, e.cashier, http.MethodPost, api.CashSessionOpen{OpeningFloat: 2000, RegisterId: &barID}, func(w http.ResponseWriter, r *http.Request) {
			e.h.OpenCashSession(w, r, est, api.OpenCashSessionParams{})
		})
		if status != 201 || body["register_id"] != barID || body["register"] != "bar" || body["register_name"] != "Bar centrale" {
			t.Fatalf("open bar: %d %v", status, body)
		}
		barSession := body["id"].(string)
		status, _, body = call(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetCurrentCashSession(w, r, est, api.GetCurrentCashSessionParams{RegisterId: &barID})
		})
		if status != 200 || body["session"].(map[string]any)["id"] != barSession {
			t.Fatalf("current bar: %d %v", status, body)
		}
		status, _, body = call(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListCashSessions(w, r, est, api.ListCashSessionsParams{RegisterId: &barID})
		})
		if items, _ := body["items"].([]any); status != 200 || len(items) != 1 {
			t.Fatalf("history bar: %d %v", status, body)
		}
		status, _, body = call(t, e.owner, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetCashRegister(w, r, est, barID)
		})
		if status != 200 || body["open_session_id"] != barSession {
			t.Fatalf("get bar: %d %v", status, body)
		}
		v2 := `"2"`
		if status, _, body := call(t, e.owner, http.MethodDelete, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.DeleteCashRegister(w, r, est, barID, api.DeleteCashRegisterParams{IfMatch: &v2})
		}); status != 409 {
			t.Fatalf("delete used: %d %v", status, body)
		}

		// Close bar, then print the Z.
		if status, _, body := call(t, e.cashier, http.MethodPost, api.CashSessionClose{CountedTotals: api.MethodTotals{"cash": 1990}}, func(w http.ResponseWriter, r *http.Request) {
			e.h.CloseCashSession(w, r, est, barSession, api.CloseCashSessionParams{})
		}); status != 200 {
			t.Fatalf("close bar: %d %v", status, body)
		}
		z := func(p *auth.Principal, session string, paper *api.GetCashSessionZReportParamsPaper) *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r = r.WithContext(auth.WithPrincipal(r.Context(), p))
			w := httptest.NewRecorder()
			e.h.GetCashSessionZReport(w, r, est, session, api.GetCashSessionZReportParams{Paper: paper})
			return w
		}
		w := z(e.viewer, barSession, nil)
		html := w.Body.String()
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") ||
			w.Header().Get("Content-Security-Policy") != printout.ContentSecurityPolicy || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("z headers: %d %v", w.Code, w.Header())
		}
		for _, want := range []string{"Chiusura Z — Bar centrale", "Lido A", "@page { size: A4;", "data:image/svg&#43;xml;base64,", `alt="Arenile"`, "−0,10 €", "chiusura-z-bar-"} {
			if !strings.Contains(html+w.Header().Get("Content-Disposition"), want) {
				t.Errorf("z report misses %q", want)
			}
		}
		if strings.Contains(html, `data-testid="draft"`) {
			t.Error("closed session printed as draft")
		}
		receipt := api.Receipt80
		if w := z(e.cashier, mainID, &receipt); w.Code != 200 || !strings.Contains(w.Body.String(), "size: 80mm auto") || !strings.Contains(w.Body.String(), `data-testid="draft"`) {
			t.Fatalf("receipt draft: %d", w.Code)
		}
		bad := api.GetCashSessionZReportParamsPaper("a3")
		if w := z(e.cashier, mainID, &bad); w.Code != 400 {
			t.Fatalf("bad paper: %d", w.Code)
		}
		if w := z(e.ownerB, mainID, nil); w.Code != 404 {
			t.Fatalf("cross tenant z: %d", w.Code)
		}
		if w := z(e.owner, store.NewID(), nil); w.Code != 404 {
			t.Fatalf("unknown z: %d", w.Code)
		}
	})
}
