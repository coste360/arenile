//go:build integration

package public_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/waitlist"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

// TestPublicHTTP_CreditsAndWaitlist drives the M5B public endpoints over
// HTTP: credit balance and apply-credit from the magic link (verified
// identity only), and the waitlist join (202, no data).
func TestPublicHTTP_CreditsAndWaitlist(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		f := e.F
		c := f.Clock
		aw, ob := audit.NewWriter(c), event.NewOutbox(c)
		reg := app.NewRegistry(c, ext.Extensions{})
		ledger := credit.NewService(credit.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		cust := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		cust.UseCreditLedger(ledger)
		pays := payment.NewService(payment.Deps{
			DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
			Cash:   cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob}),
			Fiscal: fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: reg}), Credits: cust, Providers: reg,
		})
		ledger.SetPayments(pays)
		wl := waitlist.NewService(waitlist.Deps{DB: db, Clock: c, Holds: e.Inv, Checkout: e.Svc, Customers: cust, Terms: e.Online})
		h := httpserver.New(httpserver.Deps{
			DB: db, Clock: c, Version: "test", Bus: event.NewBroker(nil, 16), Readiness: readiness{db},
			Establishments: establishment.NewService(db), Online: e.Online, Checkout: e.Svc, Waitlist: wl, Credits: ledger,
			PublicCreationsPerMinute: 1000, PublicReadsPerMinute: 1000,
		})
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		cl := &client{t: t, base: srv.URL + "/api/public/v1/" + checkouttest.Slug}

		// A confirmed desk booking of 20.00 and a magic link.
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-10")
		in.Force = true
		d, err := e.Book.Create(e.Ctx, f.Owner, f.Est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		tok := ""
		if err := store.RunInTx(e.Ctx, db, func(ctx context.Context, tx bun.Tx) error {
			l, err := e.Notify.IssueMagicLink(ctx, tx, e.ES, d.Booking.ID, nil)
			if err == nil {
				tok = l.Token
			}
			if err != nil {
				return err
			}
			return ledger.GrantCredit(ctx, tx, e.ES, &store.CustomerCredit{CustomerID: d.Booking.CustomerID, Amount: 1500, Source: store.CreditSourceGoodwill})
		}); err != nil {
			t.Fatal(err)
		}
		resp, body, _ := cl.do(http.MethodGet, "/manage/"+tok+"/credits", nil, nil)
		expect(t, resp, body, http.StatusOK, "")
		if body["available"].(float64) != 1500 || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("credits %v", body)
		}
		resp, body, _ = cl.do(http.MethodGet, "/manage/"+tok, nil, nil)
		expect(t, resp, body, http.StatusOK, "")
		if body["credit_available"].(float64) != 1500 {
			t.Fatalf("manage credit_available %v", body["credit_available"])
		}
		resp, body, _ = cl.do(http.MethodPost, "/manage/"+tok+"/apply-credit", map[string]any{"amount": 1000}, map[string]string{"Idempotency-Key": "apply-credit-0001"})
		expect(t, resp, body, http.StatusOK, "")
		if body["credit_available"].(float64) != 500 {
			t.Fatalf("after apply %v", body)
		}
		if b := e.Booking(d.Booking.ID); b.PaidAmount != 1000 {
			t.Fatalf("paid %d", b.PaidAmount)
		}
		// More than the balance due is refused; a wrong link is a plain 404.
		resp, body, _ = cl.do(http.MethodPost, "/manage/"+tok+"/apply-credit", map[string]any{"amount": 5000}, nil)
		expect(t, resp, body, http.StatusBadRequest, "")
		resp, body, _ = cl.do(http.MethodGet, "/manage/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/credits", nil, nil)
		expect(t, resp, body, http.StatusNotFound, "")

		// Waitlist join: off by default (409 waitlist_disabled), then 202.
		join := map[string]any{
			"starts_on": "2027-07-10", "ends_on": "2027-07-10", "slots": "FULL", "pack_id": f.Base.ID,
			"customer": map[string]any{"first_name": "Dora", "last_name": "Pubblica", "email": "dora@example.org", "accept_terms_version": ""},
		}
		resp, body, _ = cl.do(http.MethodPost, "/waitlist", join, nil)
		expect(t, resp, body, http.StatusConflict, "waitlist_disabled")
		if _, err := wl.UpdateSettings(e.Ctx, f.Owner, f.Est.ID, nil, waitlist.Settings{Enabled: true, PublicJoin: true, AutoOffer: true, OfferHoldMinutes: 30}, ""); err != nil {
			t.Fatal(err)
		}
		resp, body, raw := cl.do(http.MethodPost, "/waitlist", join, nil)
		if resp.StatusCode != http.StatusAccepted || len(raw) != 0 {
			t.Fatalf("join %d %v", resp.StatusCode, body)
		}
		if n := e.Count((*store.WaitlistEntry)(nil), "channel = ?", store.ChannelOnline); n != 1 {
			t.Fatalf("entries %d", n)
		}
	})
}
