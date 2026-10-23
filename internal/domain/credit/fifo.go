package credit

import (
	"cmp"
	"slices"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the ledger: the FIFO allocation of the debits to the
// grants. Pure (no I/O, no clock): property-tested in fifo_test.go.

// grant is a positive movement with what is left of it.
type grant struct {
	ID        string
	ExpiresOn *calendar.Date
	Remaining money.Cents
	// Expired reports that an expiry row already exists for it.
	Expired bool
	order   int
}

// allocation is the state of a ledger after replaying it.
type allocation struct {
	Balance money.Cents
	Grants  []*grant
	// Unallocated is the part of the debits no grant could cover (legacy
	// data written before migration 00008); it is already in Balance.
	Unallocated money.Cents
}

// allocate replays the movements in chronological order (as stored:
// created_at, id). A grant (amount > 0) opens a lot; an expiry row closes
// what is left of its reference grant; any other debit (redemption,
// adjustment, reversal) consumes the open lots FIFO by expiry date
// (earliest expiry first, grants without expiry last, then oldest first),
// skipping lots already expired on the day of the debit. That day is the
// business day of the establishment (loc, the same calendar ExpireDue and
// the policy expiry use), never the UTC day: a debit near local midnight
// replays on the day it happened.
func allocate(movements []store.CustomerCredit, loc *time.Location) allocation {
	if loc == nil {
		loc = time.UTC
	}
	movements = chronological(movements)
	var out allocation
	byID := map[string]*grant{}
	for i := range movements {
		m := &movements[i]
		out.Balance += m.Amount
		switch {
		case m.Source == store.CreditSourceExpiry:
			if m.ReferenceCreditID == nil {
				continue
			}
			if g, ok := byID[*m.ReferenceCreditID]; ok {
				g.Remaining = max(g.Remaining+m.Amount, 0)
				g.Expired = true
			}
		case m.Amount > 0:
			g := &grant{ID: m.ID, ExpiresOn: m.ExpiresOn, Remaining: m.Amount, order: i}
			byID[m.ID] = g
			out.Grants = append(out.Grants, g)
		case m.Amount < 0:
			day := calendar.In(m.CreatedAt, loc)
			debt := -m.Amount
			for _, g := range fifoOrder(out.Grants) {
				if debt == 0 {
					break
				}
				if g.Remaining == 0 || (g.ExpiresOn != nil && g.ExpiresOn.Before(day)) {
					continue
				}
				take := min(debt, g.Remaining)
				g.Remaining -= take
				debt -= take
			}
			out.Unallocated += debt
		}
	}
	return out
}

// chronological orders the movements by created_at, the expiry rows first
// among movements of the same instant (ExpireDue runs before the debit of
// the same transaction, whose id may have been generated earlier), then as
// given (id). It returns a sorted copy.
func chronological(in []store.CustomerCredit) []store.CustomerCredit {
	out := slices.Clone(in)
	slices.SortStableFunc(out, func(a, b store.CustomerCredit) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		ae, be := a.Source == store.CreditSourceExpiry, b.Source == store.CreditSourceExpiry
		switch {
		case ae && !be:
			return -1
		case be && !ae:
			return 1
		}
		return 0
	})
	return out
}

// fifoOrder sorts the lots by consumption priority (a copy).
func fifoOrder(gs []*grant) []*grant {
	out := slices.Clone(gs)
	slices.SortStableFunc(out, func(a, b *grant) int {
		switch {
		case a.ExpiresOn == nil && b.ExpiresOn != nil:
			return 1
		case a.ExpiresOn != nil && b.ExpiresOn == nil:
			return -1
		case a.ExpiresOn != nil && b.ExpiresOn != nil:
			if c := cmp.Compare(a.ExpiresOn.String(), b.ExpiresOn.String()); c != 0 {
				return c
			}
		}
		return cmp.Compare(a.order, b.order)
	})
	return out
}

// dueExpiries are the lots expired before today with something left and
// no expiry row yet: what ExpireDue writes.
func (a allocation) dueExpiries(today calendar.Date) []*grant {
	var out []*grant
	for _, g := range a.Grants {
		if g.ExpiresOn != nil && g.ExpiresOn.Before(today) && g.Remaining > 0 && !g.Expired {
			out = append(out, g)
		}
	}
	return out
}

// available is the balance spendable today: the balance minus the lots
// already expired but not yet materialised (never negative).
func (a allocation) available(today calendar.Date) money.Cents {
	v := a.Balance
	for _, g := range a.dueExpiries(today) {
		v -= g.Remaining
	}
	return max(v, 0)
}

// expiring groups what is left of the lots expiring on or after today by
// expiry date (ascending).
func (a allocation) expiring(today calendar.Date) []Expiring {
	sum := map[string]*Expiring{}
	var out []Expiring
	for _, g := range a.Grants {
		if g.ExpiresOn == nil || g.ExpiresOn.Before(today) || g.Remaining <= 0 {
			continue
		}
		if e, ok := sum[g.ExpiresOn.String()]; ok {
			e.Amount += g.Remaining
			continue
		}
		sum[g.ExpiresOn.String()] = &Expiring{ExpiresOn: *g.ExpiresOn, Amount: g.Remaining}
	}
	out = make([]Expiring, 0, len(sum))
	for _, e := range sum {
		out = append(out, *e)
	}
	slices.SortFunc(out, func(x, y Expiring) int { return cmp.Compare(x.ExpiresOn.String(), y.ExpiresOn.String()) })
	return out
}
