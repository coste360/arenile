//go:build integration

package checkout_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestCheckoutFlow_ManualConfirm: create (held, placeholder customer,
// hashed token) -> state (details) -> customer + terms + consent -> pay
// with the manual driver (no online provider) -> confirmed; reads never
// confirm, the audit names the customer session, outbox has booking.held
// and booking.confirmed, the placeholder never leaks.
func TestCheckoutFlow_ManualConfirm(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		if _, err := e.Online.PublishTerms(ctx, e.F.Manager, e.F.Est.ID, online.TermsInput{
			Version: "2027.1", Texts: map[string]store.TermsTexts{"it": {Terms: "Condizioni"}}, MakeCurrent: true,
		}); err != nil {
			t.Fatal(err)
		}

		q, err := e.Svc.Quote(ctx, slug, e.Seat(e.F.Spots[0], "2027-07-10"))
		if err != nil || !q.Available || q.Total != 2000 || q.Plan != checkout.PlanAtDesk || q.DueNow != 0 || len(q.Items) != 1 {
			t.Fatalf("quote: %v %+v", err, q)
		}
		s := e.Create(e.Seat(e.F.Spots[0], "2027-07-10"))
		if len(s.Token) != 43 || s.Status != "held" || s.Quote.Total != 2000 || !s.HoldExpiresAt.Equal(e.F.Clock.Now().Add(10*time.Minute)) {
			t.Fatalf("session %+v", s)
		}
		var row store.CheckoutSession
		if err := db.NewSelect().Model(&row).Where("booking_id = ?", s.BookingID).Scan(ctx); err != nil || row.TokenHash != checkout.HashToken(s.Token) || row.TokenHash == s.Token {
			t.Fatalf("token must be stored as hash only: %v", err)
		}
		b := e.Booking(s.BookingID)
		if b.Channel != "online" || b.CustomerID != customer.PlaceholderID(e.F.Est.ID) {
			t.Fatalf("held booking %+v", b)
		}
		// The same quote now sees the hold.
		if q, _ := e.Svc.Quote(ctx, slug, e.Seat(e.F.Spots[0], "2027-07-10")); q.Available || len(q.Conflicts) == 0 {
			t.Fatalf("quote over a held spot %+v", q)
		}

		st, err := e.Svc.State(ctx, slug, s.BookingID, s.Token)
		if err != nil || st.NextAction != checkout.NextDetails || st.Session.State != "open" {
			t.Fatalf("state: %v %+v", err, st)
		}
		for _, tok := range []string{"", "short", strings.Repeat("A", 43)} {
			if _, err := e.Svc.State(ctx, slug, s.BookingID, tok); !isNotFound(err) {
				t.Fatalf("wrong token %q: %v", tok, err)
			}
		}
		if _, err := e.Svc.State(ctx, "lido-altro", s.BookingID, s.Token); !isNotFound(err) {
			t.Fatalf("other establishment: %v", err)
		}
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); !errors.Is(err, checkout.ErrDetailsMissing) {
			t.Fatalf("pay before details: %v", err)
		}

		// Step 4: wrong terms version, invalid phone, then the real data.
		in := checkout.CustomerInput{FirstName: "Giulia", LastName: "Bianchi", Email: "Giulia@Example.org", Phone: "333 1234567", AcceptTermsVersion: "2026.9"}
		if _, err := e.Svc.UpdateCustomer(ctx, slug, s.BookingID, s.Token, in); !errors.Is(err, checkout.ErrTermsOutdated) {
			t.Fatalf("old terms: %v", err)
		}
		in.AcceptTermsVersion, in.Phone = "2027.1", "12"
		var ve *configerr.ValidationError
		if _, err := e.Svc.UpdateCustomer(ctx, slug, s.BookingID, s.Token, in); !errors.As(err, &ve) || ve.Fields[0].Field != "phone" {
			t.Fatalf("bad phone: %v", err)
		}
		in.Phone, in.MarketingConsent = "333 1234567", true
		st, err = e.Svc.UpdateCustomer(ctx, slug, s.BookingID, s.Token, in)
		if err != nil || st.NextAction != checkout.NextPay {
			t.Fatalf("details: %v %+v", err, st)
		}
		b = e.Booking(s.BookingID)
		if b.TermsVersion == nil || *b.TermsVersion != "2027.1" || b.TermsAcceptedAt == nil || b.Locale == nil || *b.Locale != "it" {
			t.Fatalf("terms on booking %+v", b)
		}
		c, err := store.GetCustomer(ctx, db, e.ES, b.CustomerID)
		if err != nil || c.Email == nil || *c.Email != "giulia@example.org" || *c.Phone != "+393331234567" || c.MarketingConsent == nil || c.MarketingConsent.Source != "online" || c.MarketingConsent.TextVersion != "2027.1" {
			t.Fatalf("customer %v %+v", err, c)
		}

		out, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{})
		if err != nil || out.Mode != checkout.PayNone || out.Status != "confirmed" {
			t.Fatalf("pay: %v %+v", err, out)
		}
		st, err = e.Svc.State(ctx, slug, s.BookingID, s.Token)
		if err != nil || st.NextAction != checkout.NextDone || st.Confirmation == nil || st.Confirmation.Code == "" ||
			st.Confirmation.EmailMasked == "" || strings.Contains(st.Confirmation.EmailMasked, "giulia@") || st.Confirmation.QRSVG == "" {
			t.Fatalf("confirmed state: %v %+v", err, st)
		}
		for _, a := range st.Booking.SpotAllocations {
			if a.Kind != store.AllocationBooking || a.ExpiresAt != nil {
				t.Fatalf("allocation after confirm %+v", a)
			}
		}
		doc, err := e.Svc.Document(ctx, slug, s.BookingID, s.Token, checkout.DocICS)
		if err != nil || !strings.HasPrefix(string(doc.Body), "BEGIN:VCALENDAR") || !strings.HasSuffix(doc.Filename, ".ics") {
			t.Fatalf("ics %v", err)
		}
		if n := e.Count((*store.OutboxEvent)(nil), "type IN (?, ?)", event.BookingHeld, event.BookingConfirmed); n != 2 {
			t.Fatalf("held + confirmed events = %d", n)
		}
		var entries []store.AuditEntry
		if err := db.NewSelect().Model(&entries).Where("entity_id = ?", s.BookingID).Order("created_at ASC").Scan(ctx); err != nil {
			t.Fatal(err)
		}
		for _, en := range entries {
			if en.ActorType != "customer" || en.ActorID == nil || *en.ActorID != row.ID {
				t.Fatalf("audit actor %+v", en)
			}
			if strings.Contains(string(en.Diff), "giulia") || strings.Contains(string(en.Diff), s.Token) {
				t.Fatalf("personal data or token in the audit: %s", en.Diff)
			}
		}
		// Paying again is refused (already confirmed), the state is stable.
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); !errors.Is(err, booking.ErrInvalidTransition) {
			t.Fatalf("second pay: %v", err)
		}
		// The placeholder stays hidden from the customer list.
		page, err := store.ListCustomers(ctx, db, e.ES, store.CustomerFilter{}, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, cu := range page.Items {
			if customer.IsPlaceholder(&cu) {
				t.Fatal("placeholder customer listed")
			}
		}
	})
}

// TestCheckoutFlow_OnlinePaymentNeverConfirmsFromBrowser: with an online
// provider the pay step moves the booking to pending_payment, extends the
// hold (max 30 minutes, bounded by the session) and hands back the
// provider URL with the return URL of the page; polling the state never
// confirms.
func TestCheckoutFlow_OnlinePaymentNeverConfirmsFromBrowser(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		sk, wh := "sk_live_51abcdefghijklmnopqrstuv", "whsec_abcdefghijklmn"
		if _, err := e.Online.UpdatePayments(ctx, e.F.Owner, e.F.Est.ID, nil, online.PaymentSettingsInput{
			Provider: "stripe", Mode: "live", Stripe: &online.StripeInput{SecretKey: &sk, WebhookSecret: &wh},
		}); err != nil {
			t.Fatal(err)
		}
		s := e.Create(e.Seat(e.F.Spots[1], "2027-07-12"))
		if s.Quote.Plan != checkout.PlanOnlineFull || s.Quote.DueNow != 2000 {
			t.Fatalf("plan %+v", s.Quote)
		}
		e.Details(s, "mario@example.org")
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{Settlement: checkout.SettleAtDesk}); !errors.Is(err, checkout.ErrSettlementNotAllowed) {
			t.Fatalf("at desk with online policy: %v", err)
		}
		e.F.Clock.Advance(2 * time.Minute)
		out, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{})
		if err != nil || out.Mode != checkout.PayRedirect || out.Status != "pending_payment" || out.Amount != 2000 || !strings.Contains(out.RedirectURL, s.BookingID) {
			t.Fatalf("pay: %v %+v", err, out)
		}
		req := e.Pay.Starts[0]
		if req.ReturnURL != "https://prenota.example.org/b/lido-delfino?checkout="+s.BookingID || req.CustomerEmail != "mario@example.org" || req.Purpose != "full" {
			t.Fatalf("start request %+v", req)
		}
		b := e.Booking(s.BookingID)
		if b.Status != "pending_payment" || !b.HoldExpiresAt.Equal(e.F.Clock.Now().Add(checkout.MaxPaymentHold)) {
			t.Fatalf("pending booking %+v", b.HoldExpiresAt)
		}
		// The browser comes back: polling never confirms.
		for range 3 {
			st, err := e.Svc.State(ctx, slug, s.BookingID, s.Token)
			if err != nil || st.NextAction != checkout.NextWait || st.Booking.Booking.Status != "pending_payment" {
				t.Fatalf("poll: %v %+v", err, st.NextAction)
			}
		}
		// A retry extends at most to the session deadline.
		e.F.Clock.Advance(20 * time.Minute)
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); err != nil {
			t.Fatal(err)
		}
		deadline := e.Booking(s.BookingID).HoldExpiresAt
		if deadline.After(e.F.Clock.Now().Add(checkout.MaxPaymentHold)) || len(e.Pay.Starts) != 2 {
			t.Fatalf("retry hold %v", deadline)
		}
		// Provider down: the hold stays.
		e.Pay.Fail = errors.New("provider down")
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); err == nil {
			t.Fatal("provider failure not reported")
		}
		if e.Booking(s.BookingID).Status != "pending_payment" {
			t.Fatal("hold lost on provider failure")
		}
	})
}

// TestCheckoutExpiry_ReleasesInventory: a hold not paid in time is expired
// by the existing sweeper (booking hold job), its allocations are deleted
// and the spot sells again; the session reads as expired, even before the
// sweep. Abandon releases at once and is idempotent.
func TestCheckoutExpiry_ReleasesInventory(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		s := e.Create(e.Seat(e.F.Spots[2], "2027-07-15"))
		if n := e.Count((*store.SpotAllocation)(nil), "booking_id = ?", s.BookingID); n != 2 {
			t.Fatalf("hold rows %d", n)
		}
		if _, err := e.Svc.Create(ctx, slug, checkout.CreateInput{Selection: e.Seat(e.F.Spots[2], "2027-07-15")}); !errors.Is(err, availability.ErrSpotUnavailable) {
			t.Fatalf("second checkout on a held spot: %v", err)
		}
		e.F.Clock.Advance(11 * time.Minute)
		st, err := e.Svc.State(ctx, slug, s.BookingID, s.Token)
		if err != nil || st.NextAction != checkout.NextExpired {
			t.Fatalf("elapsed hold before sweep: %v %+v", err, st.NextAction)
		}
		if _, err := e.Svc.UpdateCustomer(ctx, slug, s.BookingID, s.Token, checkout.CustomerInput{FirstName: "A", LastName: "B", Email: "a@b.it"}); !errors.Is(err, availability.ErrHoldExpired) {
			t.Fatalf("details on an elapsed hold: %v", err)
		}
		n, err := e.Book.ExpireHolds(ctx)
		if err != nil || n != 1 {
			t.Fatalf("sweeper: %v %d", err, n)
		}
		if e.Booking(s.BookingID).Status != "expired" || e.Count((*store.SpotAllocation)(nil), "booking_id = ?", s.BookingID) != 0 {
			t.Fatal("expired hold kept its allocations")
		}
		again := e.Create(e.Seat(e.F.Spots[2], "2027-07-15"))
		if err := e.Svc.Abandon(ctx, slug, again.BookingID, again.Token); err != nil {
			t.Fatal(err)
		}
		if err := e.Svc.Abandon(ctx, slug, again.BookingID, again.Token); err != nil {
			t.Fatalf("abandon must be idempotent: %v", err)
		}
		st, _ = e.Svc.State(ctx, slug, again.BookingID, again.Token)
		if st.Session.State != store.CheckoutAbandoned || e.Count((*store.SpotAllocation)(nil), "booking_id = ?", again.BookingID) != 0 {
			t.Fatalf("abandoned %+v", st.Session)
		}
		if n := e.Count((*store.OutboxEvent)(nil), "type = ?", event.BookingExpired); n != 2 {
			t.Fatalf("booking.expired events %d", n)
		}
	})
}

// TestCheckoutZone_AutoAssignAtConfirmation: zone-mode lines hold the zone
// pool while held and get a spot of the zone at confirmation (auto_assign).
func TestCheckoutZone_AutoAssignAtConfirmation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		s := e.Create(e.Zone("2027-07-20"))
		d, err := e.Book.DetailIn(ctx, db, e.ES, s.BookingID)
		if err != nil || d.Lines[0].AssignmentStatus != store.AssignmentPending || len(d.SpotAllocations) != 0 || len(d.PoolAllocations) == 0 {
			t.Fatalf("held zone line %v %+v", err, d.Lines)
		}
		e.Details(s, "zone@example.org")
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); err != nil {
			t.Fatal(err)
		}
		d, _ = e.Book.DetailIn(ctx, db, e.ES, s.BookingID)
		if d.Booking.Status != "confirmed" || d.Lines[0].AssignmentStatus != store.AssignmentAssigned || d.Lines[0].SpotID == nil || len(d.SpotAllocations) != 2 {
			t.Fatalf("assigned at confirmation %+v %+v", d.Lines[0], d.SpotAllocations)
		}
		st, _ := e.Svc.State(ctx, slug, s.BookingID, s.Token)
		if st.Booking.SpotLabels[*d.Lines[0].SpotID] == "" {
			t.Fatal("spot label missing in the confirmation")
		}
	})
}

// TestCheckoutBrowse_PublicReads: profile, calendar, availability (quota,
// closures, online-only zones), catalog, map.
func TestCheckoutBrowse_PublicReads(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		p, err := e.Svc.Profile(ctx, slug)
		if err != nil || len(p.Zones) != 2 || len(p.Maps) != 1 || p.HoldMinutes != 10 || p.MinDate.String() != "2027-06-01" || p.MaxDate.String() != "2027-09-15" {
			t.Fatalf("profile %v %+v", err, p)
		}
		// Hide the seat zone of the other... the zone B stays: close a day.
		now := e.F.Clock.Now()
		e.F.Insert(&store.ClosureDay{ID: store.NewID(), EstablishmentID: e.F.Est.ID, Date: calendar.MustParse("2027-07-05"), Slots: []calendar.Slot{calendar.AM, calendar.PM}, Reason: "meteo", Version: 1, CreatedAt: now, UpdatedAt: now})
		cal, err := e.Svc.Calendar(ctx, slug, checkout.CalendarQuery{Month: "2027-07"})
		if err != nil || len(cal.Days) != 31 {
			t.Fatalf("calendar %v", err)
		}
		if cal.Days[4].Status != checkout.DayClosed || cal.Days[9].Status != checkout.DayAvailable {
			t.Fatalf("days %+v %+v", cal.Days[4], cal.Days[9])
		}
		cal, _ = e.Svc.Calendar(ctx, slug, checkout.CalendarQuery{Month: "2027-05"})
		if cal.Days[0].Status != checkout.DayUnavailable {
			t.Fatalf("before the season and in the past %+v", cal.Days[0])
		}
		e.Create(e.Seat(e.F.Spots[0], "2027-07-10"))
		a, err := e.Svc.Availability(ctx, slug, checkout.AvailabilityQuery{From: calendar.MustParse("2027-07-10"), To: calendar.MustParse("2027-07-10")})
		if err != nil || len(a.Zones) != 2 {
			t.Fatalf("availability %v %+v", err, a)
		}
		seat := a.Zones[0]
		if seat.ZoneID != e.F.SeatZone.ID {
			seat = a.Zones[1]
		}
		if seat.Available != 3 || seat.FromPrice == nil || *seat.FromPrice != 2000 || len(seat.Spots) != 4 || len(a.PriceLegend) == 0 {
			t.Fatalf("seat zone %+v legend %+v", seat, a.PriceLegend)
		}
		cat, err := e.Svc.Catalog(ctx, slug, checkout.CatalogQuery{SpotID: e.F.Spots[0].ID, From: ptr(calendar.MustParse("2027-07-10")), To: ptr(calendar.MustParse("2027-07-10"))})
		if err != nil || len(cat.Packs) != 2 || len(cat.Extras) != 1 || cat.Extras[0].UnitPrice == nil || *cat.Extras[0].UnitPrice != 800 {
			t.Fatalf("catalog %v %+v", err, cat)
		}
		m, err := e.Svc.Map(ctx, slug, e.F.Spots[0].MapID)
		if err != nil || len(m.Spots) != 6 || m.PriceCategory[e.F.Spots[0].ID] != "front" {
			t.Fatalf("map %v %d", err, len(m.Spots))
		}
		// The "from" price of a chosen spot is the price of its own category,
		// not the cheapest of the beach.
		legend := map[string]money.Cents{}
		for _, l := range a.PriceLegend {
			legend[l.PriceCategory] = l.FromPrice
		}
		for _, sp := range e.F.Spots {
			c, err := e.Svc.Catalog(ctx, slug, checkout.CatalogQuery{SpotID: sp.ID, From: ptr(calendar.MustParse("2027-07-10")), To: ptr(calendar.MustParse("2027-07-10"))})
			if err != nil {
				t.Fatal(err)
			}
			cheapest := money.Cents(0)
			for _, pv := range c.Packs {
				if pv.FromPrice != nil && (cheapest == 0 || *pv.FromPrice < cheapest) {
					cheapest = *pv.FromPrice
				}
			}
			want := legend[m.PriceCategory[sp.ID]]
			if cheapest != want {
				t.Fatalf("spot %s (%s): catalog from %d, category from %d", sp.Label, m.PriceCategory[sp.ID], cheapest, want)
			}
		}
		// Zones hidden online disappear everywhere.
		if _, err := db.NewUpdate().Model((*store.Zone)(nil)).Set("online_visible = ?", false).Where("id = ?", e.F.FreeZone.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if a, _ = e.Svc.Availability(ctx, slug, checkout.AvailabilityQuery{From: calendar.MustParse("2027-07-10"), To: calendar.MustParse("2027-07-10")}); len(a.Zones) != 1 {
			t.Fatalf("hidden zone in availability %+v", a.Zones)
		}
		if _, err := e.Svc.Create(ctx, slug, checkout.CreateInput{Selection: e.Zone("2027-07-11")}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("checkout on a hidden zone: %v", err)
		}
		// Booking switched off: selling endpoints answer not found.
		if _, err := e.Online.Update(ctx, e.F.Owner, e.F.Est.ID, nil, online.Input{BookingEnabled: false}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Svc.Quote(ctx, slug, e.Seat(e.F.Spots[3], "2027-07-10")); !errors.Is(err, online.ErrBookingDisabled) {
			t.Fatalf("quote with booking off: %v", err)
		}
	})
}

// TestCheckoutLimits_PerEstablishment: the creation budget of one
// establishment answers a typed rate limit error.
func TestCheckoutLimits_PerEstablishment(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		svc := checkout.NewService(checkout.Deps{
			DB: db, Clock: e.F.Clock, Settings: e.Online, Inventory: e.Inv, Pricer: e.F.Pricing, Bookings: e.Book,
			Payments: e.Pay, Notifier: e.Notify, CreatesPerEstablishment: 2,
		})
		for i := range 2 {
			if _, err := svc.Create(e.Ctx, slug, checkout.CreateInput{Selection: e.Seat(e.F.Spots[i], "2027-07-21")}); err != nil {
				t.Fatal(err)
			}
		}
		var rl *auth.RateLimitError
		if _, err := svc.Create(e.Ctx, slug, checkout.CreateInput{Selection: e.Seat(e.F.Spots[2], "2027-07-21")}); !errors.As(err, &rl) {
			t.Fatalf("third creation: %v", err)
		}
	})
}

func ptr[T any](v T) *T { return &v }

// TestCheckoutFlow_BrokenOnlineProviderNeverConfirms (INV-19): Stripe is
// the active provider but its credentials were cleared, so no online
// payment can be taken. The plan must stay online (never "at the desk")
// and Pay must fail, leaving the booking held: a broken setup never
// confirms bookings with nothing paid.
func TestCheckoutFlow_BrokenOnlineProviderNeverConfirms(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		sk, wh, empty := "sk_live_51abcdefghijklmnopqrstuv", "whsec_abcdefghijklmn", ""
		if _, err := e.Online.UpdatePayments(ctx, e.F.Owner, e.F.Est.ID, nil, online.PaymentSettingsInput{
			Provider: "stripe", Mode: "live", Stripe: &online.StripeInput{SecretKey: &sk, WebhookSecret: &wh},
		}); err != nil {
			t.Fatal(err)
		}
		// The panel refuses to activate Stripe without both secrets; a
		// broken row (key ring rotated, credentials wiped by an operator)
		// is simulated at the database level.
		if _, err := db.NewUpdate().Model((*store.PaymentProviderSettings)(nil)).Set("credentials = ?", empty).
			Where("establishment_id = ? AND provider = ?", e.F.Est.ID, "stripe").Exec(ctx); err != nil {
			t.Fatal(err)
		}
		pe, err := e.Online.ResolvePublic(ctx, slug)
		if err != nil || pe.Provider != "stripe" || pe.Online {
			t.Fatalf("stripe without credentials must not be online: %v %+v", err, pe)
		}
		s := e.Create(e.Seat(e.F.Spots[1], "2027-07-12"))
		if s.Quote.Plan != checkout.PlanOnlineFull || s.Quote.DueNow != 2000 {
			t.Fatalf("plan must stay online, got %+v", s.Quote)
		}
		e.Details(s, "mario@example.org")
		for _, in := range []checkout.PayInput{{}, {Settlement: checkout.SettleOnline}, {Settlement: checkout.SettleAtDesk}} {
			_, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, in)
			if !errors.Is(err, checkout.ErrOnlinePaymentUnavailable) && !errors.Is(err, checkout.ErrSettlementNotAllowed) {
				t.Fatalf("pay %+v: %v", in, err)
			}
		}
		if b := e.Booking(s.BookingID); b.Status != "held" || b.PaidAmount != 0 {
			t.Fatalf("booking must stay held, got %s paid %d", b.Status, b.PaidAmount)
		}
		if n := e.Count((*store.OutboxEvent)(nil), "type = ?", event.BookingConfirmed); n != 0 {
			t.Fatalf("confirmed events = %d", n)
		}
		if len(e.Pay.Starts) != 0 {
			t.Fatal("provider must not be called")
		}
	})
}

// TestCheckoutFlow_TestKeysBlockRealCustomers (INV-88): Stripe configured
// in mode test (test keys) while test mode is off would let a real
// customer pay with a test card and get a confirmed booking for free. The
// public page reports online payment as unavailable (blocked), the plan
// stays online (never "at the desk", INV-19) and Pay refuses with
// gateway.ErrProviderTestMode without calling the provider. With test mode
// on the same keys pay the (test) bookings as before.
func TestCheckoutFlow_TestKeysBlockRealCustomers(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		sk, wh := "sk_test_51abcdefghijklmnopqrstuv", "whsec_abcdefghijklmn"
		if _, err := e.Online.UpdatePayments(ctx, e.F.Owner, e.F.Est.ID, nil, online.PaymentSettingsInput{
			Provider: "stripe", Mode: "test", Stripe: &online.StripeInput{SecretKey: &sk, WebhookSecret: &wh},
		}); err != nil {
			t.Fatal(err)
		}
		ps, err := e.Online.GetPayments(ctx, e.F.Owner, e.F.Est.ID)
		if err != nil || !ps.LivePaymentsBlocked {
			t.Fatalf("payment settings must report the block: %v %+v", err, ps)
		}
		pe, err := e.Online.ResolvePublic(ctx, slug)
		if err != nil || pe.Online || !pe.LivePaymentsBlocked {
			t.Fatalf("test keys outside test mode must not take real payments: %v %+v", err, pe)
		}
		s := e.Create(e.Seat(e.F.Spots[1], "2027-07-12"))
		if e.Booking(s.BookingID).Test {
			t.Fatal("booking flagged test with test mode off")
		}
		if s.Quote.Plan != checkout.PlanOnlineFull || s.Quote.DueNow != 2000 {
			t.Fatalf("plan must stay online, got %+v", s.Quote)
		}
		e.Details(s, "mario@example.org")
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); !errors.Is(err, gateway.ErrProviderTestMode) {
			t.Fatalf("pay with test keys: %v", err)
		}
		if b := e.Booking(s.BookingID); b.Status != "held" || b.PaidAmount != 0 {
			t.Fatalf("booking must stay held, got %s paid %d", b.Status, b.PaidAmount)
		}
		if len(e.Pay.Starts) != 0 {
			t.Fatal("provider must not be called")
		}

		// Test mode on: new bookings are test bookings and pay with the test keys.
		row, err := store.EnsureTestModeSettings(ctx, db, e.ES, e.F.Clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		row.Enabled, row.UpdatedAt = true, e.F.Clock.Now()
		if ok, err := store.UpdateTestModeSettingsVersioned(ctx, db, e.ES, row, row.Version); err != nil || !ok {
			t.Fatalf("switch test mode: %v %v", ok, err)
		}
		if ps, err = e.Online.GetPayments(ctx, e.F.Owner, e.F.Est.ID); err != nil || ps.LivePaymentsBlocked {
			t.Fatalf("no block in test mode: %v %+v", err, ps)
		}
		if pe, err = e.Online.ResolvePublic(ctx, slug); err != nil || !pe.Online || pe.LivePaymentsBlocked || !pe.TestMode {
			t.Fatalf("test mode must pay with the test keys: %v %+v", err, pe)
		}
		ts := e.Create(e.Seat(e.F.Spots[2], "2027-07-12"))
		if !e.Booking(ts.BookingID).Test {
			t.Fatal("booking not flagged test in test mode")
		}
		e.Details(ts, "mario@example.org")
		if out, err := e.Svc.Pay(ctx, slug, ts.BookingID, ts.Token, checkout.PayInput{}); err != nil || out.Mode != checkout.PayRedirect || len(e.Pay.Starts) != 1 {
			t.Fatalf("test booking pay: %v %+v", err, out)
		}
	})
}

// TestCheckoutFlow_PayUsesCurrentTotal (INV-22, INV-51): the amount asked
// to the provider is computed under the booking lock when the payment
// starts, not from the total Pay read first: a change of the total in
// between (staff edit, extras of a later submit) can never leave the
// provider paid below the confirmation threshold.
func TestCheckoutFlow_PayUsesCurrentTotal(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		sk, wh := "sk_live_51abcdefghijklmnopqrstuv", "whsec_abcdefghijklmn"
		if _, err := e.Online.UpdatePayments(ctx, e.F.Owner, e.F.Est.ID, nil, online.PaymentSettingsInput{
			Provider: "stripe", Mode: "live", Stripe: &online.StripeInput{SecretKey: &sk, WebhookSecret: &wh},
		}); err != nil {
			t.Fatal(err)
		}
		s := e.Create(e.Seat(e.F.Spots[1], "2027-07-12"))
		e.Details(s, "mario@example.org")
		// A staff edit raises the total after the page computed its quote.
		if _, err := db.NewUpdate().Model((*store.Booking)(nil)).Set("total_amount = ?", 3500).Set("deposit_due = ?", 3500).
			Where("id = ?", s.BookingID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		out, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{})
		if err != nil || out.Amount != 3500 || len(e.Pay.Starts) != 1 || e.Pay.Starts[0].Amount != 3500 {
			t.Fatalf("pay must ask the current total: %v %+v %+v", err, out, e.Pay.Starts)
		}
		if b := e.Booking(s.BookingID); b.Status != "pending_payment" {
			t.Fatalf("booking after start: %s", b.Status)
		}
	})
}

// TestCheckoutFlow_TokenLifetimeAndPromoLimit: a checkout token stops
// working SessionGrace after the session ended (a token leaking later
// reads nothing); quotes with a promo code are limited per IP beyond the
// ordinary read budget (no code guessing).
func TestCheckoutFlow_TokenLifetimeAndPromoLimit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		s := e.Create(e.Seat(e.F.Spots[0], "2027-07-10"))
		e.Details(s, "mario@example.org")
		if _, err := e.Svc.Pay(ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); err != nil {
			t.Fatal(err)
		}
		e.F.Clock.Advance(checkout.SessionGrace - time.Minute)
		if _, err := e.Svc.State(ctx, slug, s.BookingID, s.Token); err != nil {
			t.Fatalf("state within the grace: %v", err)
		}
		e.F.Clock.Advance(2 * time.Minute)
		if _, err := e.Svc.State(ctx, slug, s.BookingID, s.Token); !isNotFound(err) {
			t.Fatalf("state after the grace: %v", err)
		}
		if _, err := e.Svc.Document(ctx, slug, s.BookingID, s.Token, checkout.DocQRSVG); !isNotFound(err) {
			t.Fatalf("qr after the grace: %v", err)
		}
		// An abandoned hold: token dead SessionGrace after the hold.
		s2 := e.Create(e.Seat(e.F.Spots[1], "2027-07-10"))
		e.F.Clock.Advance(10*time.Minute + checkout.SessionGrace + time.Second)
		if _, err := e.Svc.State(ctx, slug, s2.BookingID, s2.Token); !isNotFound(err) {
			t.Fatalf("expired session token: %v", err)
		}

		sel := e.Seat(e.F.Spots[2], "2027-08-01")
		sel.PromoCode = "GUESS"
		var limited bool
		for i := 0; i < checkout.DefaultPromoAttemptsPerIP+1; i++ {
			_, err := e.Svc.Quote(ctx, slug, sel)
			var rl *auth.RateLimitError
			if errors.As(err, &rl) {
				limited = i == checkout.DefaultPromoAttemptsPerIP
				break
			}
			if err != nil {
				t.Fatalf("quote %d: %v", i, err)
			}
		}
		if !limited {
			t.Fatal("promo guessing not limited")
		}
		sel.PromoCode = ""
		if _, err := e.Svc.Quote(ctx, slug, sel); err != nil {
			t.Fatalf("quotes without a code are not limited: %v", err)
		}
	})
}
