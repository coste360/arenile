//go:build integration

package admin_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestBookingSummaryHTTP_ShareAndPrint: the WhatsApp text carries dates,
// spots, totals and a new manage link, wa.me only with a phone; the print
// is a self-contained document with the QR (spec 30.9).
func TestBookingSummaryHTTP_ShareAndPrint(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		svc := notify.NewService(notify.Deps{
			DB: db, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock),
			BaseURL: "https://arenile.example",
		})
		h := &admin.Handlers{Bookings: f.Svc, Notify: svc, Log: slog.New(slog.DiscardHandler)}
		est := f.Est.ID
		d := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		id := d.Booking.ID

		share := func(p *auth.Principal) (int, http.Header, map[string]any) {
			return bkCall(t, p, nil, "", func(w http.ResponseWriter, r *http.Request) { h.GetBookingShareSummary(w, r, est, id) })
		}
		status, hd, body := share(f.Cashier)
		bkExpect(t, "share", status, body, 200, "")
		text, _ := body["text"].(string)
		manage, _ := body["manage_url"].(string)
		wa, _ := body["whatsapp_url"].(string)
		if body["locale"] != "it" || !strings.Contains(text, "Mario") || !strings.Contains(text, d.Booking.Code) ||
			!strings.Contains(text, "A1") || !strings.Contains(text, manage) {
			t.Fatalf("share text %v", body)
		}
		if !strings.HasPrefix(manage, "https://arenile.example/b/lido-delfino/m/") {
			t.Fatalf("manage url %q", manage)
		}
		if !strings.HasPrefix(wa, "https://wa.me/393331234567?text=") || hd.Get("Cache-Control") != "no-store" {
			t.Fatalf("whatsapp url %q, cache %q", wa, hd.Get("Cache-Control"))
		}
		// A new link at every call (the previous one is revoked).
		_, _, again := share(f.Cashier)
		if again["manage_url"] == manage {
			t.Fatal("manage link not renewed")
		}
		if status, _, body := share(f.Viewer); status != http.StatusForbidden {
			t.Fatalf("viewer share: %d %v", status, body)
		}

		// Without a phone: no wa.me link.
		if _, err := db.NewUpdate().Model((*store.Customer)(nil)).Set("phone = NULL").Where("id = ?", d.Booking.CustomerID).Exec(f.Ctx); err != nil {
			t.Fatal(err)
		}
		if _, _, body := share(f.Cashier); body["whatsapp_url"] != nil {
			t.Fatalf("whatsapp without phone: %v", body)
		}

		for _, paper := range []string{"a4", "receipt80"} {
			p := api.GetBookingSummaryPrintParamsPaper(paper)
			r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(f.Ctx, f.Viewer))
			w := httptest.NewRecorder()
			h.GetBookingSummaryPrint(w, r, est, id, api.GetBookingSummaryPrintParams{Paper: &p})
			html := w.Body.String()
			if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "text/html") ||
				!strings.Contains(html, d.Booking.Code) || !strings.Contains(html, `alt="QR `+d.Booking.Code) ||
				strings.Contains(html, "<script") || strings.Contains(html, "/m/") {
				t.Fatalf("print %s: %d %s", paper, w.Code, html)
			}
			if paper == "receipt80" && !strings.Contains(html, "80mm") {
				t.Fatalf("receipt paper: %s", html)
			}
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(f.Ctx, f.Outsider))
		w := httptest.NewRecorder()
		h.GetBookingSummaryPrint(w, r, est, id, api.GetBookingSummaryPrintParams{})
		if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
			t.Fatalf("outsider print: %d", w.Code)
		}
	})
}
