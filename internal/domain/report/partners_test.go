package report

import (
	"encoding/json"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func m8day(s string) calendar.Date { return calendar.MustParse(s) }

func priceWithAgreementDiscount(t testing.TB, d money.Cents) store.JSON {
	b, err := json.Marshal(pricing.PriceBreakdown{Discounts: []pricing.DiscountBreakdown{
		{Source: "agreement:RES", Amount: -d}, {Source: "promo:X", Amount: -100},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return store.JSON(b)
}

// TestPartnerStatement_BalanceIsChargedMinusPaid: INV-104, for any
// bookings and payments the closing balance is opening + charged - paid,
// charged sums partner_amount and paid sums the non voided payments of the
// period.
func TestPartnerStatement_BalanceIsChargedMinusPaid(t *testing.T) {
	p := Period{From: m8day("2027-07-01"), To: m8day("2027-07-31")}
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(0, 20).Draw(rt, "bookings")
		var rows []store.M8ReportBooking
		var charged money.Cents
		for i := range n {
			amt := money.Cents(rapid.Int64Range(0, 500_000).Draw(rt, "partner_amount"))
			start := p.From.AddDays(rapid.IntRange(-5, 30).Draw(rt, "start"))
			rows = append(rows, store.M8ReportBooking{
				ID: string(rune('a' + i)), Code: "C", StartsOn: start, EndsOn: start.AddDays(rapid.IntRange(0, 10).Draw(rt, "len")),
				TotalAmount: amt + money.Cents(rapid.Int64Range(0, 10_000).Draw(rt, "rest")), PartnerAmount: amt, Units: 1,
			})
			charged += amt
		}
		m := rapid.IntRange(0, 10).Draw(rt, "payments")
		var pays []store.PartnerPayment
		var paid money.Cents
		for range m {
			amt := money.Cents(rapid.Int64Range(1, 300_000).Draw(rt, "amount"))
			on := p.From.AddDays(rapid.IntRange(-10, 40).Draw(rt, "received"))
			pay := store.PartnerPayment{Amount: amt, ReceivedOn: on}
			if rapid.Bool().Draw(rt, "voided") {
				now := time.Now()
				pay.VoidedAt = &now
			} else if p.Contains(on) {
				paid += amt
			}
			pays = append(pays, pay)
		}
		before := store.PartnerBalance{
			Charged: money.Cents(rapid.Int64Range(0, 1_000_000).Draw(rt, "charged_before")),
			Paid:    money.Cents(rapid.Int64Range(0, 1_000_000).Draw(rt, "paid_before")),
		}
		st := BuildStatement(p, rows, nil, before, pays)
		if st.Charged != charged || st.Paid != paid {
			rt.Fatalf("charged %d paid %d, want %d %d", st.Charged, st.Paid, charged, paid)
		}
		if st.OpeningBalance != before.Charged-before.Paid {
			rt.Fatalf("opening %d", st.OpeningBalance)
		}
		if st.ClosingBalance != st.OpeningBalance+st.Charged-st.Paid {
			rt.Fatalf("closing %d != %d + %d - %d", st.ClosingBalance, st.OpeningBalance, st.Charged, st.Paid)
		}
		for _, pay := range st.Payments {
			if !p.Contains(pay.ReceivedOn) {
				rt.Fatalf("payment outside the period listed")
			}
		}
	})
}

func TestBuildStatement_Counters(t *testing.T) {
	p := Period{From: m8day("2027-07-01"), To: m8day("2027-07-10")}
	two, one := 2, 1
	ag := "ag1"
	rows := []store.M8ReportBooking{{
		ID: "b1", Code: "AR-1", LastName: "Rossi", FirstName: "Anna", AgreementID: &ag, StartsOn: m8day("2027-06-28"),
		EndsOn: m8day("2027-07-03"), TotalAmount: 10000, PartnerAmount: 10000, PartyAdults: &two, PartyChildren: &one,
		Units: 2, PriceSnapshot: priceWithAgreementDiscount(t, 1500),
	}}
	st := BuildStatement(p, rows, map[string]string{"ag1": "Hotel"}, store.PartnerBalance{}, nil)
	b := st.Bookings[0]
	if b.SpotDays != 6 || b.People != 3 || b.Discount != 1500 || b.AgreementName != "Hotel" || b.CustomerDisplayName != "Rossi Anna" {
		t.Fatalf("booking %+v", b)
	}
}

func TestBuildPartnersReport_Rows(t *testing.T) {
	p := Period{From: m8day("2027-07-01"), To: m8day("2027-07-31")}
	pa, pb := "p1", "p2"
	rows := []store.M8ReportBooking{
		{ID: "b1", PartnerID: &pa, StartsOn: m8day("2027-07-01"), EndsOn: m8day("2027-07-02"), TotalAmount: 5000, PartnerAmount: 2500, Units: 1},
		{ID: "b2", PartnerID: &pa, StartsOn: m8day("2027-07-05"), EndsOn: m8day("2027-07-05"), TotalAmount: 3000, PartnerAmount: 3000, Units: 1},
	}
	pays := []store.PartnerPayment{{PartnerID: pb, Amount: 700, ReceivedOn: m8day("2027-07-10")}}
	partners := []store.Partner{{ID: pa, Name: "Miramare", Kind: store.PartnerKindAccommodation}, {ID: pb, Name: "Aurora", Kind: store.PartnerKindAccommodation}, {ID: "p3", Name: "Zeta"}}
	r := BuildPartnersReport(p, partners, rows, pays, "")
	if len(r.Rows) != 2 || r.Rows[0].PartnerName != "Aurora" || r.Rows[1].Bookings != 2 || r.Rows[1].SpotDays != 3 {
		t.Fatalf("rows %+v", r.Rows)
	}
	if r.Totals.Revenue != 8000 || r.Totals.Charged != 5500 || r.Totals.Paid != 700 {
		t.Fatalf("totals %+v", r.Totals)
	}
}

func TestBuildAgreementsRegister_TotalsByCategory(t *testing.T) {
	p := Period{From: m8day("2027-06-01"), To: m8day("2027-09-30")}
	res, hotel, old := "ag-res", "ag-hotel", "ag-old"
	cat := "cat-res"
	partner := "pa-1"
	target := store.AgreementTargetCategory
	ptarget := store.AgreementTargetPartner
	vt := m8day("2026-09-30")
	act := store.JSON(`{"type":"percent_off","percent":"10"}`)
	in := RegisterInput{
		Period: p,
		Agreements: []store.Agreement{
			{ID: res, Name: "Residenti", Active: true, Target: &target, CustomerCategoryID: &cat, Action: act, Requirement: "documento di residenza"},
			{
				ID: hotel, Name: "Hotel Miramare", Active: true, Target: &ptarget, PartnerID: &partner, Action: store.JSON(`{"type":"percent_off","percent":"0"}`),
				PackPrices: store.JSON(`[{"pack_id":"pk","amount":1200}]`),
			},
			{ID: old, Name: "Scaduta", Active: true, Action: act, ValidTo: &vt},
		},
		Partners:   map[string]store.Partner{partner: {ID: partner, Name: "Hotel Miramare", Kind: store.PartnerKindAccommodation}},
		Categories: map[string]store.CustomerCategory{cat: {ID: cat, Name: "Residenti", RegisterCategory: store.RegisterResidents}},
		PackNames:  map[string]string{"pk": "Giornaliero"},
		Bookings: []store.M8ReportBooking{
			{ID: "b1", AgreementID: &res, StartsOn: m8day("2027-07-01"), EndsOn: m8day("2027-07-01"), TotalAmount: 2700, Units: 1, PriceSnapshot: priceWithAgreementDiscount(t, 300)},
			{ID: "b2", AgreementID: &res, StartsOn: m8day("2027-07-02"), EndsOn: m8day("2027-07-03"), TotalAmount: 5400, Units: 1, PriceSnapshot: priceWithAgreementDiscount(t, 600)},
			{ID: "b3", AgreementID: &hotel, StartsOn: m8day("2027-08-01"), EndsOn: m8day("2027-08-01"), TotalAmount: 1200, Units: 2, PriceSnapshot: priceWithAgreementDiscount(t, 1800)},
		},
	}
	reg := BuildAgreementsRegister(in)
	if len(reg.Entries) != 2 {
		t.Fatalf("entries %+v", reg.Entries)
	}
	if reg.Entries[0].RegisterCategory != store.RegisterResidents || reg.Entries[1].RegisterCategory != store.RegisterAccommodation {
		t.Fatalf("categories %s %s", reg.Entries[0].RegisterCategory, reg.Entries[1].RegisterCategory)
	}
	if reg.Entries[0].Bookings != 2 || reg.Entries[0].SpotDays != 3 || reg.Entries[0].Discount != 900 {
		t.Fatalf("residents %+v", reg.Entries[0])
	}
	if reg.Entries[1].SpotDays != 2 || reg.Entries[1].ConditionsText != "Prezzo fisso Giornaliero: 12,00 € al giorno" {
		t.Fatalf("hotel %+v", reg.Entries[1])
	}
	if reg.Entries[0].ConditionsText != "Sconto del 10%. Requisito: documento di residenza" {
		t.Fatalf("conditions %q", reg.Entries[0].ConditionsText)
	}
	var sum RegisterCounters
	for _, c := range reg.ByCategory {
		sum.add(c.RegisterCounters)
	}
	if sum != reg.Totals || reg.Totals.Agreements != 2 || reg.Totals.Bookings != 3 || reg.Totals.Revenue != 9300 || reg.Totals.Discount != 2700 {
		t.Fatalf("totals %+v by category %+v", reg.Totals, reg.ByCategory)
	}
}
