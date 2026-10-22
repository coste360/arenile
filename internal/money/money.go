// Package money holds amounts as int64 cents. Floats are never used for money.
package money

import (
	"fmt"
	"math/big"
)

// Cents is an amount in the smallest currency unit (EUR cents by default).
type Cents int64

// String formats the amount as "12.34" (no currency symbol).
func (c Cents) String() string {
	sign := ""
	v := int64(c)
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// DivRoundHalfEven returns num/den rounded to the nearest integer, ties to even.
// den must be positive.
func DivRoundHalfEven(num, den int64) int64 {
	if den <= 0 {
		panic("money: non-positive denominator")
	}
	q, r := num/den, num%den
	if r < 0 {
		// normalise so that 0 <= r < den and q is floor division
		q--
		r += den
	}
	twice := 2 * r
	switch {
	case twice > den:
		q++
	case twice == den && q%2 != 0:
		q++
	}
	return q
}

// MulRat multiplies c by num/den with half-even rounding to the cent.
func (c Cents) MulRat(num, den int64) Cents {
	// use big.Int to avoid overflow on large intermediate products
	p := new(big.Int).Mul(big.NewInt(int64(c)), big.NewInt(num))
	if p.IsInt64() {
		return Cents(DivRoundHalfEven(p.Int64(), den))
	}
	q, r := new(big.Int).QuoRem(p, big.NewInt(den), new(big.Int))
	if r.Sign() < 0 {
		q.Sub(q, big.NewInt(1))
		r.Add(r, big.NewInt(den))
	}
	twice := new(big.Int).Mul(r, big.NewInt(2))
	cmp := twice.Cmp(big.NewInt(den))
	if cmp > 0 || (cmp == 0 && q.Bit(0) == 1) {
		q.Add(q, big.NewInt(1))
	}
	return Cents(q.Int64())
}

// PercentOf returns percent% of c (percent in basis points: 1000 = 10.00%),
// rounded half-even to the cent.
func (c Cents) PercentOfBasisPoints(bp int64) Cents { return c.MulRat(bp, 10000) }

// VATSplit splits a VAT-inclusive gross amount into taxable base and tax for a
// rate expressed in basis points (2200 = 22%).
func VATSplit(gross Cents, rateBP int64) (taxable, tax Cents) {
	taxable = gross.MulRat(10000, 10000+rateBP)
	return taxable, gross - taxable
}
