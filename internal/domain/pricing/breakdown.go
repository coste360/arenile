package pricing

import (
	"math/big"
	"sort"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// PriceBreakdown is the result of Quote and serialises exactly to the price
// snapshot of spec 8.6. Amounts are VAT-inclusive cents. Invariants:
//
//	sum(Lines[].Total) + sum(Extras[].Total) + sum(Discounts[].Amount) + Rounding == Total
//	sum(VAT[].Taxable + VAT[].Tax) == Total
type PriceBreakdown struct {
	EngineVersion int             `json:"engine_version"`
	Currency      string          `json:"currency"`
	Lines         []LineBreakdown `json:"lines"`
	// Extras attached to the booking rather than to a line (absent when none).
	Extras    []ExtraBreakdown    `json:"extras,omitempty"`
	Discounts []DiscountBreakdown `json:"discounts"`
	Rounding  money.Cents         `json:"rounding"`
	Total     money.Cents         `json:"total"`
	VAT       []VATBreakdown      `json:"vat"`
}

// LineBreakdown is one priced pack.
type LineBreakdown struct {
	Pack string `json:"pack"`
	Zone string `json:"zone"`
	Spot string `json:"spot,omitempty"`
	// Fixed is set when the pack was priced with a fixed-duration price; Days
	// is then empty.
	Fixed  *FixedBreakdown  `json:"fixed,omitempty"`
	Days   []DayBreakdown   `json:"days"`
	Extras []ExtraBreakdown `json:"extras"`
	// Total is the pack price plus the line extras, before booking discounts.
	Total money.Cents `json:"total"`
}

// FixedBreakdown records a fixed-duration price.
type FixedBreakdown struct {
	Formula    Formula     `json:"formula"`
	RatePeriod string      `json:"rate_period,omitempty"`
	Amount     money.Cents `json:"amount"`
}

// DayBreakdown is the pack price of one day.
type DayBreakdown struct {
	Date    calendar.Date  `json:"date"`
	Slots   calendar.Slots `json:"slots"`
	Base    money.Cents    `json:"base"`
	Applied []AppliedRule  `json:"applied"`
	Total   money.Cents    `json:"total"`
}

// AppliedRule is one step of the day computation.
type AppliedRule struct {
	Rule   string `json:"rule"`
	Stage  Stage  `json:"stage"`
	Effect string `json:"effect"`
}

// ExtraBreakdown is a priced extra.
type ExtraBreakdown struct {
	Code  string      `json:"code"`
	Qty   int         `json:"qty"`
	Total money.Cents `json:"total"`
}

// DiscountBreakdown is a booking-level discount; Amount is negative.
type DiscountBreakdown struct {
	Source string      `json:"source"`
	Amount money.Cents `json:"amount"`
}

// VATBreakdown is the VAT split of the total for one rate.
type VATBreakdown struct {
	// Rate is the percentage (22, 10, 5.5...).
	Rate    Decimal     `json:"rate"`
	Taxable money.Cents `json:"taxable"`
	Tax     money.Cents `json:"tax"`
}

// allocate splits total (>= 0) proportionally to weights (>= 0) with the
// largest-remainder method so that the parts sum exactly to total. Ties are
// broken by lower index. With all-zero weights everything goes to index 0.
func allocate(total int64, weights []int64) []int64 {
	out := make([]int64, len(weights))
	if len(weights) == 0 || total == 0 {
		return out
	}
	var sum int64
	for _, w := range weights {
		sum += w
	}
	if sum == 0 {
		out[0] = total
		return out
	}
	type rem struct {
		idx int
		r   *big.Int
	}
	rems := make([]rem, len(weights))
	bigTotal, bigSum := big.NewInt(total), big.NewInt(sum)
	var assigned int64
	for i, w := range weights {
		p := new(big.Int).Mul(bigTotal, big.NewInt(w))
		q, r := new(big.Int).QuoRem(p, bigSum, new(big.Int))
		out[i] = q.Int64()
		assigned += out[i]
		rems[i] = rem{i, r}
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r.Cmp(rems[b].r) > 0 })
	for k := 0; assigned < total; k++ {
		out[rems[k%len(rems)].idx]++
		assigned++
	}
	return out
}

// allocateSigned is allocate for a possibly negative total.
func allocateSigned(total int64, weights []int64) []int64 {
	if total >= 0 {
		return allocate(total, weights)
	}
	out := allocate(-total, weights)
	for i := range out {
		out[i] = -out[i]
	}
	return out
}

// roundTotal applies the establishment rounding mode (half-even).
func roundTotal(v money.Cents, mode Rounding) money.Cents {
	step := int64(0)
	switch mode {
	case RoundingEuro:
		step = 100
	case RoundingFiftyCents:
		step = 50
	}
	if step == 0 {
		return v
	}
	return money.Cents(money.DivRoundHalfEven(int64(v), step) * step)
}

// vatSplit groups the final unit amounts by rate, spreads the rounding delta
// proportionally and splits each gross amount into taxable and tax.
func vatSplit(units []unit, rounding money.Cents) []VATBreakdown {
	gross := map[int64]int64{}
	var rates []int64
	for _, u := range units {
		if _, ok := gross[u.vatBP]; !ok {
			rates = append(rates, u.vatBP)
		}
		gross[u.vatBP] += int64(u.amount)
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i] < rates[j] })
	weights := make([]int64, len(rates))
	for i, r := range rates {
		weights[i] = gross[r]
	}
	shares := allocateSigned(int64(rounding), weights)
	out := make([]VATBreakdown, 0, len(rates))
	for i, r := range rates {
		g := money.Cents(weights[i] + shares[i])
		if g == 0 {
			continue
		}
		taxable, tax := money.VATSplit(g, r)
		out = append(out, VATBreakdown{Rate: NewDecimal(r, 2), Taxable: taxable, Tax: tax})
	}
	return out
}
