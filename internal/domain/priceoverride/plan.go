package priceoverride

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Package priceoverride is the functional core of the manual price of a
// season pass (ADR 0019 E, INV-77), shared by the pass service (set and
// clear) and the booking service (re-pricing edits keep the manual price).
//
// The override never touches price_snapshot (INV-21). It writes
// price_current as the breakdown that prices the pass from now on, so that
// total_amount keeps equal to the total of CurrentPrice() (INV-12) and the
// payments split their VAT on its rate mix (INV-16):
//
//   - the engine breakdown (the current one before the first override) is
//     copied, a booking-level discount with source Source and amount
//     (override - engine total) is appended (positive when the manager
//     raises the price), Total becomes the override;
//   - the VAT lines are re-split on the new total in proportion to their
//     gross amounts (largest remainder, then money.VATSplit per rate), so
//     Taxable + Tax per rate sums exactly to the new total;
//   - the extra "override" member keeps the engine breakdown and the
//     previous price_current (null when the pass was priced by the
//     snapshot). Clearing the override restores them exactly: the engine
//     price of the pass, as the engine computed it (snapshot rules never
//     change after confirmation).
//
// Readers of price_current decode it as pricing.PriceBreakdown and ignore the
// extra member.

// Source is the discount source of the override line in price_current.
const Source = "manual_price_override"

// Limits of the override input (api.SeasonPassPriceOverrideInput).
const (
	minReasonLen = 3
	maxReasonLen = 500
	// MaxAmount caps the amount at 1,000,000 euro (input sanity, int64 safe).
	MaxAmount = money.Cents(100_000_000)
)

// Breakdown is price_current while an override is set.
type Breakdown struct {
	pricing.PriceBreakdown
	Override *Origin `json:"override,omitempty"`
}

// Origin is what clearing the override restores.
type Origin struct {
	// Engine is the engine breakdown replaced by the override.
	Engine pricing.PriceBreakdown `json:"engine"`
	// Previous is price_current before the first override (null: snapshot).
	Previous json.RawMessage `json:"previous"`
}

// Validate checks the input of SetPriceOverride.
func Validate(amount money.Cents, reason string) error {
	ve := &configerr.ValidationError{}
	if amount < 0 || amount > MaxAmount {
		ve.Add("amount", "L'importo deve essere compreso tra 0 e 1.000.000 €.")
	}
	if n := len([]rune(reason)); n < minReasonLen || n > maxReasonLen {
		ve.Add("reason", "Il motivo è obbligatorio (da 3 a 500 caratteri).")
	}
	return ve.OrNil()
}

// overridable are the statuses in which the price of a pass may
// still change (not terminal).
var overridable = map[string]bool{
	"draft": true, "held": true, "pending_payment": true, "awaiting_approval": true, "confirmed": true, "checked_in": true,
}

// Overridable reports whether the price of a pass in status may still change.
func Overridable(status string) bool { return overridable[status] }

// EngineOf returns the engine breakdown currently pricing b and the raw
// price_current to restore on clear: for a pass already overridden, the
// stored origin; otherwise CurrentPrice() and the current price_current.
func EngineOf(b *store.Booking) (pricing.PriceBreakdown, json.RawMessage, error) {
	if len(b.PriceCurrent) > 0 {
		var ob Breakdown
		if err := json.Unmarshal(b.PriceCurrent, &ob); err != nil {
			return pricing.PriceBreakdown{}, nil, fmt.Errorf("priceoverride: price current: %w", err)
		}
		if ob.Override != nil {
			return ob.Override.Engine, ob.Override.Previous, nil
		}
	}
	var bd pricing.PriceBreakdown
	if err := json.Unmarshal(b.CurrentPrice(), &bd); err != nil {
		return pricing.PriceBreakdown{}, nil, fmt.Errorf("priceoverride: price: %w", err)
	}
	var prev json.RawMessage
	if len(b.PriceCurrent) > 0 {
		prev = json.RawMessage(b.PriceCurrent)
	}
	return bd, prev, nil
}

// Apply returns price_current for the override amount on top of the
// engine breakdown (see the package comment above).
func Apply(engine pricing.PriceBreakdown, previous json.RawMessage, amount money.Cents) (store.JSON, error) {
	out := engine
	out.Lines = append([]pricing.LineBreakdown(nil), engine.Lines...)
	out.Discounts = append(append([]pricing.DiscountBreakdown{}, engine.Discounts...),
		pricing.DiscountBreakdown{Source: Source, Amount: amount - engine.Total})
	out.Total = amount
	out.VAT = RescaleVAT(engine.VAT, amount)
	if previous == nil {
		previous = json.RawMessage("null")
	}
	raw, err := json.Marshal(Breakdown{PriceBreakdown: out, Override: &Origin{Engine: engine, Previous: previous}})
	if err != nil {
		return nil, fmt.Errorf("priceoverride: %w", err)
	}
	return store.JSON(raw), nil
}

// Restore returns price_current and the total after clearing the
// override of b (b.PriceCurrent must carry the override origin).
func Restore(b *store.Booking) (store.JSON, money.Cents, error) {
	engine, prev, err := EngineOf(b)
	if err != nil {
		return nil, 0, err
	}
	if len(prev) == 0 || string(prev) == "null" {
		return nil, engine.Total, nil
	}
	return store.JSON(prev), engine.Total, nil
}

// RescaleVAT splits total over the rates of vat in proportion to their
// gross amounts (largest remainder, ties to the lower rate); each share is
// split into taxable and tax with money.VATSplit. Rates that cannot be read
// as basis points keep their proportion with the stored ratio.
func RescaleVAT(vat []pricing.VATBreakdown, total money.Cents) []pricing.VATBreakdown {
	if len(vat) == 0 {
		return vat
	}
	idx := make([]int, len(vat))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return vat[idx[a]].Rate.Cmp(vat[idx[b]].Rate) < 0 })
	weights := make([]int64, len(vat))
	var sum int64
	for k, i := range idx {
		weights[k] = int64(vat[i].Taxable + vat[i].Tax)
		sum += weights[k]
	}
	parts := largestRemainder(int64(total), weights, sum)
	out := make([]pricing.VATBreakdown, 0, len(vat))
	for k, i := range idx {
		g := money.Cents(parts[k])
		v := pricing.VATBreakdown{Rate: vat[i].Rate}
		if bp, ok := vat[i].Rate.BasisPoints(); ok {
			v.Taxable, v.Tax = money.VATSplit(g, bp)
		} else {
			v.Taxable = g
		}
		out = append(out, v)
	}
	return out
}

// largestRemainder splits total (>= 0) in proportion to weights; with no
// weight everything goes to the first part.
func largestRemainder(total int64, weights []int64, sum int64) []int64 {
	out := make([]int64, len(weights))
	if len(weights) == 0 || total == 0 {
		return out
	}
	if sum <= 0 {
		out[0] = total
		return out
	}
	type rem struct {
		i int
		r int64
	}
	rems := make([]rem, len(weights))
	var assigned int64
	for i, w := range weights {
		// total and w are bounded by MaxAmount-like amounts (< 2^40), so
		// the product fits in int64.
		p := total * w
		out[i] = p / sum
		assigned += out[i]
		rems[i] = rem{i, p % sum}
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r > rems[b].r })
	for k := 0; assigned < total; k++ {
		out[rems[k%len(rems)].i]++
		assigned++
	}
	return out
}
