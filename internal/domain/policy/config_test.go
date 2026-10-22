package policy

import (
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
)

func TestCheckConfig_Table(t *testing.T) {
	tests := []struct {
		name    string
		rules   Rules
		wantErr bool
	}{
		{"empty ok", Rules{}, false},
		{"default ok", DefaultRules(), false},
		{"negative lead days", Rules{MinLeadTime: &MinLeadTime{Days: Ptr(-1)}}, true},
		{"bad cutoff", Rules{MinLeadTime: &MinLeadTime{CutoffTime: Ptr("8:00")}}, true},
		{"cutoff 24:00 invalid", Rules{MinLeadTime: &MinLeadTime{CutoffTime: Ptr("24:00")}}, true},
		{"cutoff 23:59 ok", Rules{MinLeadTime: &MinLeadTime{CutoffTime: Ptr("23:59")}}, false},
		{"negative max lead", Rules{MaxLeadTimeDays: Ptr(-1)}, true},
		{"bad until", Rules{SameDayBooking: &SameDayBooking{Until: Ptr("ab:cd")}}, true},
		{"bad only slot", Rules{SameDayBooking: &SameDayBooking{OnlySlot: Ptr("EVENING")}}, true},
		{"PM_after without until", Rules{SameDayBooking: &SameDayBooking{OnlySlot: Ptr(OnlySlotPMAfter)}}, true},
		{"unknown formula", Rules{Duration: &Duration{AllowedFormulas: []Formula{"year"}}}, true},
		{"max < min", Rules{Duration: &Duration{MinDays: Ptr(5), MaxDays: Ptr(2)}}, true},
		{"weekday 8", Rules{Duration: &Duration{FixedStartWeekday: Ptr(8)}}, true},
		{"weekday 0 ok", Rules{Duration: &Duration{FixedStartWeekday: Ptr(0)}}, false},
		{"bad payment mode", Rules{Payment: &Payment{Mode: Ptr(PaymentMode("card"))}}, true},
		{"deposit mode without deposit", Rules{Payment: &Payment{Mode: Ptr(PaymentDeposit)}}, true},
		{"deposit percent > 100", depositRules(PaymentDeposit, DepositPercent, 101), true},
		{"deposit amount > 100 ok", depositRules(PaymentDeposit, DepositAmount, 5000), false},
		{"deposit negative", depositRules(PaymentDeposit, DepositAmount, -1), true},
		{"deposit bad type", depositRules(PaymentDeposit, DepositType("x"), 1), true},
		{"bad balance due", Rules{Payment: &Payment{BalanceDue: Ptr("later")}}, true},
		{"tier negative days", Rules{Cancellation: []CancellationTier{{UntilDaysBefore: -1}}}, true},
		{"tier percent > 100", Rules{Cancellation: []CancellationTier{{RefundPercent: 120}}}, true},
		{"tier duplicate days", Rules{Cancellation: []CancellationTier{{UntilDaysBefore: 3}, {UntilDaysBefore: 3}}}, true},
		{"tier bad as", Rules{Cancellation: []CancellationTier{{As: "voucher"}}}, true},
		{"negative modification fee", Rules{Modification: &Modification{Fee: Ptr(int64(-1))}}, true},
		{"bad weather as", Rules{WeatherPolicy: &WeatherPolicy{As: Ptr(Compensation("cash"))}}, true},
		{"zero max spots", Rules{MaxSpotsPerBooking: Ptr(0)}, true},
		{"season credit percent > 100", Rules{SeasonReleaseCredit: &SeasonReleaseCredit{Percent: Ptr(101)}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rules.CheckConfig()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidRules) {
				t.Fatalf("error does not wrap ErrInvalidRules: %v", err)
			}
		})
	}
}

func TestCheckConfig_ReportsAllProblems(t *testing.T) {
	r := Rules{MaxLeadTimeDays: Ptr(-1), MaxSpotsPerBooking: Ptr(0)}
	err := r.CheckConfig()
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) || len(joined.Unwrap()) != 2 {
		t.Fatalf("want 2 joined errors, got %v", err)
	}
}

func TestSnapshot_RoundTrip(t *testing.T) {
	res := Resolve([]Policy{{ID: "p1", Priority: 7, Scope: Scope{Channels: []Channel{ChannelOnline}}, Rules: Rules{MaxSpotsPerBooking: Ptr(2)}}},
		DefaultRules(), Target{Channel: ChannelOnline})
	ts := time.Date(2027, 7, 1, 10, 0, 0, 0, time.FixedZone("CEST", 7200))
	s := NewSnapshot(res, ChannelOnline, ts)
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.PolicyID != "p1" || back.Priority != 7 || back.Version != SnapshotVersion || *back.Rules.MaxSpotsPerBooking != 2 {
		t.Fatalf("back = %+v", back)
	}
	if !back.ResolvedAt.Equal(ts) || back.ResolvedAt.Location() != time.UTC {
		t.Fatalf("resolved_at = %v", back.ResolvedAt)
	}
	// The snapshot is self-contained: refunds computed from it match the original rules.
	start := calendar.MustParse("2027-08-15")
	for _, day := range []int{1, 2, 12, 13, 16} {
		now := time.Date(2027, 8, day, 10, 0, 0, 0, time.UTC)
		a := CancellationOutcome(start, 10000, now, time.UTC, res.Rules)
		c := CancellationOutcome(start, 10000, now, time.UTC, back.Rules)
		if a != c {
			t.Fatalf("outcome differs on day %d: %+v vs %+v", day, a, c)
		}
	}
}

func TestParseSnapshot_Errors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"not json", "{"},
		{"version 0", `{"version":0,"rules":{}}`},
		{"future version", `{"version":99,"rules":{}}`},
		{"wrong type", `{"version":1,"rules":{"max_spots_per_booking":"x"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseSnapshot([]byte(tc.in)); !errors.Is(err, ErrInvalidSnapshot) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
