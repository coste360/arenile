package pricing

import (
	"slices"
	"sort"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// discount source classes, in application order (spec 8.1 step 5).
const (
	classAgreement = iota
	classPromo
	classRule
)

type discountSource struct {
	label     string
	class     int
	action    Action
	conds     Conditions
	target    TargetType // "" = packs and extras
	targetIDs []string
	stackable bool
	priority  int
	seq       int64
	id        string
	// agreement terms beyond the action (M8): scope and pack prices.
	scope      *AgreementScope
	packPrices []AgreementPackPrice
}

// order is the application order: class, then priority ascending, then older
// first, then ID.
func (s discountSource) before(o discountSource) bool {
	if s.class != o.class {
		return s.class < o.class
	}
	if s.priority != o.priority {
		return s.priority < o.priority
	}
	if s.seq != o.seq {
		return s.seq < o.seq
	}
	return s.id < o.id
}

// preferred breaks ties between equally good non-stackable discounts:
// agreement, then promo, then rules by priority desc, recency desc, ID asc.
func (s discountSource) preferred(o discountSource) bool {
	if s.class != o.class {
		return s.class < o.class
	}
	if s.priority != o.priority {
		return s.priority > o.priority
	}
	if s.seq != o.seq {
		return s.seq > o.seq
	}
	return s.id < o.id
}

// eligible returns the indices of the units a discount applies to. Booking and
// line conditions are evaluated with the booking slots; day conditions use the
// unit date (the start date for whole-booking units).
func (e *engine) eligible(src discountSource, b *bookingCtx, lines []*lineCtx, units []unit) []int {
	var idx []int
	for i, u := range units {
		if src.target != "" && u.target != src.target {
			continue
		}
		if len(src.targetIDs) > 0 && !slices.Contains(src.targetIDs, u.itemID) {
			continue
		}
		if !src.scope.covers(u) {
			continue
		}
		var lc *lineCtx
		if u.line >= 0 {
			lc = lines[u.line]
		}
		if u.lc != nil {
			lc = u.lc
		}
		if !e.matches(src.conds, evalCtx{b: b, line: lc, date: u.date, slots: b.slots}) {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

// computeDiscount returns the discount (>= 0) and its allocation over the
// eligible units; no unit receives more than its remaining amount.
func computeDiscount(a Action, b *bookingCtx, units []unit, idx []int) (money.Cents, []int64) {
	weights := make([]int64, len(idx))
	var e money.Cents
	for k, i := range idx {
		weights[k] = int64(units[i].amount)
		e += units[i].amount
	}
	if e <= 0 {
		return 0, make([]int64, len(idx))
	}
	var d money.Cents
	switch a.Type {
	case ActionPercentOff:
		bp, _ := a.Percent.BasisPoints()
		d = e.PercentOfBasisPoints(bp)
	case ActionMultiply:
		n, den := a.Factor.Rat()
		d = e - e.MulRat(n, den)
	case ActionAmountOff:
		var count int64
		switch a.Per {
		case PerLine:
			groups := map[int]bool{}
			for _, i := range idx {
				groups[units[i].line] = true
			}
			count = int64(len(groups))
		case PerDay:
			days := map[calendar.Date]bool{}
			for _, i := range idx {
				if units[i].whole {
					for _, dd := range b.dates {
						days[dd] = true
					}
				} else {
					days[units[i].date] = true
				}
			}
			count = int64(len(days))
		default: // booking
			count = 1
		}
		d = a.Amount * money.Cents(count)
	case ActionFreeDays:
		return freeDays(a, units, idx)
	}
	d = min(max(d, 0), e)
	return d, allocate(int64(d), weights)
}

// freeDays implements "every N days, F are free": per line (booking-level
// extras form their own group), the eligible per-day units are grouped by
// date and the F cheapest dates of every N are discounted entirely (ties:
// earlier date). Whole-booking units are not day-based and are excluded.
func freeDays(a Action, units []unit, idx []int) (money.Cents, []int64) {
	alloc := make([]int64, len(idx))
	type dayGroup struct {
		date  calendar.Date
		sum   money.Cents
		units []int // positions in idx
	}
	byLine := map[int]map[calendar.Date]*dayGroup{}
	var lineOrder []int
	for k, i := range idx {
		u := units[i]
		if u.whole {
			continue
		}
		g, ok := byLine[u.line]
		if !ok {
			g = map[calendar.Date]*dayGroup{}
			byLine[u.line] = g
			lineOrder = append(lineOrder, u.line)
		}
		dg, ok := g[u.date]
		if !ok {
			dg = &dayGroup{date: u.date}
			g[u.date] = dg
		}
		dg.sum += u.amount
		dg.units = append(dg.units, k)
	}
	sort.Ints(lineOrder)
	var total money.Cents
	for _, l := range lineOrder {
		groups := make([]*dayGroup, 0, len(byLine[l]))
		for _, g := range byLine[l] {
			groups = append(groups, g)
		}
		sort.Slice(groups, func(i, j int) bool {
			if groups[i].sum != groups[j].sum {
				return groups[i].sum < groups[j].sum
			}
			return groups[i].date.Before(groups[j].date)
		})
		free := (len(groups) / a.Every) * a.Free
		for _, g := range groups[:free] {
			for _, k := range g.units {
				alloc[k] = int64(units[idx[k]].amount)
				total += units[idx[k]].amount
			}
		}
	}
	return total, alloc
}

// runDiscounts applies seq in order on a copy of units.
func (e *engine) runDiscounts(seq []discountSource, b *bookingCtx, lines []*lineCtx, units []unit) ([]DiscountBreakdown, []unit, money.Cents) {
	cur := slices.Clone(units)
	out := []DiscountBreakdown{}
	var total money.Cents
	for _, src := range seq {
		idx := e.eligible(src, b, lines, cur)
		d, alloc := computeAgreementDiscount(src, b, cur, idx)
		if d == 0 {
			continue
		}
		for k, i := range idx {
			cur[i].amount -= money.Cents(alloc[k])
		}
		out = append(out, DiscountBreakdown{Source: src.label, Amount: -d})
		total += d
	}
	return out, cur, total
}

// applyDiscounts runs step 5: agreement, promo code, automatic discount
// rules. Stackable sources all apply; among non-stackable sources only the one
// whose inclusion yields the lowest total applies.
func (e *engine) applyDiscounts(b *bookingCtx, lines []*lineCtx, units []unit) ([]DiscountBreakdown, []unit, error) {
	in := b.in
	var cands []discountSource
	if a := in.Agreement; a != nil && a.Active {
		if err := ValidateAgreement(*a); err != nil {
			return nil, nil, err
		}
		cands = append(cands, discountSource{
			label: "agreement:" + a.Code, class: classAgreement, action: a.Action, conds: a.Conditions,
			stackable: a.Stackable, id: a.ID, scope: a.Scope, packPrices: a.PackPrices,
		})
	}
	if p := in.PromoCode; p != nil {
		if err := ValidatePromoCode(*p); err != nil {
			return nil, nil, err
		}
		if !p.Active || (p.ValidFrom != nil && in.Today.Before(*p.ValidFrom)) || (p.ValidTo != nil && in.Today.After(*p.ValidTo)) {
			return nil, nil, ErrPromoNotApplicable
		}
		src := discountSource{
			label: "promo:" + p.Code, class: classPromo, action: p.Action, conds: p.Conditions,
			stackable: p.Stackable, id: p.ID,
		}
		var base money.Cents
		for _, i := range e.eligible(src, b, lines, units) {
			base += units[i].amount
		}
		if base <= 0 {
			return nil, nil, ErrPromoNotApplicable
		}
		cands = append(cands, src)
	}
	for _, r := range e.rules {
		if r.Stage != StageDiscount {
			continue
		}
		cands = append(cands, discountSource{
			label: "rule:" + r.ID, class: classRule, action: r.Action, conds: r.Conditions,
			target: r.TargetType, targetIDs: r.TargetIDs, stackable: r.Stackable,
			priority: r.Priority, seq: r.Seq, id: r.ID,
		})
	}
	var stack, nonStack []discountSource
	for _, c := range cands {
		if len(e.eligible(c, b, lines, units)) == 0 {
			continue
		}
		if c.stackable {
			stack = append(stack, c)
		} else {
			nonStack = append(nonStack, c)
		}
	}
	sortSeq := func(s []discountSource) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].before(s[j]) })
	}
	if len(nonStack) == 0 {
		sortSeq(stack)
		d, u, _ := e.runDiscounts(stack, b, lines, units)
		return d, u, nil
	}
	var (
		bestD     []DiscountBreakdown
		bestU     []unit
		bestTotal money.Cents
		bestSrc   discountSource
	)
	for i, n := range nonStack {
		seq := append(slices.Clone(stack), n)
		sortSeq(seq)
		d, u, total := e.runDiscounts(seq, b, lines, units)
		if i == 0 || total > bestTotal || (total == bestTotal && n.preferred(bestSrc)) {
			bestD, bestU, bestTotal, bestSrc = d, u, total, n
		}
	}
	return bestD, bestU, nil
}
