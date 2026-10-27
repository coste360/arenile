package admin

import (
	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/pass"
)

// releaseRangeFromAPI maps the range form of a season pass release (spec
// 30.8): nil when neither bound nor weekdays are given; a missing bound
// reaches the service as a zero date (validation error on from / to).
func releaseRangeFromAPI(from, to *calendar.Date, weekdays *[]int) *pass.ReleaseRange {
	if from == nil && to == nil && weekdays == nil {
		return nil
	}
	r := &pass.ReleaseRange{}
	if from != nil {
		r.From = *from
	}
	if to != nil {
		r.To = *to
	}
	if weekdays != nil {
		r.Weekdays = *weekdays
	}
	return r
}
