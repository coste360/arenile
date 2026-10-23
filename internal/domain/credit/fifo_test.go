package credit

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

var base = time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)

func mv(id string, amount money.Cents, source string, day int, expires *calendar.Date, ref string) store.CustomerCredit {
	c := store.CustomerCredit{ID: id, Amount: amount, Source: source, CreatedAt: base.AddDate(0, 0, day), ExpiresOn: expires}
	if ref != "" {
		c.ReferenceCreditID = &ref
	}
	return c
}

func date(s string) *calendar.Date {
	d := calendar.MustParse(s)
	return &d
}

func TestAllocate_Scenarios(t *testing.T) {
	tests := []struct {
		name      string
		ledger    []store.CustomerCredit
		today     string
		balance   money.Cents
		available money.Cents
		due       []money.Cents // remaining of the lots ExpireDue would close
		expiring  []Expiring
	}{
		{
			name:    "empty",
			today:   "2027-07-10",
			balance: 0, available: 0,
		},
		{
			name: "debit consumes the earliest expiring lot first",
			ledger: []store.CustomerCredit{
				mv("a", 5000, store.CreditSourceGoodwill, 0, nil, ""),
				mv("b", 3000, store.CreditSourceCancellation, 1, date("2027-07-20"), ""),
				mv("c", -4000, store.CreditSourceRedemption, 2, nil, ""),
			},
			today:   "2027-07-10",
			balance: 4000, available: 4000,
			expiring: nil, // b fully consumed, a (no expiry) has 4000 left
		},
		{
			name: "expired lot not yet materialised is not available",
			ledger: []store.CustomerCredit{
				mv("a", 5000, store.CreditSourceGoodwill, 0, nil, ""),
				mv("b", 3000, store.CreditSourceCancellation, 1, date("2027-07-05"), ""),
			},
			today:   "2027-07-10",
			balance: 8000, available: 5000, due: []money.Cents{3000},
		},
		{
			name: "materialised expiry closes the lot",
			ledger: []store.CustomerCredit{
				mv("a", 5000, store.CreditSourceGoodwill, 0, nil, ""),
				mv("b", 3000, store.CreditSourceCancellation, 1, date("2027-07-05"), ""),
				mv("x", -3000, store.CreditSourceExpiry, 9, nil, "b"),
			},
			today:   "2027-07-10",
			balance: 5000, available: 5000,
		},
		{
			name: "partially spent lot expires only for what is left",
			ledger: []store.CustomerCredit{
				mv("b", 3000, store.CreditSourceCancellation, 0, date("2027-07-05"), ""),
				mv("c", -1000, store.CreditSourceRedemption, 2, nil, ""),
			},
			today:   "2027-07-10",
			balance: 2000, available: 0, due: []money.Cents{2000},
		},
		{
			name: "expiring grouped by date",
			ledger: []store.CustomerCredit{
				mv("a", 1000, store.CreditSourceGoodwill, 0, date("2027-08-01"), ""),
				mv("b", 2000, store.CreditSourceManual, 0, date("2027-08-01"), ""),
				mv("c", 500, store.CreditSourceManual, 0, date("2027-09-01"), ""),
			},
			today:   "2027-07-10",
			balance: 3500, available: 3500,
			expiring: []Expiring{{ExpiresOn: calendar.MustParse("2027-08-01"), Amount: 3000}, {ExpiresOn: calendar.MustParse("2027-09-01"), Amount: 500}},
		},
		{
			name: "debit after expiry skips the expired lot (legacy data)",
			ledger: []store.CustomerCredit{
				mv("b", 3000, store.CreditSourceCancellation, 0, date("2027-07-02"), ""),
				mv("a", 2000, store.CreditSourceGoodwill, 1, nil, ""),
				mv("c", -1000, store.CreditSourceRedemption, 5, nil, ""),
			},
			today:   "2027-07-10",
			balance: 4000, available: 1000, due: []money.Cents{3000},
		},
		{
			name: "expiry row of the same instant is replayed before the debit",
			ledger: []store.CustomerCredit{
				mv("b", 3000, store.CreditSourceCancellation, 0, date("2027-07-05"), ""),
				mv("a", 2000, store.CreditSourceGoodwill, 0, nil, ""),
				mv("0-debit", -2000, store.CreditSourceRedemption, 9, nil, ""),
				mv("1-expiry", -3000, store.CreditSourceExpiry, 9, nil, "b"),
			},
			today:   "2027-07-10",
			balance: 0, available: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			today := calendar.MustParse(tt.today)
			a := allocate(tt.ledger)
			if a.Balance != tt.balance {
				t.Fatalf("balance %d, want %d", a.Balance, tt.balance)
			}
			if got := a.available(today); got != tt.available {
				t.Fatalf("available %d, want %d", got, tt.available)
			}
			var due []money.Cents
			for _, g := range a.dueExpiries(today) {
				due = append(due, g.Remaining)
			}
			if fmt.Sprint(due) != fmt.Sprint(tt.due) {
				t.Fatalf("due %v, want %v", due, tt.due)
			}
			if got := a.expiring(today); fmt.Sprint(got) != fmt.Sprint(tt.expiring) && (len(got) != 0 || len(tt.expiring) != 0) {
				t.Fatalf("expiring %v, want %v", got, tt.expiring)
			}
			if a.Unallocated != 0 {
				t.Fatalf("unallocated %d", a.Unallocated)
			}
		})
	}
}

// TestAllocate_BalanceInvariant replays random ledgers written the way the
// service writes them (expiry materialised before every debit, debits never
// above the available amount) and checks the ledger invariants: balance =
// sum of the movements = sum of what is left of the lots, never negative;
// available <= balance; after materialising the due expiries nothing is
// due and the available amount does not change.
func TestAllocate_BalanceInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var ledger []store.CustomerCredit
		n := rapid.IntRange(1, 40).Draw(t, "n")
		id := 0
		next := func() string { id++; return fmt.Sprintf("m%03d", id) }
		day := 0
		materialise := func(today calendar.Date) {
			for _, g := range allocate(ledger).dueExpiries(today) {
				ledger = append(ledger, mv(next(), -g.Remaining, store.CreditSourceExpiry, day, nil, g.ID))
			}
		}
		for range n {
			day += rapid.IntRange(0, 5).Draw(t, "gap")
			today := calendar.In(base.AddDate(0, 0, day), time.UTC)
			switch rapid.IntRange(0, 2).Draw(t, "kind") {
			case 0, 1:
				amount := money.Cents(rapid.Int64Range(1, 10_000).Draw(t, "grant"))
				var exp *calendar.Date
				if rapid.Bool().Draw(t, "expires") {
					d := today.AddDays(rapid.IntRange(0, 15).Draw(t, "validity"))
					exp = &d
				}
				ledger = append(ledger, mv(next(), amount, store.CreditSourceGoodwill, day, exp, ""))
			default:
				materialise(today)
				avail := allocate(ledger).available(today)
				if avail == 0 {
					continue
				}
				spend := money.Cents(rapid.Int64Range(1, int64(avail)).Draw(t, "spend"))
				ledger = append(ledger, mv(next(), -spend, store.CreditSourceRedemption, day, nil, ""))
			}
			a := allocate(ledger)
			var sum, left money.Cents
			for _, m := range ledger {
				sum += m.Amount
			}
			for _, g := range a.Grants {
				if g.Remaining < 0 {
					t.Fatalf("negative lot %s: %d", g.ID, g.Remaining)
				}
				left += g.Remaining
			}
			if a.Balance != sum || a.Balance < 0 {
				t.Fatalf("balance %d, sum %d", a.Balance, sum)
			}
			if a.Unallocated != 0 || left != a.Balance {
				t.Fatalf("lots %d + unallocated %d != balance %d", left, a.Unallocated, a.Balance)
			}
			before := a.available(today)
			if before > a.Balance {
				t.Fatalf("available %d > balance %d", before, a.Balance)
			}
			materialise(today)
			after := allocate(ledger)
			if len(after.dueExpiries(today)) != 0 || after.available(today) != before || after.Balance != before {
				t.Fatalf("after expiry: due %d, available %d (was %d), balance %d",
					len(after.dueExpiries(today)), after.available(today), before, after.Balance)
			}
		}
	})
}
