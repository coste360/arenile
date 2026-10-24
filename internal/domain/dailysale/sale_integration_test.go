//go:build integration

package dailysale_test

import (
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func cents(v money.Cents) *money.Cents { return &v }

// TestDailySale_SpotFirstCash: a walk-in sale on a free spot creates a
// confirmed desk booking, the cash payment in the open session (change
// computed) and ticket #001 with the spot; the walk-in customer is created
// once; the spot is then taken; a wrong expected total and a cash sale
// without an open session are refused and roll everything back.
func TestDailySale_SpotFirstCash(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID

		// No open session: cash is refused, card would pass.
		if _, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0])); !errors.Is(err, cash.ErrSessionClosed) {
			t.Fatalf("cash without session: %v", err)
		}
		if n := f.Count((*store.Ticket)(nil), ""); n != 0 {
			t.Fatalf("tickets after refused sale: %d", n)
		}
		session := e.openSession("")

		q, err := e.sales.QuoteDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0]))
		if err != nil || !q.Available || q.Result.Breakdown.Total != bookingtest.BasePerDay || q.ZoneFree != nil {
			t.Fatalf("quote: %v %+v", err, q)
		}
		in := e.spotSale(f.Spots[0])
		in.Payment.Tendered = cents(5000)
		in.ExpectedTotal = cents(bookingtest.BasePerDay)
		in.LastName = " Rossi "
		res, err := e.sales.CreateDaily(ctx, f.Cashier, est, in)
		if err != nil {
			t.Fatal(err)
		}
		if res.Ticket.TicketNumber != 1 || res.Change == nil || *res.Change != 5000-bookingtest.BasePerDay || res.PaymentID == "" {
			t.Fatalf("result %+v", res)
		}
		v, err := e.tickets.Get(ctx, f.Viewer, est, res.Ticket.ID)
		if err != nil || v.NumberLabel != "#001" || v.State != ticketing.StateAssigned || v.SpotLabel != "A1" ||
			v.Ticket.HolderName != "Rossi" || v.Formula != "day" || v.BookingCode == "" || len(v.VAT) != 1 ||
			v.Ticket.CashSessionID == nil || *v.Ticket.CashSessionID != session.ID {
			t.Fatalf("view: %v %+v", err, v)
		}
		b, _ := e.bk.Get(ctx, f.Viewer, est, res.BookingID)
		if b.Booking.Status != string(booking.StatusConfirmed) || b.Booking.Channel != store.ChannelDesk ||
			b.Booking.PaidAmount != bookingtest.BasePerDay || b.Customer.LastName != dailysale.WalkInLastName {
			t.Fatalf("booking %+v customer %+v", b.Booking, b.Customer)
		}
		byCode, err := e.tickets.GetByCode(ctx, f.Viewer, est, " "+v.BookingCode+" ")
		if err != nil || byCode.Ticket.ID != res.Ticket.ID {
			t.Fatalf("by code: %v", err)
		}

		// The spot is taken now.
		if _, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0])); !errors.Is(err, availability.ErrSpotUnavailable) {
			t.Fatalf("taken spot: %v", err)
		}
		// A stale price is refused; nothing is written, the number is given back.
		in = e.spotSale(f.Spots[1])
		in.ExpectedTotal = cents(1)
		if _, err := e.sales.CreateDaily(ctx, f.Cashier, est, in); !errors.Is(err, ticketing.ErrPriceChanged) {
			t.Fatalf("price changed: %v", err)
		}
		// Card, half day: number 2, one walk-in customer for both sales.
		in = e.spotSale(f.Spots[1])
		in.Slots, in.Payment.Method = calendar.SlotsAM, store.TicketPaymentCard
		res2, err := e.sales.CreateDaily(ctx, f.Cashier, est, in)
		if err != nil || res2.Ticket.TicketNumber != 2 || res2.Ticket.TotalAmount != bookingtest.BasePerDay/2 {
			t.Fatalf("card half day: %v %+v", err, res2)
		}
		if n := f.Count((*store.Customer)(nil), "last_name = ?", dailysale.WalkInLastName); n != 1 {
			t.Fatalf("walk-in customers: %d", n)
		}
		// Viewers cannot sell; cash.operate is required besides bookings.write.
		if _, err := e.sales.CreateDaily(ctx, f.Viewer, est, e.spotSale(f.Spots[2])); err == nil {
			t.Fatal("viewer sold a ticket")
		}
		if _, err := e.sales.CreateDaily(ctx, f.Staff, est, e.spotSale(f.Spots[2])); err == nil {
			t.Fatal("beach staff without cash.operate sold a ticket")
		}
	})
}

// TestDailySale_ZoneFirst: zone-first on a seat zone holds the last free
// spot provisionally (needs assignment), on a zone-mode zone consumes the
// implicit pool (pending line); the zone is never oversold.
func TestDailySale_ZoneFirst(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		e.openSession("")
		zoneSale := func(zoneID string) dailysale.SaleInput {
			in := e.spotSale(f.Spots[0])
			in.Target = dailysale.Target{ZoneID: zoneID}
			return in
		}
		q, err := e.sales.QuoteDaily(ctx, f.Cashier, est, zoneSale(f.SeatZone.ID))
		if err != nil || !q.Available || q.ZoneFree == nil || *q.ZoneFree != 4 || q.Result.Breakdown.Total != bookingtest.BasePerDay {
			t.Fatalf("quote seat zone: %v %+v", err, q)
		}
		res, err := e.sales.CreateDaily(ctx, f.Cashier, est, zoneSale(f.SeatZone.ID))
		if err != nil {
			t.Fatal(err)
		}
		tk := res.Ticket
		if !tk.NeedsAssignment || tk.ProvisionalSpotID == nil || *tk.ProvisionalSpotID != f.Spots[3].ID {
			t.Fatalf("provisional spot: %+v", tk)
		}
		v, _ := e.tickets.Get(ctx, f.Viewer, est, tk.ID)
		if v.State != ticketing.StateUnassigned || v.SpotID != "" {
			t.Fatalf("unassigned view %+v", v)
		}
		q, _ = e.sales.QuoteDaily(ctx, f.Cashier, est, zoneSale(f.SeatZone.ID))
		if *q.ZoneFree != 3 {
			t.Fatalf("zone free after one sale: %d", *q.ZoneFree)
		}
		// Zone mode: two places, the third sale is zone_full.
		for i := range 2 {
			r, err := e.sales.CreateDaily(ctx, f.Cashier, est, zoneSale(f.FreeZone.ID))
			if err != nil || !r.Ticket.NeedsAssignment || r.Ticket.ProvisionalSpotID != nil {
				t.Fatalf("zone mode sale %d: %v %+v", i, err, r)
			}
			lines, _ := store.ListBookingLines(ctx, db, e.es, r.BookingID)
			if len(lines) != 1 || lines[0].AssignmentStatus != store.AssignmentPending {
				t.Fatalf("pending line %+v", lines)
			}
		}
		if _, err := e.sales.CreateDaily(ctx, f.Cashier, est, zoneSale(f.FreeZone.ID)); !errors.Is(err, ticketing.ErrZoneFull) {
			t.Fatalf("zone mode full: %v", err)
		}
		// Zone-first can be disabled.
		if _, err := e.sales.UpdateSettings(ctx, f.Manager, est, dailysale.SettingsInput{PrintFormat: store.PrintThermal58, ZoneFirstEnabled: false}); err != nil {
			t.Fatal(err)
		}
		var ve *configerr.ValidationError
		if _, err := e.sales.CreateDaily(ctx, f.Cashier, est, zoneSale(f.SeatZone.ID)); !errors.As(err, &ve) {
			t.Fatalf("zone-first disabled: %v", err)
		}
	})
}

// TestTicketing_VoidKeepsNumberAndRefunds: voiding a cash ticket cancels the
// booking (spot freed), records the cash refund in the session and keeps
// the number; the next sale takes the next number; a second void is
// ticket_voided; the day list and its CSV show the voided ticket, the
// summary leaves it out of the totals; cash session and corrispettivi
// include the tickets and net out the refund.
func TestTicketing_VoidKeepsNumberAndRefunds(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		session := e.openSession("")
		first, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0]))
		if err != nil {
			t.Fatal(err)
		}
		in := e.spotSale(f.Spots[1])
		in.Payment.Method = store.TicketPaymentCard
		second, err := e.sales.CreateDaily(ctx, f.Cashier, est, in)
		if err != nil {
			t.Fatal(err)
		}
		// The cashier has refunds.issue? The void needs it unless refund=none.
		if _, err := e.tickets.Void(ctx, f.Staff, est, first.Ticket.ID, ticketing.VoidInput{Reason: "errore"}); err == nil {
			t.Fatal("beach staff voided with refund")
		}
		v, err := e.tickets.Void(ctx, f.Manager, est, first.Ticket.ID, ticketing.VoidInput{Reason: "cliente andato via"})
		if err != nil {
			t.Fatal(err)
		}
		if v.State != ticketing.StateVoided || v.Ticket.TicketNumber != 1 || v.Ticket.RefundPaymentID == nil || v.Ticket.VoidReason != "cliente andato via" {
			t.Fatalf("voided view %+v", v.Ticket)
		}
		refund, err := store.GetPayment(ctx, db, e.es, *v.Ticket.RefundPaymentID)
		if err != nil || refund.Direction != store.PaymentDirectionRefund || refund.Method != store.PaymentMethodCash ||
			refund.Amount != bookingtest.BasePerDay || refund.CashSessionID == nil || *refund.CashSessionID != session.ID {
			t.Fatalf("refund %v %+v", err, refund)
		}
		b, _ := e.bk.Get(ctx, f.Viewer, est, first.BookingID)
		if b.Booking.Status != string(booking.StatusCancelled) || len(b.SpotAllocations) != 0 || b.Booking.PaidAmount != 0 {
			t.Fatalf("cancelled booking %+v allocations %d", b.Booking, len(b.SpotAllocations))
		}
		if _, err := e.tickets.Void(ctx, f.Manager, est, first.Ticket.ID, ticketing.VoidInput{Reason: "ancora"}); !errors.Is(err, ticketing.ErrVoided) {
			t.Fatalf("second void: %v", err)
		}
		// Credit to the walk-in customer is refused.
		var ve *configerr.ValidationError
		if _, err := e.tickets.Void(ctx, f.Manager, est, second.Ticket.ID, ticketing.VoidInput{Reason: "x", Refund: ticketing.VoidCredit}); !errors.As(err, &ve) {
			t.Fatalf("credit to walk-in: %v", err)
		}
		// The freed spot sells again with number 3: voids keep their number.
		third, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0]))
		if err != nil || third.Ticket.TicketNumber != 3 {
			t.Fatalf("resale: %v %+v", err, third)
		}

		l, err := e.tickets.List(ctx, f.Viewer, est, ticketing.ListInput{Date: bookingtest.D(day)})
		if err != nil || len(l.Items) != 3 {
			t.Fatalf("list: %v %+v", err, l)
		}
		s := l.Summary
		if s.Issued != 2 || s.Voided != 1 || s.Total != 2*bookingtest.BasePerDay || len(s.ByMethod) != 2 || len(s.VAT) != 1 {
			t.Fatalf("summary %+v", s)
		}
		page, _ := e.tickets.List(ctx, f.Viewer, est, ticketing.ListInput{Date: bookingtest.D(day), Limit: 2})
		if len(page.Items) != 2 || page.NextCursor != "2" {
			t.Fatalf("page %+v", page)
		}
		rest, _ := e.tickets.List(ctx, f.Viewer, est, ticketing.ListInput{Date: bookingtest.D(day), Limit: 2, Cursor: page.NextCursor})
		if len(rest.Items) != 1 || rest.Items[0].Ticket.TicketNumber != 3 || rest.NextCursor != "" {
			t.Fatalf("second page %+v", rest)
		}
		voided, _ := e.tickets.List(ctx, f.Viewer, est, ticketing.ListInput{Date: bookingtest.D(day), State: ticketing.StateVoided})
		if len(voided.Items) != 1 || voided.Items[0].Ticket.ID != first.Ticket.ID {
			t.Fatalf("voided filter %+v", voided.Items)
		}

		// Cash session: two cash sales minus one cash refund, card apart.
		d, err := e.cash.Get(ctx, f.Cashier, est, session.ID)
		if err != nil || d.Summary.Takings[store.PaymentMethodCash] != bookingtest.BasePerDay ||
			d.Summary.Takings[store.PaymentMethodCard] != bookingtest.BasePerDay {
			t.Fatalf("session takings %v %+v", err, d.Summary.Takings)
		}
		// Corrispettivi by VAT rate include the tickets (refund negative).
		rep, err := e.fiscal.Corrispettivi(ctx, f.Owner, est, fiscal.CorrispettiviQuery{From: bookingtest.D(day), To: bookingtest.D(day)})
		if err != nil || rep.GrandTotal != 2*bookingtest.BasePerDay || len(rep.Totals) != 1 || rep.Totals[0].RateBP != 2200 {
			t.Fatalf("corrispettivi %v %+v", err, rep)
		}
		// Reprint counts and marks the receipt.
		pd, err := e.tickets.Print(ctx, f.Viewer, est, second.Ticket.ID, "")
		if err != nil || pd.Reprint || pd.Format != store.PrintThermal80 || pd.QRPayload == "" || pd.Establishment.Name != f.Est.Name {
			t.Fatalf("print %v %+v", err, pd)
		}
		pd, err = e.tickets.Reprint(ctx, f.Cashier, est, second.Ticket.ID, store.PrintA5)
		if err != nil || !pd.Reprint || pd.Format != store.PrintA5 || pd.View.Ticket.PrintCount != 1 {
			t.Fatalf("reprint %v %+v", err, pd)
		}
	})
}

// TestDailySale_DayRolloverInEstablishmentTimezone: numbers restart at 1
// on the next day of the establishment timezone (Europe/Rome), even when
// the UTC date has not changed yet; a past day cannot be sold.
func TestDailySale_DayRolloverInEstablishmentTimezone(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		e.openSession("")
		// Morning of day (newEnv): numbers 1 and 2.
		for i, sp := range f.Spots[:2] {
			r, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(sp))
			if err != nil || r.Ticket.TicketNumber != int64(i+1) {
				t.Fatalf("sale %d on day: %v %+v", i, err, r)
			}
		}
		f.Clock.Set(time.Date(2027, 7, 10, 22, 30, 0, 0, time.UTC)) // 00:30 on 2027-07-11 in Rome, still 07-10 in UTC
		var ve *configerr.ValidationError
		if _, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[2])); !errors.As(err, &ve) {
			t.Fatalf("sale for a past day: %v", err)
		}
		in := e.spotSale(f.Spots[0])
		in.Date = bookingtest.D("2027-07-11")
		r, err := e.sales.CreateDaily(ctx, f.Cashier, est, in)
		if err != nil || r.Ticket.TicketNumber != 1 || !r.Ticket.TicketDate.Equal(bookingtest.D("2027-07-11")) {
			t.Fatalf("first sale of the new day: %v %+v", err, r)
		}
	})
}

// TestEntrySale_GateAndCapacity: entry tickets share the daily numbering,
// consume the quantity pool (never beyond its capacity) and enter once at
// the gate; voided tickets and tickets of another day are not admitted.
func TestEntrySale_GateAndCapacity(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		e.openSession("")
		entry := func(adults, children int) dailysale.EntryInput {
			in := dailysale.EntryInput{Date: bookingtest.D(day), Slots: "FULL", Payment: dailysale.SalePayment{Method: store.TicketPaymentCash}}
			if adults > 0 {
				in.Items = append(in.Items, dailysale.EntryItem{TypeID: e.adult.ID, Quantity: adults})
			}
			if children > 0 {
				in.Items = append(in.Items, dailysale.EntryItem{TypeID: e.child.ID, Quantity: children})
			}
			return in
		}
		q, err := e.sales.QuoteEntry(ctx, f.Cashier, est, entry(2, 1))
		if err != nil || q.Result.Breakdown.Total != 2*adultPerDay+childPerDay || q.Remaining != gateCapacity || !q.Available {
			t.Fatalf("quote: %v %+v", err, q)
		}
		spot, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0]))
		if err != nil {
			t.Fatal(err)
		}
		res, err := e.sales.CreateEntry(ctx, f.Cashier, est, entry(2, 1))
		if err != nil {
			t.Fatal(err)
		}
		if res.Ticket.TicketNumber != spot.Ticket.TicketNumber+1 || res.Ticket.Kind != store.TicketKindEntry || res.Ticket.TotalAmount != 2*adultPerDay+childPerDay {
			t.Fatalf("entry ticket %+v", res.Ticket)
		}
		v, _ := e.tickets.Get(ctx, f.Viewer, est, res.Ticket.ID)
		if v.State != ticketing.StateIssued || len(v.EntryItems) != 2 || v.EntryItems[0].ID != e.adult.ID || v.EntryItems[0].Quantity != 2 {
			t.Fatalf("entry view %+v", v)
		}
		// The pool is full (3 of 3): one more adult is refused.
		if _, err := e.sales.CreateEntry(ctx, f.Cashier, est, entry(1, 0)); !errors.Is(err, availability.ErrPoolExhausted) {
			t.Fatalf("pool full: %v", err)
		}
		// The QR carries a random gate token (INV-84): the sequential booking
		// code or the printed number alone never admit, not even a valid one.
		token := res.Ticket.GateToken
		if len(token) < 20 || v.BookingCode == "" {
			t.Fatalf("gate token %q, code %q", token, v.BookingCode)
		}
		pd, err := e.tickets.Print(ctx, f.Viewer, est, res.Ticket.ID, "")
		if err != nil || pd.QRPayload != token {
			t.Fatalf("print QR %v %+v", err, pd)
		}
		for _, guess := range []string{v.BookingCode, "2", "#002"} {
			if _, err := e.tickets.CheckIn(ctx, f.Staff, est, guess, false); !errors.Is(err, ticketing.ErrNotFound) {
				t.Fatalf("admitted by %q alone: %v", guess, err)
			}
		}
		// Gate: admitted once, then already entered (also by a confirmed
		// manual entry of the printed number).
		g, err := e.tickets.CheckIn(ctx, f.Staff, est, token, false)
		if err != nil || g.Outcome != ticketing.GateAdmitted || g.View.State != ticketing.StateEntered {
			t.Fatalf("gate: %v %+v", err, g)
		}
		g, _ = e.tickets.CheckIn(ctx, f.Staff, est, token, false)
		if g.Outcome != ticketing.GateAlreadyEntered {
			t.Fatalf("second scan %+v", g)
		}
		if g, err = e.tickets.CheckIn(ctx, f.Staff, est, "#002", true); err != nil || g.Outcome != ticketing.GateAlreadyEntered {
			t.Fatalf("manual number %v %+v", err, g)
		}
		if g, err = e.tickets.CheckIn(ctx, f.Staff, est, v.BookingCode, true); err != nil || g.Outcome != ticketing.GateAlreadyEntered {
			t.Fatalf("manual booking code %v %+v", err, g)
		}
		if _, err := e.tickets.Void(ctx, f.Manager, est, res.Ticket.ID, ticketing.VoidInput{Reason: "x"}); !errors.Is(err, ticketing.ErrAlreadyEntered) {
			t.Fatalf("void entered: %v", err)
		}
		// Sale tickets are not entry tickets; unknown codes are not found.
		var ve *configerr.ValidationError
		if _, err := e.tickets.CheckIn(ctx, f.Staff, est, spot.Ticket.GateToken, false); !errors.As(err, &ve) {
			t.Fatalf("sale ticket at the gate: %v", err)
		}
		if _, err := e.tickets.CheckIn(ctx, f.Staff, est, "XX-2027-999999", true); !errors.Is(err, ticketing.ErrNotFound) {
			t.Fatalf("unknown code: %v", err)
		}
		// A voided entry ticket (sold for another day: today is full) is refused.
		later := entry(0, 1)
		later.Date = bookingtest.D("2027-07-12")
		lt := mustEntry(t, e, later)
		if _, err := e.tickets.Void(ctx, f.Manager, est, lt.Ticket.ID, ticketing.VoidInput{Reason: "x", Refund: ticketing.VoidNone}); err != nil {
			t.Fatal(err)
		}
		g, _ = e.tickets.CheckIn(ctx, f.Staff, est, lt.Ticket.GateToken, false)
		if g.Outcome != ticketing.GateVoided {
			t.Fatalf("voided at gate %+v", g)
		}
		// A ticket of tomorrow is wrong_date today.
		tomorrow := entry(0, 1)
		tomorrow.Date = bookingtest.D("2027-07-11")
		rt, err := e.sales.CreateEntry(ctx, f.Cashier, est, tomorrow)
		if err != nil {
			t.Fatal(err)
		}
		if g, _ = e.tickets.CheckIn(ctx, f.Staff, est, rt.Ticket.GateToken, false); g.Outcome != ticketing.GateWrongDate {
			t.Fatalf("tomorrow at gate %+v", g)
		}
		// The booking behind a ticket is cancelled only by the void: the
		// generic cancellation is refused (ticket_owned), so the ticket and
		// its gate never drift apart (INV-84, INV-18).
		if _, err := e.bk.Cancel(ctx, f.Manager, est, rt.BookingID, booking.CancelInput{Reason: "fuori dal biglietto"}); !errors.Is(err, booking.ErrTicketOwned) {
			t.Fatalf("generic cancel of a ticket booking: %v", err)
		}
		if _, err := e.bk.Update(ctx, f.Manager, est, rt.BookingID, nil, booking.PatchInput{StartsOn: ptrDate("2027-07-12")}); !errors.Is(err, booking.ErrTicketOwned) {
			t.Fatalf("generic modify of a ticket booking: %v", err)
		}
	})
}

// TestTicketing_SummaryIsByDayOfUse: the day summary counts the tickets
// valid on the day (ticket_date), not the money taken on it. An advance
// sale made today for a later day sits in that day's summary and in today's
// cash session and corrispettivi; a void without refund leaves the summary
// but not the takings. The cash desk and the corrispettivi reconcile the
// money; the ticket summary is a by-day-of-use view (spec 14.3).
func TestTicketing_SummaryIsByDayOfUse(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		session := e.openSession("")
		todaySale, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0]))
		if err != nil {
			t.Fatal(err)
		}
		advance := e.spotSale(f.Spots[1])
		advance.Date = bookingtest.D("2027-07-12")
		adv, err := e.sales.CreateDaily(ctx, f.Cashier, est, advance)
		if err != nil || adv.Ticket.TicketNumber != 1 || !adv.Ticket.TicketDate.Equal(bookingtest.D("2027-07-12")) {
			t.Fatalf("advance sale takes the numbering of its day: %v %+v", err, adv.Ticket)
		}
		summary := func(d string) ticketing.Summary {
			l, err := e.tickets.List(ctx, f.Viewer, est, ticketing.ListInput{Date: bookingtest.D(d)})
			if err != nil {
				t.Fatal(err)
			}
			return l.Summary
		}
		if s := summary(day); s.Issued != 1 || s.Total != bookingtest.BasePerDay {
			t.Fatalf("today's summary %+v", s)
		}
		if s := summary("2027-07-12"); s.Issued != 1 || s.Total != bookingtest.BasePerDay {
			t.Fatalf("later day's summary %+v", s)
		}
		// The money of both sales is in today's cash session.
		d, err := e.cash.Get(ctx, f.Cashier, est, session.ID)
		if err != nil || d.Summary.Takings[store.PaymentMethodCash] != 2*bookingtest.BasePerDay {
			t.Fatalf("session takings %v %+v", err, d.Summary.Takings)
		}
		// A void without refund leaves the day summary; the takings stay.
		if _, err := e.tickets.Void(ctx, f.Manager, est, todaySale.Ticket.ID, ticketing.VoidInput{Reason: "x", Refund: ticketing.VoidNone}); err != nil {
			t.Fatal(err)
		}
		if s := summary(day); s.Issued != 0 || s.Voided != 1 || s.Total != 0 {
			t.Fatalf("today's summary after the void %+v", s)
		}
		d, _ = e.cash.Get(ctx, f.Cashier, est, session.ID)
		if d.Summary.Takings[store.PaymentMethodCash] != 2*bookingtest.BasePerDay {
			t.Fatalf("takings after a void without refund %+v", d.Summary.Takings)
		}
		rep, err := e.fiscal.Corrispettivi(ctx, f.Owner, est, fiscal.CorrispettiviQuery{From: bookingtest.D(day), To: bookingtest.D(day)})
		if err != nil || rep.GrandTotal != 2*bookingtest.BasePerDay {
			t.Fatalf("corrispettivi of the day of payment %v %+v", err, rep)
		}
	})
}

func ptrDate(s string) *calendar.Date {
	d := bookingtest.D(s)
	return &d
}

func mustEntry(t *testing.T, e *env, in dailysale.EntryInput) *dailysale.Result {
	t.Helper()
	r, err := e.sales.CreateEntry(e.f.Ctx, e.f.Cashier, e.f.Est.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestTicketing_ListWholeDay: a negative limit lists the whole day (CSV
// export) and a void without refund is allowed to bookings.write.
func TestTicketing_ListWholeDay(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		e.openSession("")
		r, err := e.sales.CreateDaily(ctx, f.Cashier, est, e.spotSale(f.Spots[0]))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.tickets.Void(ctx, f.Manager, est, r.Ticket.ID, ticketing.VoidInput{Reason: "prova", Refund: ticketing.VoidNone}); err != nil {
			t.Fatal(err)
		}
		l, err := e.tickets.List(ctx, f.Viewer, est, ticketing.ListInput{Date: bookingtest.D(day), Limit: -1})
		if err != nil || len(l.Items) != 1 || l.Items[0].State != ticketing.StateVoided {
			t.Fatalf("list %v %+v", err, l)
		}
	})
}
