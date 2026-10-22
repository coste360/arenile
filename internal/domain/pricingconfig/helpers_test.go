package pricingconfig

import (
	"errors"
	"fmt"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

func TestWithoutClosedDays_Splits(t *testing.T) {
	d := calendar.MustParse
	gap := pricing.CoverageGap{PackID: "p", PriceCategory: "c", Channel: pricing.ChannelDesk, From: d("2027-06-01"), To: d("2027-06-10")}
	tests := []struct {
		name   string
		closed []string
		want   [][2]string
	}{
		{"none", nil, [][2]string{{"2027-06-01", "2027-06-10"}}},
		{"middle", []string{"2027-06-05"}, [][2]string{{"2027-06-01", "2027-06-04"}, {"2027-06-06", "2027-06-10"}}},
		{"edges", []string{"2027-06-01", "2027-06-10"}, [][2]string{{"2027-06-02", "2027-06-09"}}},
		{"all", []string{"2027-06-01", "2027-06-02", "2027-06-03", "2027-06-04", "2027-06-05", "2027-06-06", "2027-06-07", "2027-06-08", "2027-06-09", "2027-06-10"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			closed := map[calendar.Date]bool{}
			for _, c := range tc.closed {
				closed[d(c)] = true
			}
			got := withoutClosedDays([]pricing.CoverageGap{gap}, closed)
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v", got)
			}
			for i, w := range tc.want {
				if got[i].From != d(w[0]) || got[i].To != d(w[1]) || got[i].PackID != "p" {
					t.Fatalf("gap %d: %+v want %v", i, got[i], w)
				}
			}
		})
	}
}

func TestClosedDays_RequiresEverySlot(t *testing.T) {
	d := calendar.MustParse
	closures := []store.ClosureDay{
		{Date: d("2027-06-01"), Slots: []calendar.Slot{calendar.AM}},
		{Date: d("2027-06-02"), Slots: []calendar.Slot{calendar.AM, calendar.PM}},
	}
	full := closedDays(closures, calendar.SlotsFull)
	if full[d("2027-06-01")] || !full[d("2027-06-02")] {
		t.Fatalf("full: %v", full)
	}
	am := closedDays(closures, calendar.SlotsAM)
	if !am[d("2027-06-01")] || !am[d("2027-06-02")] {
		t.Fatalf("am: %v", am)
	}
}

func TestClipRanges_Season(t *testing.T) {
	d := calendar.MustParse
	from, to := d("2028-05-13"), d("2028-09-13")
	got, clipped := clipRanges([]pricing.DateRange{
		{From: d("2028-05-01"), To: d("2028-05-31")},
		{From: d("2028-07-01"), To: d("2028-07-31")},
		{From: d("2028-09-20"), To: d("2028-09-30")},
	}, from, to)
	if !clipped || len(got) != 2 || got[0].From != from || got[1].From != d("2028-07-01") {
		t.Fatalf("clip: %+v %v", got, clipped)
	}
	got, clipped = clipRanges([]pricing.DateRange{{From: d("2028-06-01"), To: d("2028-06-30")}}, from, to)
	if clipped || len(got) != 1 {
		t.Fatalf("inside: %+v %v", got, clipped)
	}
}

func TestFromEngine_FieldErrors(t *testing.T) {
	var ve *configerr.ValidationError
	err := fromEngine(&pricing.RuleError{RuleID: "r", Field: "action.percent", Reason: "required"}, "")
	if !errors.As(err, &ve) || ve.Fields[0].Field != "action.percent" {
		t.Fatalf("rule error: %v", err)
	}
	err = fromEngine(fmt.Errorf("wrap: %w", &pricing.RuleError{Field: "code", Reason: "required"}), "promo")
	if !errors.As(err, &ve) || ve.Fields[0].Field != "promo.code" {
		t.Fatalf("prefixed: %v", err)
	}
	if err := fromEngine(fmt.Errorf("%w: bad", pricing.ErrInvalidInput), "x"); !errors.Is(err, configerr.ErrValidation) {
		t.Fatalf("invalid input: %v", err)
	}
	other := errors.New("boom")
	if err := fromEngine(other, ""); !errors.Is(err, other) {
		t.Fatalf("passthrough: %v", err)
	}
	if fromEngine(nil, "") != nil {
		t.Fatal("nil")
	}
}

func TestPrefixValidation_Fields(t *testing.T) {
	v := configerr.Invalid("name", "Campo obbligatorio.")
	v.Add("", "generic")
	var ve *configerr.ValidationError
	if err := prefixValidation(v, "rules[2]"); !errors.As(err, &ve) || ve.Fields[0].Field != "rules[2].name" || ve.Fields[1].Field != "rules[2]" {
		t.Fatalf("prefixed: %v", err)
	}
	other := errors.New("x")
	if prefixValidation(other, "p") != other { //nolint:errorlint // identity check
		t.Fatal("non-validation errors pass through")
	}
}

func TestCleanIDs_NilForEmpty(t *testing.T) {
	if got := cleanIDs([]string{"", " "}); got != nil {
		t.Fatalf("got %v", got)
	}
	if got := cleanIDs([]string{"a", " a ", "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
}

func TestPromoAndAgreement_Helpers(t *testing.T) {
	d := calendar.MustParse
	maxUses := 3
	pc := PromoCode{MaxUses: &maxUses, Uses: 3}
	if !pc.Exhausted() {
		t.Fatal("exhausted")
	}
	pc.Uses = 2
	if pc.Exhausted() {
		t.Fatal("not exhausted")
	}
	from, to := d("2027-06-01"), d("2027-06-30")
	a := Agreement{Active: true, ValidFrom: &from, ValidTo: &to}
	for date, want := range map[string]bool{"2027-05-31": false, "2027-06-01": true, "2027-06-30": true, "2027-07-01": false} {
		if got := a.ValidOn(d(date)); got != want {
			t.Fatalf("%s: %v", date, got)
		}
	}
	a.Active = false
	if a.ValidOn(d("2027-06-10")) {
		t.Fatal("inactive")
	}
	if !codeRe.MatchString("ESTATE_10") || codeRe.MatchString("a") || codeRe.MatchString("con spazio") {
		t.Fatal("code format")
	}
}
