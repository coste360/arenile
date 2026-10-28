package pricing

import (
	"fmt"
	"slices"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Agreement terms of M8 (spec 30.2.2, ADR 0028), still in the discount
// stage (step 5) so that the applied price, the discount and the snapshot
// (INV-21) keep their meaning:
//
//   - Scope restricts the agreement to some packs (empty = all) and, only
//     when ApplyToExtras is set, to extras (ExtraIDs, empty = all). A nil
//     Scope keeps the behaviour before M8 (every pack and extra).
//   - PackPrices give a fixed price to a pack ("giornaliero residenti 12
//     euro"): per day (default) the pack costs Amount on each day of each
//     line, per line Amount for the whole line, per booking Amount for all
//     the lines of the pack together. The discount is the list price minus
//     the fixed price, never negative: a fixed price above the list price
//     leaves the list price. Units of other packs, and extras, get Action
//     (when set).

// AgreementPackPrice is a fixed price of a pack in an agreement.
type AgreementPackPrice struct {
	PackID string      `json:"pack_id"`
	Amount money.Cents `json:"amount"`
	// Per is day (default), line or booking.
	Per AmountPer `json:"per,omitempty"`
}

// AgreementScope restricts the units an agreement applies to.
type AgreementScope struct {
	PackIDs       []string `json:"pack_ids,omitempty"`
	ExtraIDs      []string `json:"extra_ids,omitempty"`
	ApplyToExtras bool     `json:"apply_to_extras,omitempty"`
}

// covers reports whether the scope admits u (nil scope: everything).
func (s *AgreementScope) covers(u unit) bool {
	if s == nil {
		return true
	}
	switch u.target {
	case TargetPack:
		return len(s.PackIDs) == 0 || slices.Contains(s.PackIDs, u.itemID)
	case TargetExtra:
		return s.ApplyToExtras && (len(s.ExtraIDs) == 0 || slices.Contains(s.ExtraIDs, u.itemID))
	}
	return false
}

func validatePackPrices(id string, pp []AgreementPackPrice) error {
	seen := map[string]bool{}
	for i, p := range pp {
		field := fmt.Sprintf("pack_prices[%d]", i)
		switch {
		case p.PackID == "":
			return &RuleError{RuleID: id, Field: field + ".pack_id", Reason: "required"}
		case seen[p.PackID]:
			return &RuleError{RuleID: id, Field: field + ".pack_id", Reason: "duplicate pack"}
		case p.Amount < 0:
			return &RuleError{RuleID: id, Field: field + ".amount", Reason: "must be >= 0"}
		}
		switch p.Per {
		case "", PerDay, PerLine, PerBooking:
		default:
			return &RuleError{RuleID: id, Field: field + ".per", Reason: fmt.Sprintf("unknown value %q", p.Per)}
		}
		seen[p.PackID] = true
	}
	return nil
}

// computeAgreementDiscount is computeDiscount for a source with pack
// prices: the units of a priced pack are brought down to the fixed price,
// the other eligible units get the action. Sources without pack prices
// fall back to computeDiscount.
func computeAgreementDiscount(src discountSource, b *bookingCtx, units []unit, idx []int) (money.Cents, []int64) {
	if len(src.packPrices) == 0 {
		return computeDiscount(src.action, b, units, idx)
	}
	prices := make(map[string]AgreementPackPrice, len(src.packPrices))
	for _, p := range src.packPrices {
		prices[p.PackID] = p
	}
	alloc := make([]int64, len(idx))
	var rest []int // positions in idx without a fixed price
	type groupKey struct {
		line int
		date calendar.Date
		pack string
	}
	groups := map[groupKey][]int{}
	var order []groupKey
	for k, i := range idx {
		u := units[i]
		pp, ok := prices[u.itemID]
		if u.target != TargetPack || !ok {
			rest = append(rest, k)
			continue
		}
		key := groupKey{pack: u.itemID}
		switch pp.Per {
		case PerBooking:
		case PerLine:
			key.line = u.line
		default: // day
			key.line = u.line
			if !u.whole {
				key.date = u.date
			}
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], k)
	}
	var total money.Cents
	for _, key := range order {
		ks := groups[key]
		pp := prices[key.pack]
		var sum money.Cents
		weights := make([]int64, len(ks))
		whole := false
		for j, k := range ks {
			u := units[idx[k]]
			sum += u.amount
			weights[j] = int64(u.amount)
			whole = whole || u.whole
		}
		target := pp.Amount
		if (pp.Per == "" || pp.Per == PerDay) && whole {
			target = pp.Amount * money.Cents(len(b.dates)) // fixed-duration pack: per day x days
		}
		d := min(max(sum-target, 0), sum)
		if d == 0 {
			continue
		}
		for j, a := range allocate(int64(d), weights) {
			alloc[ks[j]] += a
		}
		total += d
	}
	if len(rest) > 0 && !noAction(src.action) {
		sub := make([]int, len(rest))
		for j, k := range rest {
			sub[j] = idx[k]
		}
		d, a := computeDiscount(src.action, b, units, sub)
		for j, k := range rest {
			alloc[k] += a[j]
		}
		total += d
	}
	return total, alloc
}

// noAction reports whether a is empty or a zero discount (an agreement with
// fixed pack prices only; the API always carries an action).
func noAction(a Action) bool {
	switch a.Type {
	case "":
		return true
	case ActionPercentOff:
		if a.Percent == nil {
			return true
		}
		bp, _ := a.Percent.BasisPoints()
		return bp == 0
	case ActionAmountOff:
		return a.Amount == 0
	}
	return false
}
