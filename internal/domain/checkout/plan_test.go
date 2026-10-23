package checkout

import (
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

var d = calendar.MustParse

func testCatalog() *onlineCatalog {
	return &onlineCatalog{
		zones: map[string]store.Zone{
			"seat":   {ID: "seat", AssignmentMode: "seat", OnlineVisible: true},
			"zone":   {ID: "zone", AssignmentMode: "zone", OnlineVisible: true},
			"qty":    {ID: "qty", AssignmentMode: "quantity", OnlineVisible: true},
			"hidden": {ID: "hidden", AssignmentMode: "zone", OnlineVisible: false},
		},
		spots: map[string]store.Spot{
			"s1":  {ID: "s1", ZoneID: "seat", Active: true, Kind: "umbrella"},
			"off": {ID: "off", ZoneID: "seat", Active: false},
			"z1":  {ID: "z1", ZoneID: "zone", Active: true},
		},
		packs: map[string]store.Pack{
			"base":   {ID: "base", Active: true, Channels: []string{"desk", "online"}, AllowedFormulas: []string{"day", "multi_day"}, MaxPerBooking: 2},
			"desk":   {ID: "desk", Active: true, Channels: []string{"desk"}},
			"gazebo": {ID: "gazebo", Active: true, Channels: []string{"online"}, RequiresSpotKind: []string{"gazebo"}},
			"onlyz":  {ID: "onlyz", Active: true, Channels: []string{"online"}, AllowedZoneIDs: []string{"zone"}},
		},
		extras: map[string]store.Extra{
			"sunbed": {ID: "sunbed", Active: true, Channels: []string{"online"}, MaxQuantity: 3},
			"desk":   {ID: "desk", Active: true, Channels: []string{"desk"}},
		},
	}
}

func TestValidateSelection_Rules(t *testing.T) {
	today := d("2027-07-01")
	base := func() Selection {
		return Selection{
			StartsOn: d("2027-07-10"), EndsOn: d("2027-07-10"), Slots: calendar.SlotsFull, Formula: "day",
			Lines: []booking.LineInput{{PackID: "base", SpotID: "s1"}},
		}
	}
	tests := []struct {
		name  string
		edit  func(*Selection, *online.Config)
		field string // "" = valid
	}{
		{"valid seat", func(*Selection, *online.Config) {}, ""},
		{"valid zone", func(s *Selection, _ *online.Config) { s.Lines = []booking.LineInput{{PackID: "base", ZoneID: "zone"}} }, ""},
		{"valid quantity", func(s *Selection, _ *online.Config) {
			s.Lines = []booking.LineInput{{PackID: "base", ZoneID: "qty", Quantity: 2}}
		}, ""},
		{"past date", func(s *Selection, _ *online.Config) { s.StartsOn, s.EndsOn = d("2027-06-30"), d("2027-06-30") }, "starts_on"},
		{"ends before", func(s *Selection, _ *online.Config) { s.EndsOn = d("2027-07-09") }, "ends_on"},
		{"too far ahead", func(s *Selection, c *online.Config) { c.MaxDaysAhead = 5 }, "starts_on"},
		{"slot not offered", func(s *Selection, c *online.Config) { c.Slots = []string{"FULL"}; s.Slots = calendar.SlotsAM }, "slots"},
		{"formula not offered", func(s *Selection, c *online.Config) { c.Formulas = []string{"week"} }, "formula"},
		{"no lines", func(s *Selection, _ *online.Config) { s.Lines = nil }, "lines"},
		{"desk pack", func(s *Selection, _ *online.Config) { s.Lines[0].PackID = "desk" }, "lines[0].pack_id"},
		{"pack formula", func(s *Selection, _ *online.Config) { s.Formula = "week" }, "lines[0].pack_id"},
		{"inactive spot", func(s *Selection, _ *online.Config) { s.Lines[0].SpotID = "off" }, "lines[0].spot_id"},
		{"spot kind", func(s *Selection, _ *online.Config) { s.Lines[0].PackID = "gazebo" }, "lines[0].spot_id"},
		{"hidden zone", func(s *Selection, _ *online.Config) {
			s.Lines = []booking.LineInput{{PackID: "base", ZoneID: "hidden"}}
		}, "lines[0].zone_id"},
		{"pack zone", func(s *Selection, _ *online.Config) { s.Lines[0].PackID = "onlyz" }, "lines[0].pack_id"},
		{"seat without spot", func(s *Selection, _ *online.Config) { s.Lines = []booking.LineInput{{PackID: "base", ZoneID: "seat"}} }, "lines[0].spot_id"},
		{"zone with spot", func(s *Selection, _ *online.Config) { s.Lines = []booking.LineInput{{PackID: "base", SpotID: "z1"}} }, "lines[0].spot_id"},
		{"quantity on seat", func(s *Selection, _ *online.Config) { s.Lines[0].Quantity = 2 }, "lines[0].quantity"},
		{"max per booking", func(s *Selection, _ *online.Config) {
			s.Lines = []booking.LineInput{{PackID: "base", ZoneID: "qty", Quantity: 3}}
		}, "lines"},
		{"desk extra", func(s *Selection, _ *online.Config) { s.Extras = []booking.ExtraInput{{ExtraID: "desk", Quantity: 1}} }, "extras[0].extra_id"},
		{"extra quantity", func(s *Selection, _ *online.Config) {
			s.Lines[0].Extras = []booking.ExtraInput{{ExtraID: "sunbed", Quantity: 4}}
		}, "lines[0].extras[0].quantity"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sel, flow := base(), online.DefaultConfig()
			tc.edit(&sel, &flow)
			err := validateSelection(sel, testCatalog(), flow, today)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			var ve *configerr.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want validation error on %s, got %v", tc.field, err)
			}
			for _, f := range ve.Fields {
				if f.Field == tc.field {
					return
				}
			}
			t.Fatalf("fields %+v, want %s", ve.Fields, tc.field)
		})
	}
}

func TestPaymentPlan_Modes(t *testing.T) {
	mode := func(m policy.PaymentMode) policy.Rules {
		r := policy.Rules{Payment: &policy.Payment{Mode: &m}}
		return r
	}
	tests := []struct {
		name               string
		in                 planInput
		plan               string
		dueNow, balanceDue money.Cents
	}{
		{"full online", planInput{Rules: mode(policy.PaymentFull), ProviderOnline: true, Total: 5000, Deposit: 5000}, PlanOnlineFull, 5000, 0},
		{"default is full", planInput{ProviderOnline: true, Total: 5000, Deposit: 5000}, PlanOnlineFull, 5000, 0},
		{"deposit", planInput{Rules: mode(policy.PaymentDeposit), ProviderOnline: true, Total: 5000, Deposit: 1500}, PlanOnlineDeposit, 1500, 3500},
		{"deposit equal total", planInput{Rules: mode(policy.PaymentDeposit), ProviderOnline: true, Total: 5000, Deposit: 5000}, PlanOnlineFull, 5000, 0},
		{"mode none", planInput{Rules: mode(policy.PaymentNone), ProviderOnline: true, Total: 5000}, PlanAtDesk, 0, 5000},
		{"manual provider", planInput{Rules: mode(policy.PaymentFull), ProviderManual: true, Total: 5000, Deposit: 5000}, PlanAtDesk, 0, 5000},
		{"manual provider, deposit", planInput{Rules: mode(policy.PaymentDeposit), ProviderManual: true, Total: 5000, Deposit: 1500}, PlanAtDesk, 0, 5000},
		{"manual provider, mode none", planInput{Rules: mode(policy.PaymentNone), ProviderManual: true, Total: 5000}, PlanAtDesk, 0, 5000},
		// Stripe active but not usable (credentials cleared): never at the desk (INV-19).
		{"online provider unusable", planInput{Rules: mode(policy.PaymentFull), Total: 5000, Deposit: 5000}, PlanOnlineFull, 5000, 0},
		{"online provider unusable, deposit", planInput{Rules: mode(policy.PaymentDeposit), Total: 5000, Deposit: 1500}, PlanOnlineDeposit, 1500, 3500},
		{"online provider unusable, mode none", planInput{Rules: mode(policy.PaymentNone), Total: 5000}, PlanAtDesk, 0, 5000},
		{"online provider unusable, free", planInput{Total: 0}, PlanAtDesk, 0, 0},
		{"approval first", planInput{Rules: mode(policy.PaymentFull), ApprovalRequired: true, ProviderOnline: true, Total: 5000}, PlanApproval, 0, 5000},
		{"free", planInput{ProviderOnline: true}, PlanAtDesk, 0, 0},
		{"partly paid", planInput{ProviderOnline: true, Total: 5000, Deposit: 5000, Paid: 2000}, PlanOnlineFull, 3000, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, due, later := paymentPlan(tc.in)
			if plan != tc.plan || due != tc.dueNow || later != tc.balanceDue {
				t.Fatalf("got %s %d %d, want %s %d %d", plan, due, later, tc.plan, tc.dueNow, tc.balanceDue)
			}
			if due+later != max(tc.in.Total-tc.in.Paid, 0) {
				t.Fatalf("due now + later must equal the remaining total")
			}
		})
	}
}

func TestDeriveState_NextAction(t *testing.T) {
	now := time.Date(2027, 7, 1, 10, 0, 0, 0, time.UTC)
	future, past := now.Add(5*time.Minute), now.Add(-time.Second)
	b := func(st booking.Status, hold *time.Time) *store.Booking {
		return &store.Booking{Status: string(st), HoldExpiresAt: hold}
	}
	tests := []struct {
		name    string
		stored  string
		b       *store.Booking
		details bool
		pay     *PaymentState
		want    flowState
	}{
		{"held without details", store.CheckoutOpen, b(booking.StatusHeld, &future), false, nil, flowState{store.CheckoutOpen, NextDetails}},
		{"held ready", store.CheckoutOpen, b(booking.StatusHeld, &future), true, nil, flowState{store.CheckoutOpen, NextPay}},
		{"held elapsed before sweep", store.CheckoutOpen, b(booking.StatusHeld, &past), true, nil, flowState{store.CheckoutExpired, NextExpired}},
		{"paying", store.CheckoutPaying, b(booking.StatusPendingPayment, &future), true, &PaymentState{Status: "pending"}, flowState{store.CheckoutPaying, NextWait}},
		{"payment failed", store.CheckoutPaying, b(booking.StatusPendingPayment, &future), true, &PaymentState{Status: "failed"}, flowState{store.CheckoutPaying, NextPay}},
		{"approval", store.CheckoutCompleted, b(booking.StatusAwaitingApproval, &future), true, nil, flowState{store.CheckoutCompleted, NextWait}},
		{"confirmed", store.CheckoutPaying, b(booking.StatusConfirmed, nil), true, nil, flowState{store.CheckoutCompleted, NextDone}},
		{"rejected", store.CheckoutCompleted, b(booking.StatusRejected, nil), true, nil, flowState{store.CheckoutExpired, NextRejected}},
		{"expired", store.CheckoutOpen, b(booking.StatusExpired, nil), true, nil, flowState{store.CheckoutExpired, NextExpired}},
		{"abandoned", store.CheckoutAbandoned, b(booking.StatusExpired, nil), true, nil, flowState{store.CheckoutAbandoned, NextExpired}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveState(tc.stored, tc.b, tc.details, tc.pay, now); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDetailsComplete_Terms(t *testing.T) {
	v1, v2 := "v1", "v2"
	cur := &store.TermsVersion{Version: "v2"}
	tests := []struct {
		name        string
		accepted    *string
		placeholder bool
		current     *store.TermsVersion
		want        bool
	}{
		{"placeholder", &v2, true, cur, false},
		{"no terms published", nil, false, nil, true},
		{"current accepted", &v2, false, cur, true},
		{"old version", &v1, false, cur, false},
		{"none accepted", nil, false, cur, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detailsComplete(&store.Booking{TermsVersion: tc.accepted}, tc.placeholder, tc.current); got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestIndicatorAndDayStatus_Thresholds(t *testing.T) {
	for _, tc := range []struct {
		avail, capacity int
		ind, day        string
	}{
		{0, 50, IndicatorNone, DayFull},
		{2, 50, IndicatorLast, DayLimited},
		{3, 50, IndicatorLast, DayLimited},
		{5, 50, IndicatorFew, DayLimited},
		{10, 50, IndicatorFew, DayAvailable},
		{11, 50, IndicatorPlenty, DayAvailable},
		{40, 200, IndicatorFew, DayAvailable},
	} {
		if got := indicator(tc.avail, tc.capacity); got != tc.ind {
			t.Errorf("indicator(%d,%d) = %s, want %s", tc.avail, tc.capacity, got, tc.ind)
		}
		if got := dayStatus(tc.avail, tc.capacity); got != tc.day {
			t.Errorf("dayStatus(%d,%d) = %s, want %s", tc.avail, tc.capacity, got, tc.day)
		}
	}
}

func TestDefaultFormula_Derived(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		slots    calendar.Slots
		want     string
	}{
		{"2027-07-10", "2027-07-10", calendar.SlotsFull, "day"},
		{"2027-07-10", "2027-07-10", calendar.SlotsAM, "half_day"},
		{"2027-07-10", "2027-07-12", calendar.SlotsFull, "multi_day"},
	} {
		if got := DefaultFormula(d(tc.from), d(tc.to), tc.slots); got != tc.want {
			t.Errorf("%s..%s %s = %s", tc.from, tc.to, tc.slots, got)
		}
	}
}

func TestLegendTones_Ordered(t *testing.T) {
	got := legendTones(map[string]money.Cents{"front": 3000, "back": 2000, "vip": 5000})
	if len(got) != 3 || got[0].PriceCategory != "back" || got[0].Tone != 0 || got[2].PriceCategory != "vip" || got[2].Tone != 2 {
		t.Fatalf("legend %+v", got)
	}
}

func TestManageActions_Policy(t *testing.T) {
	two, today := 2, d("2027-07-01")
	rules := policy.Rules{
		Modification:                 &policy.Modification{AllowedUntilDaysBefore: &two},
		Cancellation:                 []policy.CancellationTier{{UntilDaysBefore: 14, RefundPercent: 100}, {UntilDaysBefore: 3, RefundPercent: 50, As: policy.AsCredit}},
		CustomerCanReleaseSeasonDays: policy.Ptr(true),
	}
	b := &store.Booking{Status: "confirmed", StartsOn: d("2027-07-10"), TotalAmount: 5000, PaidAmount: 1000}
	a := manageActions(b, rules, today, true)
	if !a.PayBalance || !a.Modify || !a.Cancel || a.ReleaseDays || a.ModifyUntil.String() != "2027-07-08" || a.CancelUntilFree.String() != "2027-06-26" {
		t.Fatalf("actions %+v", a)
	}
	late := manageActions(b, rules, d("2027-07-09"), false)
	if late.Modify || late.PayBalance || !late.Cancel {
		t.Fatalf("late actions %+v", late)
	}
	started := manageActions(b, rules, d("2027-07-11"), true)
	if started.Cancel {
		t.Fatalf("cancel after the start %+v", started)
	}
	pass := &store.Booking{Status: "confirmed", StartsOn: d("2027-05-15"), IsSeasonPass: true}
	if pa := manageActions(pass, rules, today, true); !pa.ReleaseDays || pa.Modify {
		t.Fatalf("pass actions %+v", pa)
	}
}

func TestQuoteItems_SumToTotal(t *testing.T) {
	bd := pricing.PriceBreakdown{
		Lines:     []pricing.LineBreakdown{{Total: 2000}, {Total: 2000}, {Total: 5000}},
		Extras:    []pricing.ExtraBreakdown{{Code: "parking", Qty: 1, Total: 500}},
		Discounts: []pricing.DiscountBreakdown{{Source: "promo:ESTATE", Amount: -900}},
		Rounding:  -1,
		Total:     8599,
	}
	lines := []booking.LineInput{{PackID: "base", Quantity: 2}, {PackID: "vip"}}
	_, owner := booking.PricingLines([]booking.PricedLine{{PackID: "base", Quantity: 2}, {PackID: "vip", Quantity: 1}})
	items := quoteItems(bd, owner, lines, []booking.ExtraInput{{ExtraID: "p", Quantity: 1}}, map[string]store.Pack{}, map[string]store.Extra{})
	var sum money.Cents
	for _, it := range items {
		sum += it.Amount
	}
	if sum != bd.Total || len(items) != 5 || items[0].Amount != 4000 || items[0].Quantity != 2 {
		t.Fatalf("items %+v sum %d", items, sum)
	}
}
