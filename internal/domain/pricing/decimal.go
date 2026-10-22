package pricing

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// maxDecimalScale bounds the number of fractional digits accepted in rule
// JSON (factors, percentages). Nine digits are far beyond any sensible price
// configuration and keep 10^scale inside int64.
const maxDecimalScale = 9

var errBadDecimal = errors.New("invalid decimal")

// Decimal is an exact base-10 number Units / 10^Scale. It is used for values
// that the configuration expresses as JSON numbers with decimals (e.g. a
// multiply factor of 1.2 or a percentage of 12.5) so that they never pass
// through float64. The zero value is 0.
type Decimal struct {
	Units int64
	Scale uint8
}

// NewDecimal returns Units/10^scale in normalised form.
func NewDecimal(units int64, scale uint8) Decimal { return Decimal{units, scale}.normalize() }

// ParseDecimal parses a plain decimal literal: an optional minus sign, digits,
// and an optional fractional part. Exponents are rejected on purpose.
func ParseDecimal(s string) (Decimal, error) {
	orig := s
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	intPart, frac, hasDot := strings.Cut(s, ".")
	if intPart == "" || (hasDot && frac == "") || len(frac) > maxDecimalScale {
		return Decimal{}, fmt.Errorf("%w %q", errBadDecimal, orig)
	}
	for _, r := range intPart + frac {
		if r < '0' || r > '9' {
			return Decimal{}, fmt.Errorf("%w %q", errBadDecimal, orig)
		}
	}
	u, err := strconv.ParseInt(intPart+frac, 10, 64)
	if err != nil {
		return Decimal{}, fmt.Errorf("%w %q: %w", errBadDecimal, orig, err)
	}
	if neg {
		u = -u
	}
	return Decimal{Units: u, Scale: uint8(len(frac))}.normalize(), nil
}

// MustParseDecimal is ParseDecimal that panics; for tests and constants.
func MustParseDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Decimal) normalize() Decimal {
	for d.Scale > 0 && d.Units%10 == 0 {
		d.Units /= 10
		d.Scale--
	}
	return d
}

func pow10(n uint8) int64 {
	p := int64(1)
	for i := uint8(0); i < n; i++ {
		p *= 10
	}
	return p
}

// Rat returns the value as the exact ratio num/den (den > 0).
func (d Decimal) Rat() (num, den int64) { return d.Units, pow10(d.Scale) }

// Sign returns -1, 0 or +1.
func (d Decimal) Sign() int {
	switch {
	case d.Units < 0:
		return -1
	case d.Units > 0:
		return 1
	}
	return 0
}

// Cmp compares d and o exactly: -1 if d < o, 0 if equal, +1 if d > o.
func (d Decimal) Cmp(o Decimal) int {
	an, ad := d.Rat()
	bn, bd := o.Rat()
	l := new(big.Int).Mul(big.NewInt(an), big.NewInt(bd))
	r := new(big.Int).Mul(big.NewInt(bn), big.NewInt(ad))
	return l.Cmp(r)
}

// BasisPoints interprets d as a percentage and returns it in basis points
// (12.5 -> 1250). ok is false when the value has more than two decimals.
func (d Decimal) BasisPoints() (bp int64, ok bool) {
	n := d.normalize()
	if n.Scale > 2 {
		return 0, false
	}
	return n.Units * pow10(2-n.Scale), true
}

// String formats the decimal without exponent and without trailing zeros.
func (d Decimal) String() string {
	n := d.normalize()
	if n.Scale == 0 {
		return strconv.FormatInt(n.Units, 10)
	}
	neg := n.Units < 0
	u := n.Units
	if neg {
		u = -u
	}
	s := strconv.FormatInt(u, 10)
	if len(s) <= int(n.Scale) {
		s = strings.Repeat("0", int(n.Scale)-len(s)+1) + s
	}
	cut := len(s) - int(n.Scale)
	out := s[:cut] + "." + s[cut:]
	if neg {
		out = "-" + out
	}
	return out
}

// MarshalJSON writes the decimal as a JSON number.
func (d Decimal) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalJSON accepts a JSON number (1.2) or a numeric string ("1.2").
func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	p, err := ParseDecimal(s)
	if err != nil {
		return err
	}
	*d = p
	return nil
}
