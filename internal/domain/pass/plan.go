package pass

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Pure rules of season passes and releases (functional core): no I/O and
// no clock, today is passed in by the service.

// MaxReleaseDates bounds the dates of one release request.
const MaxReleaseDates = 200

// Pass interval is the season or a sub-range of it, never in the past.
type interval struct {
	from, to calendar.Date
}

// passInterval resolves the dates of a new pass: defaults are the season
// bounds, with the start moved to today when the season already started.
func passInterval(season *store.Season, startsOn, endsOn *calendar.Date, today calendar.Date) (interval, error) {
	ve := &configerr.ValidationError{}
	iv := interval{from: season.OpensOn, to: season.ClosesOn}
	if startsOn != nil {
		iv.from = *startsOn
	} else if iv.from.Before(today) {
		iv.from = today
	}
	if endsOn != nil {
		iv.to = *endsOn
	}
	if iv.from.Before(season.OpensOn) || iv.from.After(season.ClosesOn) {
		ve.Add("starts_on", fmt.Sprintf("La data di inizio deve essere nella stagione (%s – %s).", season.OpensOn, season.ClosesOn))
	}
	if iv.to.Before(season.OpensOn) || iv.to.After(season.ClosesOn) {
		ve.Add("ends_on", fmt.Sprintf("La data di fine deve essere nella stagione (%s – %s).", season.OpensOn, season.ClosesOn))
	}
	if iv.to.Before(iv.from) {
		ve.Add("ends_on", "La data di fine non può precedere quella di inizio.")
	}
	if iv.to.Before(today) {
		ve.Add("ends_on", "L'abbonamento non può terminare nel passato.")
	}
	return iv, ve.OrNil()
}

// planRelease validates a release request against the pass and returns the
// half-days to release, sorted. slots empty means the slots of the pass;
// a FULL pass may release AM, PM or FULL, an AM/PM pass only its slot.
func planRelease(b *store.Booking, dates []calendar.Date, slots calendar.Slots, today calendar.Date) ([]availability.DateSlot, error) {
	ve := &configerr.ValidationError{}
	if slots == "" {
		slots = b.Slots
	}
	if !slots.Valid() {
		ve.Add("slots", "Fascia non valida (AM, PM o FULL).")
		return nil, ve
	}
	passSlots := b.Slots.Expand()
	for _, s := range slots.Expand() {
		if !slices.Contains(passSlots, s) {
			ve.Add("slots", "L'abbonamento non copre la fascia richiesta.")
			return nil, ve
		}
	}
	if len(dates) == 0 {
		ve.Add("dates", "Indica almeno un giorno da rilasciare.")
		return nil, ve
	}
	if len(dates) > MaxReleaseDates {
		ve.Add("dates", "Al massimo 200 giorni per richiesta.")
		return nil, ve
	}
	sorted := slices.Clone(dates)
	slices.SortFunc(sorted, func(x, y calendar.Date) int { return strings.Compare(x.String(), y.String()) })
	sorted = slices.CompactFunc(sorted, func(x, y calendar.Date) bool { return x.Equal(y) })
	out := make([]availability.DateSlot, 0, len(sorted)*2)
	for i, d := range sorted {
		switch {
		case d.Before(b.StartsOn) || d.After(b.EndsOn):
			ve.Add(fmt.Sprintf("dates[%d]", i), fmt.Sprintf("Il giorno %s è fuori dal periodo dell'abbonamento.", d))
		case d.Before(today):
			ve.Add(fmt.Sprintf("dates[%d]", i), fmt.Sprintf("Il giorno %s è già passato.", d))
		default:
			for _, s := range slots.Expand() {
				out = append(out, availability.DateSlot{Date: d, Slot: s})
			}
		}
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	return out, nil
}

// overlapping returns the requested half-days already covered by an active
// release.
func overlapping(days []availability.DateSlot, active []store.SeasonRelease) []availability.DateSlot {
	taken := map[string]bool{}
	for _, r := range active {
		taken[r.Date.String()+"/"+string(r.Slot)] = true
	}
	var out []availability.DateSlot
	for _, d := range days {
		if taken[d.Date.String()+"/"+string(d.Slot)] {
			out = append(out, d)
		}
	}
	return out
}

// halfDays is the number of half-days covered by a booking.
func halfDays(b *store.Booking) int {
	return len(calendar.Range(b.StartsOn, b.EndsOn)) * len(b.Slots.Expand())
}

// resaleAmountPerHalfDay is the share of a resale line for one half-day
// (line total spread evenly on the half-days of its booking, half-even).
func resaleAmountPerHalfDay(line *store.BookingLine, b *store.Booking) money.Cents {
	n := halfDays(b)
	if n <= 0 || line.LineTotal <= 0 {
		return 0
	}
	return line.LineTotal.MulRat(1, int64(n))
}

// releaseCredit is the credit accrued by the holder for one resold half-day
// under the pass policy snapshot (spec 9, season_release_credit).
func releaseCredit(perHalfDay money.Cents, snapshot store.JSON) money.Cents {
	return policy.SeasonReleaseCreditAmount(perHalfDay, snapshotRules(snapshot))
}

// snapshotRules reads the rules of a booking policy snapshot
// ({"policy_id": ..., "rules": {...}}). A malformed snapshot yields empty
// rules, i.e. no credit.
func snapshotRules(snapshot store.JSON) policy.Rules {
	var s struct {
		Rules policy.Rules `json:"rules"`
	}
	if len(snapshot) == 0 || json.Unmarshal(snapshot, &s) != nil {
		return policy.Rules{}
	}
	return s.Rules
}

// dateStrings returns the distinct dates of days as YYYY-MM-DD, sorted.
func dateStrings(days []availability.DateSlot) []string {
	out := make([]string, 0, len(days))
	for _, d := range days {
		out = append(out, d.Date.String())
	}
	slices.Sort(out)
	return slices.Compact(out)
}
