package pass

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Releases by range and recurring weekdays (spec 30.8, ADR 0028): "dal 10
// al 20 luglio", "ogni lunedì di luglio". The range is expanded on the
// dates of the pass (never outside its season period, never in the past)
// and only the half-days the holder still occupies are released, so the
// same range sent twice (offline queue, double tap) releases nothing new:
// releases are idempotent per day. Explicit Dates keep their strict
// semantics (an already released day fails with ErrAlreadyReleased). The
// credit of a released day sold to someone else follows
// season_release_credit as for single days (Resale.MarkResold).

// MaxRangeDays bounds the span of a release range.
const MaxRangeDays = 366

// ReleaseRange is a range of days to release, optionally only on some ISO
// weekdays (1 = Monday .. 7 = Sunday; empty = every day).
type ReleaseRange struct {
	From, To calendar.Date
	Weekdays []int
}

// expandRange returns the dates of r inside the pass period and not before
// today, filtered by weekday, sorted. The result may be empty (e.g. the
// range is already past). Pure.
func expandRange(b *store.Booking, r ReleaseRange, today calendar.Date) ([]calendar.Date, error) {
	ve := &configerr.ValidationError{}
	if r.From.IsZero() {
		ve.Add("from", "Indica il primo giorno dell'intervallo.")
	}
	if r.To.IsZero() {
		ve.Add("to", "Indica l'ultimo giorno dell'intervallo.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	if r.To.Before(r.From) {
		ve.Add("to", "La data di fine non può precedere quella di inizio.")
	} else if r.From.DaysUntil(r.To)+1 > MaxRangeDays {
		ve.Add("to", fmt.Sprintf("Al massimo %d giorni per intervallo.", MaxRangeDays))
	}
	wanted := map[int]bool{}
	for i, wd := range r.Weekdays {
		if wd < 1 || wd > 7 {
			ve.Add(fmt.Sprintf("weekdays[%d]", i), "Giorno della settimana non valido (1 = lunedì … 7 = domenica).")
			continue
		}
		wanted[wd] = true
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	from, to := latest(r.From, b.StartsOn, today), earliest(r.To, b.EndsOn)
	var out []calendar.Date
	for _, d := range calendar.Range(from, to) {
		if len(wanted) == 0 || wanted[d.ISOWeekday()] {
			out = append(out, d)
		}
	}
	return out, nil
}

// releaseSlots validates the slots of a release against the pass (empty =
// the slots of the pass). Pure.
func releaseSlots(b *store.Booking, slots calendar.Slots) ([]calendar.Slot, error) {
	if slots == "" {
		slots = b.Slots
	}
	if !slots.Valid() {
		return nil, configerr.Invalid("slots", "Fascia non valida (AM, PM o FULL).")
	}
	passSlots := b.Slots.Expand()
	for _, s := range slots.Expand() {
		if !slices.Contains(passSlots, s) {
			return nil, configerr.Invalid("slots", "L'abbonamento non copre la fascia richiesta.")
		}
	}
	return slots.Expand(), nil
}

// planReleaseDays returns the half-days to release: the explicit dates
// (planRelease, strict) plus the half-days of the range the pass still
// occupies in tx. Empty when the range is already released.
func (s *Service) planReleaseDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, v *store.SeasonPassView, in ReleaseInput, today calendar.Date) ([]availability.DateSlot, error) {
	if in.Range == nil {
		return planRelease(&v.Booking, in.Dates, in.Slots, today)
	}
	dates, err := expandRange(&v.Booking, *in.Range, today)
	if err != nil {
		return nil, err
	}
	var days []availability.DateSlot
	if len(in.Dates) > 0 {
		if days, err = planRelease(&v.Booking, in.Dates, in.Slots, today); err != nil {
			return nil, err
		}
	}
	if len(dates) == 0 && len(days) == 0 {
		return nil, configerr.Invalid("from", "Nessun giorno dell'abbonamento da rilasciare in questo intervallo.")
	}
	if len(dates) == 0 {
		return days, nil
	}
	slots, err := releaseSlots(&v.Booking, in.Slots)
	if err != nil {
		return nil, err
	}
	held, err := store.SeasonAllocationsOfLine(ctx, tx, es, v.LineID, dates[0], dates[len(dates)-1])
	if err != nil {
		return nil, fmt.Errorf("pass: release range: %w", err)
	}
	occupied := make(map[availability.DateSlot]bool, len(held))
	for _, a := range held {
		occupied[availability.DateSlot{Date: a.Date, Slot: a.Slot}] = true
	}
	seen := make(map[availability.DateSlot]bool, len(days))
	for _, ds := range days {
		seen[ds] = true
	}
	for _, d := range dates {
		for _, sl := range slots {
			ds := availability.DateSlot{Date: d, Slot: sl}
			if occupied[ds] && !seen[ds] {
				seen[ds] = true
				days = append(days, ds)
			}
		}
	}
	slices.SortFunc(days, func(a, b availability.DateSlot) int {
		return cmp.Or(cmp.Compare(a.Date.String(), b.Date.String()), cmp.Compare(a.Slot, b.Slot))
	})
	return days, nil
}

func latest(ds ...calendar.Date) calendar.Date {
	out := ds[0]
	for _, d := range ds[1:] {
		if d.After(out) {
			out = d
		}
	}
	return out
}

func earliest(ds ...calendar.Date) calendar.Date {
	out := ds[0]
	for _, d := range ds[1:] {
		if d.Before(out) {
			out = d
		}
	}
	return out
}
