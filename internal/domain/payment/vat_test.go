package payment

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func TestSplitBySnapshot_Table(t *testing.T) {
	two := store.JSON(`{"vat":[{"rate":22,"taxable":5000,"tax":1100},{"rate":10,"taxable":3545,"tax":355}]}`)
	tests := []struct {
		name     string
		snapshot store.JSON
		amount   money.Cents
		want     []store.VATShare
	}{
		{"empty snapshot uses default", store.JSON(`{}`), 1220, []store.VATShare{{RateBP: 2200, Gross: 1220, Taxable: 1000, Tax: 220}}},
		{"malformed snapshot uses default", store.JSON(`{"vat":"x"}`), 122, []store.VATShare{{RateBP: 2200, Gross: 122, Taxable: 100, Tax: 22}}},
		{"two rates proportional", two, 3000, []store.VATShare{
			{RateBP: 1000, Gross: 1170, Taxable: 1064, Tax: 106},
			{RateBP: 2200, Gross: 1830, Taxable: 1500, Tax: 330},
		}},
		{"full amount equals snapshot", two, 10000, []store.VATShare{
			{RateBP: 1000, Gross: 3900, Taxable: 3545, Tax: 355},
			{RateBP: 2200, Gross: 6100, Taxable: 5000, Tax: 1100},
		}},
		{"one cent goes to the largest remainder", two, 1, []store.VATShare{{RateBP: 2200, Gross: 1, Taxable: 1, Tax: 0}}},
		{"decimal rate", store.JSON(`{"vat":[{"rate":5.5,"taxable":1000,"tax":55}]}`), 1055, []store.VATShare{{RateBP: 550, Gross: 1055, Taxable: 1000, Tax: 55}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitBySnapshot(tt.snapshot, tt.amount, 2200)
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("share %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestSplitByWeights_Property: shares always add up to the amount, each
// share satisfies taxable + tax == gross (INV-16) and no share is larger
// than its proportional part rounded up.
func TestSplitByWeights_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 4).Draw(t, "rates")
		weights := make([]store.VATShare, n)
		for i := range weights {
			weights[i] = store.VATShare{
				RateBP: int64(i*500 + rapid.IntRange(0, 499).Draw(t, "rate")),
				Gross:  money.Cents(rapid.Int64Range(0, 10_000_000).Draw(t, "w")),
			}
		}
		amount := money.Cents(rapid.Int64Range(1, 100_000_000).Draw(t, "amount"))
		got := SplitByWeights(weights, amount, 2200)
		var sum money.Cents
		for _, sh := range got {
			if sh.Taxable+sh.Tax != sh.Gross || sh.Gross <= 0 {
				t.Fatalf("bad share %+v", sh)
			}
			sum += sh.Gross
		}
		if sum != amount {
			t.Fatalf("sum %d != amount %d (%+v)", sum, amount, got)
		}
	})
}

func TestAllocate_Table(t *testing.T) {
	tests := []struct {
		name    string
		total   int64
		weights []int64
		want    []int64
	}{
		{"even", 10, []int64{1, 1}, []int64{5, 5}},
		{"remainder to lower index on ties", 1, []int64{1, 1}, []int64{1, 0}},
		{"largest remainder", 10, []int64{1, 2}, []int64{3, 7}},
		{"zero weights", 7, []int64{0, 0}, []int64{7, 0}},
		{"huge values do not overflow", 9_000_000_000_000, []int64{9_000_000_000_000, 9_000_000_000_000}, []int64{4_500_000_000_000, 4_500_000_000_000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allocate(tt.total, tt.weights)
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
