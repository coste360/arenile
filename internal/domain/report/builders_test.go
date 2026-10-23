package report

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

var rome = mustLoc("Europe/Rome")

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

func per(from, to string) Period { return Period{From: d(from), To: d(to)} }

func ptr[T any](v T) *T { return &v }

func TestWeekdaySpan_Scenarios(t *testing.T) {
	tests := []struct {
		name         string
		from, to     string
		year         int
		wantF, wantT string
	}{
		// 2027-07-03 is a Saturday; 364 days earlier is Saturday 2026-07-04.
		{"previous year saturday", "2027-07-03", "2027-07-10", 0, "2026-07-04", "2026-07-11"},
		{"season", "2027-06-01", "2027-09-30", 0, "2026-06-02", "2026-10-01"},
		// 2028 is a leap year: 2028-03-04 (Sat) -> 2027-03-04 is a Thursday -> nearest Saturday 2027-03-06.
		{"after leap day", "2028-03-04", "2028-03-05", 0, "2027-03-06", "2027-03-07"},
		{"two years back", "2027-07-03", "2027-07-03", 2025, "2025-07-05", "2025-07-05"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := per(tt.from, tt.to)
			got := ComparisonSpan(p, tt.year, AlignWeekday)
			if got.From.String() != tt.wantF || got.To.String() != tt.wantT {
				t.Fatalf("got %s..%s, want %s..%s", got.From, got.To, tt.wantF, tt.wantT)
			}
			if got.From.ISOWeekday() != p.From.ISOWeekday() || got.Days() != p.Days() {
				t.Fatalf("weekday/length not preserved: %s (%d days)", got.From, got.Days())
			}
		})
	}
	if got := ComparisonSpan(per("2027-06-01", "2027-06-30"), 0, ""); got.From.String() != "2026-06-01" {
		t.Fatalf("default align must be calendar, got %s", got.From)
	}
}

func TestWeekdaySpan_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		from := d("2020-01-01").AddDays(rapid.IntRange(0, 3000).Draw(t, "from"))
		p := Period{From: from, To: from.AddDays(rapid.IntRange(0, 200).Draw(t, "len"))}
		year := rapid.IntRange(2005, 2035).Draw(t, "year")
		got := WeekdaySpan(p, year)
		anchor := PreviousSpan(p, year).From
		if got.From.ISOWeekday() != p.From.ISOWeekday() || got.Days() != p.Days() {
			t.Fatalf("weekday or length changed: %v -> %v", p, got)
		}
		if dist := anchor.DaysUntil(got.From); dist < -3 || dist > 3 {
			t.Fatalf("start %s is %d days from the calendar anchor %s", got.From, dist, anchor)
		}
	})
}

func TestDayWindow_DST(t *testing.T) {
	// 2027-03-28 is the spring DST switch in Rome: the day lasts 23 hours.
	from, to := dayWindow(per("2027-03-28", "2027-03-28"), rome)
	if from != time.Date(2027, 3, 27, 23, 0, 0, 0, time.UTC) || to != time.Date(2027, 3, 28, 22, 0, 0, 0, time.UTC) {
		t.Fatalf("window = %s .. %s", from, to)
	}
	if calendar.In(to.Add(-time.Nanosecond), rome).String() != "2027-03-28" || calendar.In(to, rome).String() != "2027-03-29" {
		t.Fatal("window bounds do not match the local calendar day")
	}
}

// ---- occupancy ----------------------------------------------------------------

func unit(date string, slot calendar.Slot, kind, zone, row string, n int64) store.ReportUnitCount {
	return store.ReportUnitCount{Date: d(date), Slot: slot, Kind: kind, ZoneID: zone, RowLabel: row, Units: n}
}

func occupancyFixture() occupancyInput {
	p := per("2027-07-01", "2027-07-03")
	cal := &store.ReportOpenCalendar{
		Seasons: []store.ReportSeasonSpan{{OpensOn: d("2027-05-01"), ClosesOn: d("2027-07-02")}}, // 07-03 outside the season
		Closures: []store.ClosureDay{
			{Date: d("2027-07-02"), Slots: []calendar.Slot{calendar.PM}}, // afternoon closed
		},
	}
	return occupancyInput{
		period: p,
		open:   openUnits(p, cal),
		zones: []store.ReportZone{
			{ID: "zb", Code: "B", Name: store.I18nText{"it": "Seconda fila"}, SortOrder: 2},
			{ID: "za", Code: "A", Name: store.I18nText{"it": "Prima fila"}, SortOrder: 1},
		},
		spots: []store.ReportSpotCount{
			{ZoneID: "za", RowLabel: "A", Spots: 10},
			{ZoneID: "zb", RowLabel: "B", Spots: 5},
		},
		allocs: []store.ReportUnitCount{
			unit("2027-07-01", calendar.AM, store.AllocationBooking, "za", "A", 6),
			unit("2027-07-01", calendar.PM, store.AllocationSeason, "za", "A", 4),
			unit("2027-07-01", calendar.AM, store.AllocationBlock, "zb", "B", 1),
			unit("2027-07-02", calendar.AM, store.AllocationBooking, "zb", "B", 5),
			unit("2027-07-02", calendar.PM, store.AllocationBooking, "zb", "B", 5), // closed slot: ignored
			unit("2027-07-03", calendar.AM, store.AllocationBooking, "za", "A", 3), // outside season: ignored
		},
		free: []store.ReportUnitCount{unit("2027-07-01", calendar.PM, "release", "za", "A", 2)},
	}
}

func TestOpenUnits_SeasonsAndClosures(t *testing.T) {
	in := occupancyFixture()
	want := map[string][2]bool{"2027-07-01": {true, true}, "2027-07-02": {true, false}}
	if len(in.open) != len(want) {
		t.Fatalf("open = %v", in.open)
	}
	for k, v := range want {
		if in.open[k] != v {
			t.Fatalf("open[%s] = %v, want %v", k, in.open[k], v)
		}
	}
}

func TestBuildOccupancy_Groupings(t *testing.T) {
	// Open units: 07-01 AM+PM, 07-02 AM -> 3 half-days x 15 spots = 45, minus 1 blocked.
	tests := []struct {
		by   OccupancyGroupBy
		keys []string
		occ  []int64
		cap  []int64
	}{
		{OccupancyByDay, []string{"2027-07-01", "2027-07-02", "2027-07-03"}, []int64{10, 5, 0}, []int64{29, 15, 0}},
		{OccupancyByZone, []string{"za", "zb"}, []int64{10, 5}, []int64{30, 14}},
		{OccupancyByRow, []string{"za:A", "zb:B"}, []int64{10, 5}, []int64{30, 14}},
		{OccupancyBySlot, []string{"AM", "PM"}, []int64{6 + 5, 4}, []int64{29, 15}},
	}
	for _, tt := range tests {
		t.Run(string(tt.by), func(t *testing.T) {
			o := buildOccupancy(occupancyFixture(), tt.by)
			if len(o.Rows) != len(tt.keys) {
				t.Fatalf("rows = %+v", o.Rows)
			}
			for i, r := range o.Rows {
				if r.Key != tt.keys[i] || r.OccupiedUnits != tt.occ[i] || r.CapacityUnits != tt.cap[i] {
					t.Fatalf("row %d = %+v, want key %s occ %d cap %d", i, r, tt.keys[i], tt.occ[i], tt.cap[i])
				}
				if r.OccupancyBP != Ratio(r.OccupiedUnits, r.CapacityUnits) {
					t.Fatalf("row %d bp = %d", i, r.OccupancyBP)
				}
			}
			tot := o.Totals
			if tot.CapacityUnits != 44 || tot.OccupiedUnits != 15 || tot.SeasonUnits != 4 || tot.BlockedUnits != 1 || tot.ReleasedUnits != 2 {
				t.Fatalf("totals = %+v", tot)
			}
			if tot.OccupancyBP != 3409 { // 15 / 44
				t.Fatalf("totals bp = %d", tot.OccupancyBP)
			}
		})
	}
	labels := buildOccupancy(occupancyFixture(), OccupancyByRow).Rows
	if labels[0].Label != "Prima fila · Fila A" || labels[0].ZoneID != "za" {
		t.Fatalf("row label = %+v", labels[0])
	}
	if l := buildOccupancy(occupancyFixture(), OccupancyByDay).Rows[0].Label; l != "01/07/2027" {
		t.Fatalf("day label = %s", l)
	}
}

// ---- revenue ------------------------------------------------------------------

func payRow(day, method, dir string, channel *string, shares ...store.VATShare) store.ReportPayment {
	var amount money.Cents
	for _, s := range shares {
		amount += s.Gross
	}
	return store.ReportPayment{BusinessDate: d(day), Method: method, Direction: dir, Amount: amount, VATBreakdown: shares, Channel: channel}
}

func share(rate int64, gross money.Cents) store.VATShare {
	tx, tax := money.VATSplit(gross, rate)
	return store.VATShare{RateBP: rate, Gross: gross, Taxable: tx, Tax: tax}
}

func TestBuildRevenueCollected_Scenarios(t *testing.T) {
	online, desk := ptr(store.ChannelOnline), ptr(store.ChannelDesk)
	pays := []store.ReportPayment{
		payRow("2027-07-01", store.PaymentMethodCard, store.PaymentDirectionIn, online, share(2200, 10000)),
		payRow("2027-07-01", store.PaymentMethodCash, store.PaymentDirectionIn, desk, share(2200, 3333), share(1000, 1111)),
		payRow("2027-07-02", store.PaymentMethodCash, store.PaymentDirectionRefund, desk, share(2200, 1000)),
		payRow("2027-07-02", store.PaymentMethodCash, store.PaymentDirectionIn, nil, share(1000, 500)),                      // counter sale
		{BusinessDate: d("2027-07-02"), Method: store.PaymentMethodOther, Direction: store.PaymentDirectionIn, Amount: 700}, // no split
		payRow("2027-08-01", store.PaymentMethodCard, store.PaymentDirectionIn, online, share(2200, 9999)),                  // outside
	}
	p := per("2027-07-01", "2027-07-31")
	tests := []struct {
		by    RevenueGroupBy
		keys  []string
		gross []money.Cents
	}{
		{RevenueByDay, []string{"2027-07-01", "2027-07-02"}, []money.Cents{14444, 200}},
		{RevenueByWeek, []string{"2027-W26"}, []money.Cents{14644}},
		{RevenueByMonth, []string{"2027-07"}, []money.Cents{14644}},
		{RevenueByChannel, []string{"desk", "online", "counter"}, []money.Cents{3444, 10000, 1200}},
		{RevenueByPaymentMethod, []string{"cash", "card", "other"}, []money.Cents{3944, 10000, 700}},
		{RevenueByVATRate, []string{"1000", "2200"}, []money.Cents{1611, 13033}},
	}
	for _, tt := range tests {
		t.Run(string(tt.by), func(t *testing.T) {
			r := buildRevenueCollected(p, pays, tt.by, 2200)
			if len(r.Rows) != len(tt.keys) {
				t.Fatalf("rows = %+v", r.Rows)
			}
			for i, row := range r.Rows {
				if row.Key != tt.keys[i] || row.Gross != tt.gross[i] {
					t.Fatalf("row %d = %+v, want %s %d", i, row, tt.keys[i], tt.gross[i])
				}
				if row.Taxable+row.Tax != row.Gross {
					t.Fatalf("row %d: taxable+tax != gross: %+v", i, row)
				}
			}
			if r.Totals.Gross != 14644 || r.Totals.Refunds != 1000 || r.Totals.Count != 5 {
				t.Fatalf("totals = %+v", r.Totals)
			}
		})
	}
}

// TestBuildRevenueCollected_ReconcilesWithCorrispettivi checks, on random
// ledgers, that the collected revenue grouped by day and by VAT rate adds
// up to the cent with the daily corrispettivi export: gross, taxable and tax
// per rate and in total.
func TestBuildRevenueCollected_ReconcilesWithCorrispettivi(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rates := []int64{0, 400, 1000, 2200}
		methods := []string{store.PaymentMethodCash, store.PaymentMethodCard, store.PaymentMethodBankTransfer}
		n := rapid.IntRange(0, 40).Draw(t, "n")
		var pays []store.ReportPayment
		var ledger []store.Payment
		for i := range n {
			day := d("2027-07-01").AddDays(rapid.IntRange(0, 9).Draw(t, "day"))
			method := rapid.SampledFrom(methods).Draw(t, "method")
			dir := store.PaymentDirectionIn
			if rapid.IntRange(0, 4).Draw(t, "refund") == 0 {
				dir = store.PaymentDirectionRefund
			}
			var shares []store.VATShare
			for range rapid.IntRange(1, 2).Draw(t, "shares") {
				shares = append(shares, share(rapid.SampledFrom(rates).Draw(t, "rate"), money.Cents(rapid.Int64Range(1, 50000).Draw(t, "gross"))))
			}
			pr := payRow(day.String(), method, dir, nil, shares...)
			pays = append(pays, pr)
			ledger = append(ledger, store.Payment{
				ID: string(rune('a' + i)), Status: store.PaymentSucceeded, Method: method, Direction: dir,
				Amount: pr.Amount, BusinessDate: day, VATBreakdown: shares,
			})
		}
		p := per("2027-07-01", "2027-07-10")
		corr := fiscal.BuildReport(ledger, p.From, p.To, fiscal.GroupByDay, "EUR", 2200)
		byRate := buildRevenueCollected(p, pays, RevenueByVATRate, 2200)
		byDay := buildRevenueCollected(p, pays, RevenueByDay, 2200)
		if byRate.Totals.Gross != corr.GrandTotal || byDay.Totals.Gross != corr.GrandTotal {
			t.Fatalf("gross %d / %d, corrispettivi %d", byRate.Totals.Gross, byDay.Totals.Gross, corr.GrandTotal)
		}
		var corrTax, corrTaxable money.Cents
		for _, rt := range corr.Totals {
			corrTax += rt.Tax
			corrTaxable += rt.Taxable
			found := false
			for _, row := range byRate.Rows {
				if row.Key == FormatCount(rt.RateBP) {
					found = true
					if row.Gross != rt.Gross || row.Taxable != rt.Taxable || row.Tax != rt.Tax {
						t.Fatalf("rate %d: report %+v, corrispettivi %+v", rt.RateBP, row, rt)
					}
				}
			}
			if !found && rt.Gross != 0 {
				t.Fatalf("rate %d missing", rt.RateBP)
			}
		}
		if byDay.Totals.Tax != corrTax || byDay.Totals.Taxable != corrTaxable {
			t.Fatalf("by day tax %d/%d, corrispettivi %d/%d", byDay.Totals.Taxable, byDay.Totals.Tax, corrTaxable, corrTax)
		}
	})
}

func confirmedBooking(id, channel, status string, confirmed time.Time, total money.Cents, snapshot string) store.ReportBooking {
	return store.ReportBooking{
		ID: id, Channel: channel, Status: status, TotalAmount: total, ConfirmedAt: &confirmed, CreatedAt: confirmed,
		PriceSnapshot: store.JSON(snapshot), StartsOn: calendar.In(confirmed, rome),
	}
}

func TestBuildRevenueBooked_Scenarios(t *testing.T) {
	mixed := `{"vat":[{"rate":22,"taxable":8197,"tax":1803},{"rate":10,"taxable":4545,"tax":455}]}`
	bks := []store.ReportBooking{
		// 23:30 UTC on 30 June is 1 July in Rome.
		confirmedBooking("b1", store.ChannelOnline, "confirmed", time.Date(2027, 6, 30, 23, 30, 0, 0, time.UTC), 15000, mixed),
		confirmedBooking("b2", store.ChannelDesk, "completed", time.Date(2027, 7, 2, 9, 0, 0, 0, time.UTC), 5000, `{}`),
		confirmedBooking("b3", store.ChannelDesk, "cancelled", time.Date(2027, 7, 2, 9, 0, 0, 0, time.UTC), 7000, `{}`),
		confirmedBooking("b4", store.ChannelDesk, "confirmed", time.Date(2027, 7, 31, 22, 30, 0, 0, time.UTC), 1000, `{}`), // 1 Aug in Rome
	}
	p := per("2027-07-01", "2027-07-31")
	byDay := buildRevenueBooked(p, bks, RevenueByDay, rome, 2200)
	if len(byDay.Rows) != 2 || byDay.Rows[0].Key != "2027-07-01" || byDay.Rows[0].Gross != 15000 || byDay.Rows[1].Gross != 5000 {
		t.Fatalf("by day = %+v", byDay.Rows)
	}
	if byDay.Totals.Gross != 20000 || byDay.Totals.Count != 2 || byDay.Totals.Refunds != 0 {
		t.Fatalf("totals = %+v", byDay.Totals)
	}
	byRate := buildRevenueBooked(p, bks, RevenueByVATRate, rome, 2200)
	if len(byRate.Rows) != 2 || byRate.Rows[0].Key != "1000" || byRate.Rows[1].Key != "2200" {
		t.Fatalf("by rate = %+v", byRate.Rows)
	}
	if byRate.Rows[0].Gross+byRate.Rows[1].Gross != 20000 || byRate.Rows[0].Gross != 5000 {
		t.Fatalf("by rate gross = %+v", byRate.Rows)
	}
	for _, r := range byRate.Rows {
		if r.Taxable+r.Tax != r.Gross {
			t.Fatalf("split = %+v", r)
		}
	}
	cat := buildRevenueCatalog(p, RevenueByPack, []store.ReportCatalogRevenue{
		{Code: "STD", Name: store.I18nText{"it": "Standard"}, Count: 3, Gross: 12200},
		{Code: "VIP", Name: store.I18nText{}, VATRateBP: ptr(int64(1000)), Count: 1, Gross: 1100},
	}, 2200)
	if cat.Rows[0].Label != "Standard" || cat.Rows[0].Tax != 2200 || cat.Rows[1].Label != "VIP" || cat.Rows[1].Tax != 100 || cat.Totals.Gross != 13300 {
		t.Fatalf("catalog = %+v", cat)
	}
}

func TestValidateRevenue_Combinations(t *testing.T) {
	tests := []struct {
		by    RevenueGroupBy
		basis RevenueBasis
		ok    bool
	}{
		{RevenueByPaymentMethod, BasisCollected, true},
		{RevenueByPaymentMethod, BasisBooked, false},
		{RevenueByPack, BasisBooked, true},
		{RevenueByExtra, BasisCollected, false},
		{RevenueByVATRate, BasisBooked, true},
		{"nope", BasisCollected, false},
		{RevenueByDay, "nope", false},
	}
	for _, tt := range tests {
		if err := validateRevenue(RevenueQuery{GroupBy: tt.by, Basis: tt.basis}); (err == nil) != tt.ok {
			t.Fatalf("%s/%s: err = %v", tt.by, tt.basis, err)
		}
	}
}

// ---- bookings, cancellations, passes --------------------------------------------

func sold(created time.Time, starts string, channel, formula, status string, total, paid, refunded money.Cents) store.ReportBooking {
	return store.ReportBooking{
		CreatedAt: created, StartsOn: d(starts), Channel: channel, Formula: formula, Status: status,
		TotalAmount: total, PaidAmount: paid, Refunded: refunded,
	}
}

func TestBuildBookings_Scenarios(t *testing.T) {
	at := func(day string) time.Time { return time.Date(2027, 7, d(day).Day(), 10, 0, 0, 0, time.UTC) }
	bks := []store.ReportBooking{
		sold(at("2027-07-01"), "2027-07-01", store.ChannelDesk, "day", "confirmed", 3000, 0, 0),
		sold(at("2027-07-01"), "2027-07-05", store.ChannelOnline, "multi_day", "confirmed", 9000, 0, 0),
		sold(at("2027-07-02"), "2027-09-30", store.ChannelOnline, "season", "pending_payment", 90000, 0, 0),
		sold(at("2027-07-02"), "2027-06-30", store.ChannelDesk, "day", "cancelled", 1000, 0, 0),                            // negative lead -> 0
		sold(time.Date(2027, 7, 3, 22, 30, 0, 0, time.UTC), "2027-07-04", store.ChannelAPI, "day", "confirmed", 500, 0, 0), // 4 July in Rome
	}
	p := per("2027-07-01", "2027-07-03")
	ch := buildBookings(p, bks, BookingsByChannel, rome)
	if len(ch.Rows) != 2 || ch.Rows[0].Key != "desk" || ch.Rows[0].Bookings != 2 || ch.Rows[1].Key != "online" || ch.Rows[1].Value != 99000 {
		t.Fatalf("by channel = %+v", ch.Rows)
	}
	if ch.Rows[1].ShareBP != 5000 || ch.Rows[1].AverageValue != 49500 || ch.Totals.Bookings != 4 || ch.Totals.ShareBP != 10000 {
		t.Fatalf("shares = %+v / %+v", ch.Rows[1], ch.Totals)
	}
	// leads: 0, 4, 90, 0 -> average 23
	if ch.Totals.AverageLeadDays != 23 {
		t.Fatalf("average lead = %d", ch.Totals.AverageLeadDays)
	}
	lead := buildBookings(p, bks, BookingsByLeadTime, rome)
	if len(lead.Rows) != len(LeadTimeBuckets) || lead.Rows[0].Bookings != 2 || lead.Rows[2].Bookings != 1 || lead.Rows[6].Bookings != 1 {
		t.Fatalf("by lead = %+v", lead.Rows)
	}
	day := buildBookings(p, bks, BookingsByDay, rome)
	if len(day.Rows) != 3 || day.Rows[2].Bookings != 0 || day.Rows[0].Bookings != 2 {
		t.Fatalf("by day = %+v", day.Rows)
	}
	f := buildBookings(p, bks, BookingsByFormula, rome)
	if f.Rows[0].Key != "day" || f.Rows[len(f.Rows)-1].Key != "season" {
		t.Fatalf("by formula order = %+v", f.Rows)
	}
}

func TestBuildCancellations_Scenarios(t *testing.T) {
	now := time.Date(2027, 6, 1, 10, 0, 0, 0, time.UTC)
	bks := []store.ReportBooking{
		sold(now, "2027-07-01", store.ChannelDesk, "day", "completed", 3000, 3000, 0),
		sold(now, "2027-07-01", store.ChannelOnline, "day", "cancelled", 10000, 3000, 7000), // 30% penalty kept
		sold(now, "2027-07-15", store.ChannelOnline, "day", "no_show", 2000, 2000, 0),
		sold(now, "2027-08-02", store.ChannelOnline, "day", "cancelled", 5000, 0, 0),
		sold(now, "2027-09-01", store.ChannelOnline, "day", "cancelled", 5000, 0, 0), // outside
	}
	r := buildCancellations(per("2027-07-01", "2027-08-31"), bks, CancellationsByMonth)
	if len(r.Rows) != 2 || r.Rows[0].Key != "2027-07" || r.Rows[0].Label != "Luglio 2027" {
		t.Fatalf("rows = %+v", r.Rows)
	}
	jul := r.Rows[0]
	if jul.Bookings != 3 || jul.Cancelled != 1 || jul.NoShows != 1 || jul.CancelledBP != 3333 || jul.LostValue != 7000 || jul.Refunded != 7000 {
		t.Fatalf("july = %+v", jul)
	}
	if r.Totals.Bookings != 4 || r.Totals.Cancelled != 2 || r.Totals.LostValue != 12000 || r.Totals.CancelledBP != 5000 || r.Totals.NoShowBP != 2500 {
		t.Fatalf("totals = %+v", r.Totals)
	}
	byCh := buildCancellations(per("2027-07-01", "2027-08-31"), bks, CancellationsByChannel)
	if byCh.Rows[0].Key != "desk" || byCh.Rows[1].Key != "online" {
		t.Fatalf("by channel = %+v", byCh.Rows)
	}
}

func TestResaleShares_Prorate(t *testing.T) {
	l1, l2 := ptr("l1"), ptr("l2")
	rels := []store.ReportRelease{
		{ResoldLineID: l1, LineTotal: 1000, LineUnits: 3},
		{ResoldLineID: l1, LineTotal: 1000, LineUnits: 3},
		{},
		{ResoldLineID: l1, LineTotal: 1000, LineUnits: 3},
		{ResoldLineID: l2, LineTotal: 999, LineUnits: 4}, // only one of the four units was a release
	}
	got := resaleShares(rels)
	want := []money.Cents{334, 333, 0, 333, 250}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shares = %v, want %v", got, want)
		}
	}
}

func TestBuildSeasonPasses_Scenarios(t *testing.T) {
	conf := time.Date(2027, 5, 10, 9, 0, 0, 0, time.UTC)
	bks := []store.ReportBooking{
		{IsSeasonPass: true, Status: "confirmed", ConfirmedAt: &conf, TotalAmount: 150000},
		{IsSeasonPass: true, Status: "confirmed", ConfirmedAt: &conf, TotalAmount: 120000, RenewalOf: ptr("old")},
		{IsSeasonPass: true, Status: "cancelled", ConfirmedAt: &conf, TotalAmount: 99999},
		{IsSeasonPass: false, Status: "confirmed", ConfirmedAt: &conf, TotalAmount: 1000},
	}
	line := ptr("line")
	rels := []store.ReportRelease{
		{Date: d("2027-05-20"), Slot: calendar.AM, ZoneID: "za", CreditAmount: 500, ResoldLineID: line, LineTotal: 2000, LineUnits: 2},
		{Date: d("2027-05-20"), Slot: calendar.PM, ZoneID: "za", CreditAmount: 500, ResoldLineID: line, LineTotal: 2000, LineUnits: 2},
		{Date: d("2027-05-21"), Slot: calendar.AM, ZoneID: "zb", CreditAmount: 0},
	}
	zones := []store.ReportZone{{ID: "za", Code: "A", SortOrder: 1}, {ID: "zb", Code: "B", Name: store.I18nText{"it": "Palme"}, SortOrder: 2}}
	r := buildSeasonPasses(per("2027-05-01", "2027-05-31"), bks, rels, zones, SeasonPassesByZone, rome)
	if r.PassesSold != 2 || r.PassesValue != 270000 || r.Renewals != 1 {
		t.Fatalf("passes = %d %d %d", r.PassesSold, r.PassesValue, r.Renewals)
	}
	if len(r.Rows) != 2 || r.Rows[0].Label != "A" || r.Rows[1].Label != "Palme" {
		t.Fatalf("rows = %+v", r.Rows)
	}
	if r.Rows[0].ReleasedUnits != 2 || r.Rows[0].ResoldUnits != 2 || r.Rows[0].ResaleRevenue != 2000 || r.Rows[0].CreditsIssued != 1000 || r.Rows[0].ResaleBP != 10000 {
		t.Fatalf("zone A = %+v", r.Rows[0])
	}
	if r.Totals.ReleasedUnits != 3 || r.Totals.ResaleBP != 6666 {
		t.Fatalf("totals = %+v", r.Totals)
	}
}

func TestBuildTopCustomersAndBalances_DisplayNameOnly(t *testing.T) {
	anon := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	tc := buildTopCustomers(per("2027-07-01", "2027-07-31"), SortByRevenue, []store.ReportCustomer{
		{CustomerID: "c1", FirstName: " Mario ", LastName: "Rossi", Bookings: 3, Revenue: 50000, LastBookingOn: ptr(d("2027-07-20"))},
		{CustomerID: "c2", FirstName: "Anna", LastName: "Bianchi", AnonymizedAt: &anon, Bookings: 1, Revenue: 100},
	}, map[string]int{"c1": 12})
	if tc.Rows[0].Rank != 1 || tc.Rows[0].DisplayName != "Mario Rossi" || tc.Rows[0].SpotDays != 12 || tc.Rows[1].DisplayName != AnonymizedName {
		t.Fatalf("top = %+v", tc.Rows)
	}
	ob := buildOpenBalances(per("2027-07-01", "2027-07-31"), []store.ReportOpenBalance{
		{BookingID: "b1", FirstName: "Mario", LastName: "Rossi", TotalAmount: 10000, PaidAmount: 2500},
		{BookingID: "b2", LastName: "Verdi", TotalAmount: 3000, PaidAmount: 0},
	})
	if ob.TotalBalance != 10500 || ob.Rows[0].Balance != 7500 || ob.Rows[1].DisplayName != "Verdi" {
		t.Fatalf("balances = %+v", ob)
	}
}

// ---- facts, summary, comparison ---------------------------------------------------

func TestBuildFactsAndSummary_UsesReportDefinitions(t *testing.T) {
	p := per("2027-07-01", "2027-07-02")
	occ := buildOccupancy(occupancyFixture(), OccupancyByDay)
	conf := time.Date(2027, 7, 1, 8, 0, 0, 0, time.UTC)
	sf := buildFacts(factsInput{
		period: p, occupancy: occ, loc: rome,
		payments: []store.ReportPayment{
			payRow("2027-07-01", store.PaymentMethodCash, store.PaymentDirectionIn, nil, share(2200, 5000)),
			payRow("2027-07-02", store.PaymentMethodCash, store.PaymentDirectionRefund, nil, share(2200, 1000)),
		},
		bookings: []store.ReportBooking{
			{Status: "confirmed", CreatedAt: conf, ConfirmedAt: &conf, StartsOn: d("2027-07-02"), TotalAmount: 4000, IsSeasonPass: true},
			{Status: "cancelled", CreatedAt: conf, StartsOn: d("2027-07-01"), TotalAmount: 2001},
			{Status: "no_show", CreatedAt: conf.AddDate(0, -1, 0), StartsOn: d("2027-07-02"), TotalAmount: 999},
		},
		releases: []store.ReportRelease{{Date: d("2027-07-01"), ResoldLineID: ptr("l")}, {Date: d("2027-07-02")}},
	})
	s := buildSummary(sf, &OpenBalances{TotalBalance: 700, Rows: []OpenBalanceRow{{}}}, conf)
	if s.CapacityUnits != 44 || s.OccupiedUnits != 15 || s.RevenueCollected != 4000 || s.RevenueBooked != 4000 {
		t.Fatalf("summary money/occupancy = %+v", s)
	}
	if s.Bookings != 2 || s.AverageBookingValue != 3000 || s.Cancellations != 1 || s.NoShows != 1 || s.SeasonPassesSold != 1 {
		t.Fatalf("summary counts = %+v", s)
	}
	if s.OpenBalanceTotal != 700 || s.OpenBalanceBookings != 1 || !s.GeneratedAt.Equal(conf) {
		t.Fatalf("summary balances = %+v", s)
	}
	if t2 := sf.all(); t2.released != 2 || t2.resold != 1 {
		t.Fatalf("releases = %+v", t2)
	}
}

func spanOf(p Period, perDay func(i int) dayFacts) *spanFacts {
	sf := &spanFacts{period: p, days: make([]dayFacts, p.Days())}
	for i := range sf.days {
		sf.days[i] = perDay(i)
	}
	return sf
}

func TestBuildComparison_MetricsAndSeries(t *testing.T) {
	cur := spanOf(per("2028-02-26", "2028-03-05"), func(i int) dayFacts { // 9 days, includes 29 Feb
		return dayFacts{capacity: 100, occupied: 50, collected: 1000, bookings: 2, bookingsValue: 3000}
	})
	prevP := PreviousSpan(cur.period, 0) // 2027-02-26 .. 2027-03-05: 8 days
	prev := spanOf(prevP, func(i int) dayFacts {
		return dayFacts{capacity: 100, occupied: 25, collected: 500, bookings: 1, bookingsValue: 1000}
	})
	c := buildComparison(cur, prev, GranularityWeek, AlignCalendar)
	m := map[string]ComparisonMetric{}
	for _, x := range c.Metrics {
		m[x.Metric] = x
	}
	if len(c.Metrics) != 10 {
		t.Fatalf("metrics = %d", len(c.Metrics))
	}
	if x := m[MetricOccupancyBP]; x.Current != 5000 || x.Previous != 2500 || x.Delta != 2500 || *x.DeltaBP != 10000 {
		t.Fatalf("occupancy = %+v", x)
	}
	if x := m[MetricRevenueCollected]; x.Current != 9000 || x.Previous != 4000 || x.Unit != UnitCents {
		t.Fatalf("revenue = %+v", x)
	}
	if x := m[MetricAverageValue]; x.Current != 1500 || x.Previous != 1000 {
		t.Fatalf("average = %+v", x)
	}
	if x := m[MetricReleasedUnits]; x.DeltaBP != nil {
		t.Fatalf("delta of a zero previous must be absent: %+v", x)
	}
	var curRev, prevRev money.Cents
	var curB, prevB int
	for _, pt := range c.Series {
		curRev += pt.CurrentRevenue
		prevRev += pt.PreviousRevenue
		curB += pt.CurrentBookings
		prevB += pt.PreviousBookings
	}
	if curRev != 9000 || prevRev != 4000 || curB != 18 || prevB != 8 {
		t.Fatalf("series does not add up: %d %d %d %d", curRev, prevRev, curB, prevB)
	}
	if c.Series[0].Key != "2028-W08" || c.Series[0].CurrentFrom.String() != "2028-02-26" || c.Series[0].PreviousFrom.String() != "2027-02-26" {
		t.Fatalf("first point = %+v", c.Series[0])
	}
	if c.Align != AlignCalendar || c.Granularity != GranularityWeek {
		t.Fatalf("echo = %s %s", c.Align, c.Granularity)
	}
	// Previous span one day longer (current non-leap, previous leap): the
	// extra day lands in the last bucket.
	cur2 := spanOf(per("2029-02-27", "2029-03-01"), func(int) dayFacts { return dayFacts{collected: 1} })
	prev2 := spanOf(per("2028-02-27", "2028-03-01"), func(int) dayFacts { return dayFacts{collected: 1} })
	c2 := buildComparison(cur2, prev2, GranularityDay, AlignCalendar)
	if len(c2.Series) != 3 || c2.Series[2].PreviousRevenue != 2 {
		t.Fatalf("leap series = %+v", c2.Series)
	}
}
