//go:build integration

package checkout_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// Agreement link and code on the public page (spec 30.2.2, ADR 0028).
func TestCheckout_AgreementCode(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		f := e.F
		mk := func(name, code string, channels []string, active bool, maxPerDay *int) *pricingconfig.Agreement {
			t.Helper()
			ten := pricing.MustParseDecimal("10")
			in := pricingconfig.AgreementInput{
				Name: name, Code: &code, Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: &ten}, Active: active,
				M8: pricingconfig.AgreementM8{Channels: channels, Requirement: "conferma dell'hotel"},
			}
			if maxPerDay != nil {
				in.M8.Quota = &pricingconfig.AgreementQuota{MaxPerDay: maxPerDay}
			}
			a, err := f.Pricing.CreateAgreement(e.Ctx, f.Owner, f.Est.ID, in)
			if err != nil {
				t.Fatal(err)
			}
			return a
		}
		one := 1
		link := mk("Ospiti Miramare", "MIRAMARE", []string{pricingconfig.ChannelOnlineLink}, true, &one)
		mk("Solo cassa", "CASSA", []string{pricingconfig.ChannelDesk}, true, nil)
		mk("Spenta", "SPENTA", nil, false, nil)
		// An agreement from before M8: a code and channels NULL (as
		// migration 00013 leaves every existing row). Desk only.
		legacy := mk("Residenti", "RESIDENTI", nil, true, nil)
		if _, err := db.NewUpdate().TableExpr("agreements").Set("channels = NULL").Where("id = ?", legacy.ID).Exec(e.Ctx); err != nil {
			t.Fatal(err)
		}
		// Valid online, but expired: the public page must not tell it
		// apart from an unknown code.
		expired := mk("Scaduta", "SCADUTA", []string{pricingconfig.ChannelOnlineCode}, true, nil)
		if _, err := db.NewUpdate().TableExpr("agreements").Set("valid_to = ?", "2020-01-01").Where("id = ?", expired.ID).Exec(e.Ctx); err != nil {
			t.Fatal(err)
		}

		got, err := e.Svc.GetAgreement(e.Ctx, checkouttest.Slug, "miramare")
		if err != nil || got.ID != link.ID || got.M8.Requirement != "conferma dell'hotel" || got.Summary != "-10%" {
			t.Fatalf("public agreement: %v %+v", err, got)
		}
		for _, code := range []string{"CASSA", "SPENTA", "RESIDENTI", "SCADUTA", "NESSUNO", "a b"} {
			if _, err := e.Svc.GetAgreement(e.Ctx, checkouttest.Slug, code); !errors.Is(err, checkout.ErrAgreementNotFound) {
				t.Fatalf("code %q: %v (must be 404)", code, err)
			}
		}
		// The quote is not an oracle either: every refused code is
		// "invalid" without a name (ADR 0028).
		for _, code := range []string{"CASSA", "SPENTA", "RESIDENTI", "SCADUTA", "NESSUNO"} {
			sel := e.Seat(f.Spots[0], "2027-07-10")
			sel.AgreementCode = code
			q, err := e.Svc.Quote(e.Ctx, checkouttest.Slug, sel)
			if err != nil || q.Agreement == nil || q.Agreement.Status != checkout.AgreementInvalid || q.Agreement.Name != "" || q.Total != 2000 {
				t.Fatalf("quote with code %q: %v %+v total %d (must be invalid, no name, full price)", code, err, q.Agreement, q.Total)
			}
		}

		sel := e.Seat(f.Spots[0], "2027-07-10")
		sel.AgreementCode = "MIRAMARE"
		q, err := e.Svc.Quote(e.Ctx, checkouttest.Slug, sel)
		if err != nil || q.Agreement == nil || q.Agreement.Status != checkout.AgreementApplied || q.Total != 1800 {
			t.Fatalf("quote with the code: %v %+v total %d", err, q.Agreement, q.Total)
		}
		sel.AgreementCode = "CASSA"
		if q, err = e.Svc.Quote(e.Ctx, checkouttest.Slug, sel); err != nil || q.Agreement.Status != checkout.AgreementInvalid || q.Total != 2000 {
			t.Fatalf("desk-only code online: %v %+v", err, q.Agreement)
		}

		sel.AgreementCode = "MIRAMARE"
		s := e.Create(sel)
		b := e.Booking(s.BookingID)
		if b.AgreementID == nil || *b.AgreementID != link.ID || b.TotalAmount != 1800 {
			t.Fatalf("online booking with the code: %+v", b)
		}
		// The cap of 1 per half-day is reached: the quote says so and the
		// booking is refused by the quota counter.
		sel2 := e.Seat(f.Spots[1], "2027-07-10")
		sel2.AgreementCode = "MIRAMARE"
		if q, err = e.Svc.Quote(e.Ctx, checkouttest.Slug, sel2); err != nil || q.Agreement.Status != checkout.AgreementQuotaExhausted {
			t.Fatalf("quota exhausted: %v %+v", err, q.Agreement)
		}
		if _, err := e.Svc.Create(e.Ctx, checkouttest.Slug, checkout.CreateInput{Selection: sel2, Locale: "it"}); err == nil {
			t.Fatal("booking over the agreement cap")
		}
		sel2.AgreementCode = "CASSA"
		if _, err := e.Svc.Create(e.Ctx, checkouttest.Slug, checkout.CreateInput{Selection: sel2, Locale: "it"}); !errors.Is(err, booking.ErrAgreementNotApplicable) {
			t.Fatalf("desk-only code on a booking: %v", err)
		}
	})
}
