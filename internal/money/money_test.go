package money

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

func TestDivRoundHalfEven_Table(t *testing.T) {
	tests := []struct {
		num, den, want int64
	}{
		{0, 5, 0},
		{10, 1, 10},
		{-10, 1, -10},
		{1, 3, 0},
		{2, 3, 1},
		{-1, 3, 0},
		{-2, 3, -1},
		{5, 2, 2},   // 2.5 -> 2 (even)
		{7, 2, 4},   // 3.5 -> 4 (even)
		{-5, 2, -2}, // -2.5 -> -2 (even)
		{-7, 2, -4}, // -3.5 -> -4 (even)
		{-3, 2, -2}, // -1.5 -> -2 (even)
		{1, 2, 0},   // 0.5 -> 0
		{-1, 2, 0},  // -0.5 -> 0
		{15, 10, 2}, // 1.5 -> 2
		{25, 10, 2}, // 2.5 -> 2
		{26, 10, 3},
		{24, 10, 2},
		{math.MaxInt64, 1, math.MaxInt64},
	}
	for _, tt := range tests {
		if got := DivRoundHalfEven(tt.num, tt.den); got != tt.want {
			t.Errorf("DivRoundHalfEven(%d, %d) = %d, want %d", tt.num, tt.den, got, tt.want)
		}
	}
}

func TestDivRoundHalfEven_PanicsOnNonPositiveDen(t *testing.T) {
	for _, den := range []int64{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("den=%d: expected panic", den)
				}
			}()
			DivRoundHalfEven(1, den)
		}()
	}
}

// refMulRat is the reference: exact rational product rounded half-even.
func refMulRat(c, num, den int64) int64 {
	r := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(c), big.NewInt(num)), big.NewInt(den))
	fl := new(big.Int).Div(r.Num(), r.Denom()) // Euclidean: floor for positive denom
	frac := new(big.Rat).Sub(r, new(big.Rat).SetInt(fl))
	half := big.NewRat(1, 2)
	switch frac.Cmp(half) {
	case 1:
		fl.Add(fl, big.NewInt(1))
	case 0:
		if fl.Bit(0) == 1 {
			fl.Add(fl, big.NewInt(1))
		}
	}
	return fl.Int64()
}

func TestMulRat_Table(t *testing.T) {
	tests := []struct {
		name     string
		c        Cents
		num, den int64
		want     Cents
	}{
		{"identity", 1234, 1, 1, 1234},
		{"ten percent", 1234, 1000, 10000, 123},
		{"half tie to even down", 5, 1, 2, 2},
		{"half tie to even up", 15, 1, 10, 2},
		{"negative", -1234, 1, 2, -617},
		{"negative tie", -5, 1, 2, -2},
		// big.Int branch: c*num overflows int64.
		{"overflow exact", 4_000_000_000_000_000_000, 3, 4, 3_000_000_000_000_000_000},
		{"overflow negative", -4_000_000_000_000_000_000, 5, 10, -2_000_000_000_000_000_000},
		{"overflow tie to even", math.MaxInt64, 2, 4, 4611686018427387904},
		{"overflow tie negative", math.MinInt64 + 1, 2, 4, -4611686018427387904},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.MulRat(tt.num, tt.den); got != tt.want {
				t.Fatalf("%d.MulRat(%d,%d) = %d, want %d", tt.c, tt.num, tt.den, got, tt.want)
			}
			if ref := refMulRat(int64(tt.c), tt.num, tt.den); ref != int64(tt.want) {
				t.Fatalf("table disagrees with reference: %d", ref)
			}
		})
	}
}

func TestMulRat_MatchesBigRatReference(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 20000; i++ {
		var c int64
		if i%2 == 0 {
			c = r.Int64N(2_000_000) - 1_000_000
		} else {
			c = r.Int64() - math.MaxInt64/2 // large values exercise the big.Int branch
		}
		num := r.Int64N(30000) + 1
		den := num + r.Int64N(30000) // ratio <= 1 keeps the result in int64
		if got, want := Cents(c).MulRat(num, den), refMulRat(c, num, den); int64(got) != want {
			t.Fatalf("MulRat(%d, %d/%d) = %d, want %d", c, num, den, got, want)
		}
	}
}

func TestPercentOfBasisPoints_Table(t *testing.T) {
	tests := []struct {
		c    Cents
		bp   int64
		want Cents
	}{
		{10000, 1000, 1000},
		{999, 1000, 100}, // 99.9 -> 100
		{995, 1000, 100}, // 99.5 -> 100 (even)
		{985, 1000, 98},  // 98.5 -> 98 (even)
		{1234, 0, 0},
		{1234, 10000, 1234},
	}
	for _, tt := range tests {
		if got := tt.c.PercentOfBasisPoints(tt.bp); got != tt.want {
			t.Errorf("%d.PercentOfBasisPoints(%d) = %d, want %d", tt.c, tt.bp, got, tt.want)
		}
	}
}

func TestVATSplit_SumsToGross(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for _, rate := range []int64{400, 500, 1000, 2200} {
		for i := 0; i < 5000; i++ {
			gross := Cents(r.Int64N(10_000_000))
			taxable, tax := VATSplit(gross, rate)
			if taxable+tax != gross {
				t.Fatalf("rate %d gross %d: %d + %d != gross", rate, gross, taxable, tax)
			}
			if tax < 0 || taxable < 0 {
				t.Fatalf("rate %d gross %d: negative part %d/%d", rate, gross, taxable, tax)
			}
			// taxable is the nearest cent to gross/(1+rate).
			if want := refMulRat(int64(gross), 10000, 10000+rate); int64(taxable) != want {
				t.Fatalf("rate %d gross %d: taxable %d, want %d", rate, gross, taxable, want)
			}
		}
	}
}

func TestVATSplit_Table(t *testing.T) {
	tests := []struct {
		gross        Cents
		rate         int64
		taxable, tax Cents
	}{
		{12200, 2200, 10000, 2200},
		{11000, 1000, 10000, 1000},
		{1000, 2200, 820, 180}, // 819.67 -> 820
		{10400, 400, 10000, 400},
		{10500, 500, 10000, 500},
		{0, 2200, 0, 0},
	}
	for _, tt := range tests {
		taxable, tax := VATSplit(tt.gross, tt.rate)
		if taxable != tt.taxable || tax != tt.tax {
			t.Errorf("VATSplit(%d, %d) = %d/%d, want %d/%d", tt.gross, tt.rate, taxable, tax, tt.taxable, tt.tax)
		}
	}
}

func TestCents_String(t *testing.T) {
	tests := map[Cents]string{0: "0.00", 5: "0.05", 100: "1.00", 1234: "12.34", -5: "-0.05", -1234: "-12.34"}
	for c, want := range tests {
		if got := c.String(); got != want {
			t.Errorf("Cents(%d).String() = %q, want %q", int64(c), got, want)
		}
	}
}
