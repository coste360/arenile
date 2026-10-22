package policy

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

func depositRules(mode PaymentMode, typ DepositType, value int64) Rules {
	r := Rules{Payment: &Payment{Mode: Ptr(mode)}}
	if typ != "" {
		r.Payment.Deposit = &DepositRule{Type: Ptr(typ), Value: Ptr(value)}
	}
	return r
}

func TestDeposit_Table(t *testing.T) {
	tests := []struct {
		name  string
		total money.Cents
		rules Rules
		want  money.Cents
	}{
		{"unset mode is full", 10000, Rules{}, 10000},
		{"full", 12345, depositRules(PaymentFull, "", 0), 12345},
		{"none", 12345, depositRules(PaymentNone, "", 0), 0},
		{"30 percent", 10000, depositRules(PaymentDeposit, DepositPercent, 30), 3000},
		{"30 percent tie rounds to even", 1005, depositRules(PaymentDeposit, DepositPercent, 30), 302}, // 301.5 -> 302 (even)
		{"50 percent tie to even", 101, depositRules(PaymentDeposit, DepositPercent, 50), 50},          // 50.5 -> 50
		{"50 percent tie to even up", 103, depositRules(PaymentDeposit, DepositPercent, 50), 52},       // 51.5 -> 52
		{"33 percent", 999, depositRules(PaymentDeposit, DepositPercent, 33), 330},                     // 329.67
		{"0 percent", 10000, depositRules(PaymentDeposit, DepositPercent, 0), 0},
		{"100 percent", 10000, depositRules(PaymentDeposit, DepositPercent, 100), 10000},
		{"over 100 percent capped", 10000, depositRules(PaymentDeposit, DepositPercent, 150), 10000},
		{"fixed amount", 10000, depositRules(PaymentDeposit, DepositAmount, 2500), 2500},
		{"fixed amount capped at total", 2000, depositRules(PaymentDeposit, DepositAmount, 2500), 2000},
		{"negative amount floored", 2000, depositRules(PaymentDeposit, DepositAmount, -5), 0},
		{"deposit without config falls back to full", 2000, Rules{Payment: &Payment{Mode: Ptr(PaymentDeposit)}}, 2000},
		{"unknown deposit type falls back to full", 2000, depositRules(PaymentDeposit, DepositType("weird"), 10), 2000},
		{"zero total", 0, depositRules(PaymentDeposit, DepositPercent, 30), 0},
		{"negative total", -100, Rules{}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Deposit(tc.total, tc.rules); got != tc.want {
				t.Fatalf("Deposit = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBalanceDueDate_Table(t *testing.T) {
	start := calendar.MustParse("2027-08-15")
	withDue := func(s string) Rules {
		r := depositRules(PaymentDeposit, DepositPercent, 30)
		r.Payment.BalanceDue = Ptr(s)
		return r
	}
	tests := []struct {
		name   string
		rules  Rules
		want   string
		wantOK bool
	}{
		{"full has no balance", depositRules(PaymentFull, "", 0), "", false},
		{"none has no balance", depositRules(PaymentNone, "", 0), "", false},
		{"unset mode has no balance", Rules{}, "", false},
		{"deposit default on arrival", depositRules(PaymentDeposit, DepositPercent, 30), "2027-08-15", true},
		{"on arrival", withDue("on_arrival"), "2027-08-15", true},
		{"7 days before", withDue("days_before:7"), "2027-08-08", true},
		{"0 days before", withDue("days_before:0"), "2027-08-15", true},
		{"20 days before crosses month", withDue("days_before:20"), "2027-07-26", true},
		{"malformed falls back to arrival", withDue("tomorrow"), "2027-08-15", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := BalanceDueDate(start, tc.rules)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got.String() != tc.want {
				t.Fatalf("date = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestParseBalanceDue_Table(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
		want    BalanceDue
	}{
		{"on_arrival", false, BalanceDue{OnArrival: true}},
		{"days_before:7", false, BalanceDue{DaysBefore: 7}},
		{"days_before:0", false, BalanceDue{DaysBefore: 0}},
		{"days_before:-1", true, BalanceDue{}},
		{"days_before:", true, BalanceDue{}},
		{"days_before:x", true, BalanceDue{}},
		{"", true, BalanceDue{}},
		{"on arrival", true, BalanceDue{}},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseBalanceDue(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if err == nil {
				if got != tc.want {
					t.Fatalf("got %+v", got)
				}
				if got.String() != tc.in {
					t.Fatalf("String = %q", got.String())
				}
			}
		})
	}
}

func TestCancellationOutcome_Table(t *testing.T) {
	loc := rome(t)
	def := DefaultRules() // 14d: 100% refund, 3d: 50% credit, 0d: 0%
	start := calendar.MustParse("2027-08-15")
	unsorted := Rules{Cancellation: []CancellationTier{
		{UntilDaysBefore: 0, RefundPercent: 10},
		{UntilDaysBefore: 30, RefundPercent: 100},
		{UntilDaysBefore: 7, RefundPercent: 50, As: AsCredit},
	}}
	tests := []struct {
		name       string
		rules      Rules
		paid       money.Cents
		now        time.Time
		refund     money.Cents
		credit     money.Cents
		tier       int
		daysBefore int
	}{
		{"60 days before full refund", def, 50000, at("2027-06-16", 10, 0, loc), 50000, 0, 0, 60},
		{"exactly 14 days before", def, 50000, at("2027-08-01", 23, 59, loc), 50000, 0, 0, 14},
		{"13 days before half credit", def, 50000, at("2027-08-02", 0, 1, loc), 0, 25000, 1, 13},
		{"exactly 3 days before", def, 50001, at("2027-08-12", 12, 0, loc), 0, 25000, 1, 3}, // 25000.5 -> 25000
		{"2 days before nothing", def, 50000, at("2027-08-13", 12, 0, loc), 0, 0, 2, 2},
		{"start day nothing", def, 50000, at("2027-08-15", 7, 0, loc), 0, 0, 2, 0},
		{"after start no tier", def, 50000, at("2027-08-16", 7, 0, loc), 0, 0, -1, -1},
		{"nothing paid", def, 0, at("2027-06-16", 10, 0, loc), 0, 0, 0, 60},
		{"no tiers configured", Rules{}, 50000, at("2027-06-16", 10, 0, loc), 0, 0, -1, 60},
		{"unsorted tiers: 30+ days", unsorted, 10000, at("2027-07-01", 10, 0, loc), 10000, 0, 1, 45},
		{"unsorted tiers: 7..29 days", unsorted, 10000, at("2027-08-01", 10, 0, loc), 0, 5000, 2, 14},
		{"unsorted tiers: 0..6 days", unsorted, 10000, at("2027-08-14", 10, 0, loc), 1000, 0, 0, 1},
		{"no zero tier: last days nothing", Rules{Cancellation: []CancellationTier{{UntilDaysBefore: 5, RefundPercent: 100}}}, 10000, at("2027-08-12", 10, 0, loc), 0, 0, -1, 3},
		// 2027-08-01 22:30 UTC is 2027-08-02 00:30 in Rome: 13 days before.
		{"timezone boundary", def, 50000, time.Date(2027, 8, 1, 22, 30, 0, 0, time.UTC), 0, 25000, 1, 13},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CancellationOutcome(start, tc.paid, tc.now, loc, tc.rules)
			if got.RefundAmount != tc.refund || got.CreditAmount != tc.credit || got.TierApplied != tc.tier || got.DaysBefore != tc.daysBefore {
				t.Fatalf("got %+v, want refund=%d credit=%d tier=%d days=%d", got, tc.refund, tc.credit, tc.tier, tc.daysBefore)
			}
		})
	}
}

func TestModificationAllowed_Table(t *testing.T) {
	loc := rome(t)
	start := calendar.MustParse("2027-08-15")
	withFee := Rules{Modification: &Modification{AllowedUntilDaysBefore: Ptr(2), Fee: Ptr(int64(500))}}
	tests := []struct {
		name    string
		rules   Rules
		now     time.Time
		allowed bool
		fee     money.Cents
	}{
		{"no rule not allowed", Rules{}, at("2027-08-01", 10, 0, loc), false, 0},
		{"rule without days not allowed", Rules{Modification: &Modification{Fee: Ptr(int64(1))}}, at("2027-08-01", 10, 0, loc), false, 0},
		{"well before", DefaultRules(), at("2027-08-01", 10, 0, loc), true, 0},
		{"exactly 2 days before", withFee, at("2027-08-13", 23, 0, loc), true, 500},
		{"1 day before", withFee, at("2027-08-14", 9, 0, loc), false, 0},
		{"after start", Rules{Modification: &Modification{AllowedUntilDaysBefore: Ptr(0)}}, at("2027-08-16", 9, 0, loc), false, 0},
		{"same day with 0", Rules{Modification: &Modification{AllowedUntilDaysBefore: Ptr(0)}}, at("2027-08-15", 9, 0, loc), true, 0},
		{"negative fee floored", Rules{Modification: &Modification{AllowedUntilDaysBefore: Ptr(0), Fee: Ptr(int64(-3))}}, at("2027-08-10", 9, 0, loc), true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ModificationAllowed(start, tc.now, loc, tc.rules)
			if got.Allowed != tc.allowed || got.Fee != tc.fee {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestWeatherOutcome_Table(t *testing.T) {
	tests := []struct {
		name           string
		rules          Rules
		amount         money.Cents
		applies        bool
		refund, credit money.Cents
	}{
		{"no policy", Rules{}, 1000, false, 0, 0},
		{"disabled", Rules{WeatherPolicy: &WeatherPolicy{Enabled: Ptr(false), As: Ptr(AsCredit)}}, 1000, false, 0, 0},
		{"credit", Rules{WeatherPolicy: &WeatherPolicy{Enabled: Ptr(true), As: Ptr(AsCredit)}}, 1000, true, 0, 1000},
		{"refund", Rules{WeatherPolicy: &WeatherPolicy{Enabled: Ptr(true), As: Ptr(AsRefund)}}, 1000, true, 1000, 0},
		{"default as is refund", Rules{WeatherPolicy: &WeatherPolicy{Enabled: Ptr(true)}}, 700, true, 700, 0},
		{"zero amount", Rules{WeatherPolicy: &WeatherPolicy{Enabled: Ptr(true)}}, 0, false, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := WeatherOutcome(tc.amount, tc.rules)
			if got.Applies != tc.applies || got.RefundAmount != tc.refund || got.CreditAmount != tc.credit {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestSeasonReleaseCreditAmount_Table(t *testing.T) {
	tests := []struct {
		name   string
		rules  Rules
		resold money.Cents
		want   money.Cents
	}{
		{"no config", Rules{}, 3000, 0},
		{"disabled", Rules{SeasonReleaseCredit: &SeasonReleaseCredit{Enabled: Ptr(false), Percent: Ptr(50)}}, 3000, 0},
		{"50 percent", Rules{SeasonReleaseCredit: &SeasonReleaseCredit{Enabled: Ptr(true), Percent: Ptr(50)}}, 3000, 1500},
		{"50 percent tie even", Rules{SeasonReleaseCredit: &SeasonReleaseCredit{Enabled: Ptr(true), Percent: Ptr(50)}}, 2501, 1250},
		{"100 percent", Rules{SeasonReleaseCredit: &SeasonReleaseCredit{Enabled: Ptr(true), Percent: Ptr(100)}}, 2501, 2501},
		{"enabled without percent", Rules{SeasonReleaseCredit: &SeasonReleaseCredit{Enabled: Ptr(true)}}, 2501, 0},
		{"negative resold", Rules{SeasonReleaseCredit: &SeasonReleaseCredit{Enabled: Ptr(true), Percent: Ptr(50)}}, -10, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SeasonReleaseCreditAmount(tc.resold, tc.rules); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
