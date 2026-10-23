package rollover

import (
	"fmt"
	"slices"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// Pure decisions of the rollover (functional core): no database, no clock.
// The service loads the data, calls these and writes the outcome in one
// transaction. Table tests in plan_test.go.

// TargetDates returns the opening period of the target season: the source
// dates moved by the year difference with pricing.ShiftDate (same weekday
// where it makes sense, calendar anchors for holidays and month bounds),
// unless overridden. It validates the result against targetYear.
func TargetDates(src store.Season, targetYear int, opensOn, closesOn *calendar.Date) (calendar.Date, calendar.Date, error) {
	years := targetYear - src.Year
	opens, closes := pricing.ShiftDate(src.OpensOn, years, false), pricing.ShiftDate(src.ClosesOn, years, true)
	if opensOn != nil {
		opens = *opensOn
	}
	if closesOn != nil {
		closes = *closesOn
	}
	v := &configerr.ValidationError{}
	if years == 0 {
		v.Add("target_year", "Deve essere diverso dall'anno della stagione di origine.")
	}
	if opens.Year() != targetYear {
		v.Add("opens_on", "Deve cadere nell'anno di destinazione.")
	}
	if closes.Before(opens) {
		v.Add("closes_on", "Deve essere successiva o uguale a opens_on.")
	}
	return opens, closes, v.OrNil()
}

// ClosureCopy is one closure day to create in the target season.
type ClosureCopy struct {
	From   calendar.Date // date in the source season
	Date   calendar.Date // shifted date
	Slots  []calendar.Slot
	Reason string
}

// PlanClosureDays maps the closure days of the source season onto the
// target season: each date moves by years with pricing.ShiftDate (a
// recurring event on a Saturday stays on a Saturday; fixed holidays keep
// their calendar date). A shifted date outside [opens, closes], already
// closed in the target (existing, keyed by YYYY-MM-DD) or produced twice is not copied, with a
// warning. The result is sorted by date.
func PlanClosureDays(src []store.ClosureDay, years int, opens, closes calendar.Date, existing map[string]bool) ([]ClosureCopy, []string) {
	out := []ClosureCopy{}
	warnings := []string{}
	taken := map[string]bool{}
	sorted := slices.Clone(src)
	slices.SortFunc(sorted, func(a, b store.ClosureDay) int { return strings.Compare(a.Date.String(), b.Date.String()) })
	for _, c := range sorted {
		d := pricing.ShiftDate(c.Date, years, false)
		switch {
		case d.Before(opens) || d.After(closes):
			warnings = append(warnings, fmt.Sprintf("closure %s (%s) falls outside the target season and was not copied", c.Date, d))
		case existing[d.String()]:
			warnings = append(warnings, fmt.Sprintf("closure %s: the target season is already closed on %s", c.Date, d))
		case taken[d.String()]:
			warnings = append(warnings, fmt.Sprintf("closure %s: %s is already copied from another closure", c.Date, d))
		default:
			taken[d.String()] = true
			out = append(out, ClosureCopy{From: c.Date, Date: d, Slots: slices.Clone(c.Slots), Reason: c.Reason})
		}
	}
	return out, warnings
}
