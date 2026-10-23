package payment

import (
	"math/big"
	"sort"

	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the VAT split frozen on every payment (INV-16): a
// payment of a booking is attributed to the VAT rates of the booking price
// snapshot in proportion to their gross amounts (largest remainder, so the
// shares add up exactly to the payment), then each share is split with
// money.VATSplit (half-even). The corrispettivi export and the cash desk
// summary read these shares instead of re-reading the snapshot.

// snapshotVAT is the part of the price snapshot (pricing.PriceBreakdown)
// the split needs.
type snapshotVAT struct {
	VAT []pricing.VATBreakdown `json:"vat"`
}

// rateWeights returns the gross amount per VAT rate (basis points) of a
// price snapshot, ordered by rate; nil when the snapshot has no usable split.
func rateWeights(snapshot store.JSON) []store.VATShare {
	if len(snapshot) == 0 {
		return nil
	}
	var sv snapshotVAT
	if err := snapshot.Unmarshal(&sv); err != nil {
		return nil
	}
	byRate := map[int64]money.Cents{}
	for _, v := range sv.VAT {
		bp, ok := v.Rate.BasisPoints()
		if !ok || bp < 0 {
			return nil
		}
		byRate[bp] += v.Taxable + v.Tax
	}
	out := make([]store.VATShare, 0, len(byRate))
	for bp, g := range byRate {
		if g > 0 {
			out = append(out, store.VATShare{RateBP: bp, Gross: g})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RateBP < out[j].RateBP })
	if len(out) == 0 {
		return nil
	}
	return out
}

// SplitBySnapshot attributes amount (> 0) to the VAT rates of a booking
// price snapshot; with no usable snapshot the whole amount goes to
// defaultRateBP (the establishment default rate).
func SplitBySnapshot(snapshot store.JSON, amount money.Cents, defaultRateBP int64) []store.VATShare {
	return SplitByWeights(rateWeights(snapshot), amount, defaultRateBP)
}

// SplitByWeights attributes amount to the rates of weights in proportion to
// their Gross (e.g. the shares of the payment a refund refers to). The
// result always sums to amount and satisfies Taxable + Tax == Gross for
// every share; shares of 0 are omitted.
func SplitByWeights(weights []store.VATShare, amount money.Cents, defaultRateBP int64) []store.VATShare {
	if amount == 0 {
		return []store.VATShare{}
	}
	if len(weights) == 0 {
		return []store.VATShare{share(defaultRateBP, amount)}
	}
	ws := make([]int64, len(weights))
	for i, w := range weights {
		ws[i] = int64(w.Gross)
	}
	parts := allocate(int64(amount), ws)
	out := make([]store.VATShare, 0, len(parts))
	for i, g := range parts {
		if g != 0 {
			out = append(out, share(weights[i].RateBP, money.Cents(g)))
		}
	}
	return out
}

// FlatSplit is the split of an amount at a single rate (counter sales).
func FlatSplit(amount money.Cents, rateBP int64) []store.VATShare {
	return SplitByWeights(nil, amount, rateBP)
}

func share(rateBP int64, gross money.Cents) store.VATShare {
	taxable, tax := money.VATSplit(gross, rateBP)
	return store.VATShare{RateBP: rateBP, Gross: gross, Taxable: taxable, Tax: tax}
}

// allocate splits total (> 0) proportionally to weights (>= 0) with the
// largest-remainder method; ties go to the lower index; all-zero weights
// give everything to index 0. Products are computed with math/big, so no
// amount can overflow.
func allocate(total int64, weights []int64) []int64 {
	out := make([]int64, len(weights))
	var sum int64
	for _, w := range weights {
		sum += w
	}
	if sum <= 0 {
		out[0] = total
		return out
	}
	type rem struct {
		i int
		r *big.Int
	}
	rems := make([]rem, len(weights))
	bt, bs := big.NewInt(total), big.NewInt(sum)
	var assigned int64
	for i, w := range weights {
		q, r := new(big.Int).QuoRem(new(big.Int).Mul(bt, big.NewInt(w)), bs, new(big.Int))
		out[i] = q.Int64()
		assigned += out[i]
		rems[i] = rem{i, r}
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r.Cmp(rems[b].r) > 0 })
	for k := 0; assigned < total; k++ {
		out[rems[k%len(rems)].i]++
		assigned++
	}
	return out
}
