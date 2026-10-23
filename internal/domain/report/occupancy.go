package report

import (
	"cmp"
	"slices"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the occupancy report. The unit is (spot, date, slot).
// Capacity counts the active spots on every open half-day of the period
// (inside a season and not closed by a closure day, including closures of a
// single slot) minus the blocked units; occupied counts the booking and
// season allocations on open half-days. Released season units are deleted
// from spot_allocations by the pass service (ADR 0010), so a resold unit
// appears as a booking allocation and a free released unit only in the
// released count. Holds never count.

var slots = [2]calendar.Slot{calendar.AM, calendar.PM}

func slotIndex(s calendar.Slot) int {
	if s == calendar.PM {
		return 1
	}
	return 0
}

// openUnits returns the open half-days of p by date ("YYYY-MM-DD" -> [AM,
// PM]): a date is open when a season covers it, a slot is closed when a
// closure day lists it.
func openUnits(p Period, cal *store.ReportOpenCalendar) map[string][2]bool {
	out := make(map[string][2]bool, p.Days())
	for _, d := range calendar.Range(p.From, p.To) {
		for _, sn := range cal.Seasons {
			if !d.Before(sn.OpensOn) && !d.After(sn.ClosesOn) {
				out[d.String()] = [2]bool{true, true}
				break
			}
		}
	}
	for _, c := range cal.Closures {
		k := c.Date.String()
		o, ok := out[k]
		if !ok {
			continue
		}
		for _, s := range c.Slots {
			o[slotIndex(s)] = false
		}
		out[k] = o
	}
	return out
}

// occupancyInput is everything buildOccupancy needs, loaded by the shell.
type occupancyInput struct {
	period Period
	open   map[string][2]bool
	zones  []store.ReportZone
	spots  []store.ReportSpotCount
	allocs []store.ReportUnitCount
	free   []store.ReportUnitCount
}

// occupancyGroups assigns rows to groups and seeds them in display order.
type occupancyGroups struct {
	by    OccupancyGroupBy
	zones map[string]store.ReportZone
	rows  map[string]*OccupancyRow
	order []string
}

func (g *occupancyGroups) key(date string, slot calendar.Slot, zoneID, rowLabel string) string {
	switch g.by {
	case OccupancyByZone:
		return zoneID
	case OccupancyByRow:
		return zoneID + ":" + rowLabel
	case OccupancyBySlot:
		return string(slot)
	default:
		return date
	}
}

// row returns the group of key, creating it (label from the zone) if new.
func (g *occupancyGroups) row(key, zoneID, rowLabel string) *OccupancyRow {
	if r, ok := g.rows[key]; ok {
		return r
	}
	r := &OccupancyRow{Key: key, Label: key}
	switch g.by {
	case OccupancyByZone, OccupancyByRow:
		z := g.zones[zoneID]
		r.ZoneID = zoneID
		r.Label = i18nName(z.Name, z.Code)
		if g.by == OccupancyByRow {
			if rowLabel == "" {
				r.Label += " · Senza fila"
			} else {
				r.Label += " · Fila " + rowLabel
			}
		}
	case OccupancyBySlot:
		r.Label = labelOr(slotLabels, calendar.Slot(key))
	default:
		if d, err := calendar.Parse(key); err == nil {
			r.Label = dayLabel(d)
		}
	}
	g.rows[key] = r
	g.order = append(g.order, key)
	return r
}

// buildOccupancy aggregates the occupancy of in by groupBy. Day groups
// cover every day of the period (closed days with capacity 0), slot groups
// are AM and PM, zone and row groups are those with active spots (plus any
// group that only has allocations), ordered by zone sort order and row.
func buildOccupancy(in occupancyInput, groupBy OccupancyGroupBy) *Occupancy {
	g := &occupancyGroups{by: groupBy, zones: map[string]store.ReportZone{}, rows: map[string]*OccupancyRow{}}
	for _, z := range in.zones {
		g.zones[z.ID] = z
	}
	switch groupBy {
	case OccupancyByZone, OccupancyByRow:
		spots := slices.Clone(in.spots)
		slices.SortFunc(spots, func(a, b store.ReportSpotCount) int {
			za, zb := g.zones[a.ZoneID], g.zones[b.ZoneID]
			return cmp.Or(cmp.Compare(za.SortOrder, zb.SortOrder), cmp.Compare(za.Code, zb.Code),
				cmp.Compare(a.ZoneID, b.ZoneID), compareRowLabels(a.RowLabel, b.RowLabel))
		})
		for _, sp := range spots {
			g.row(g.key("", "", sp.ZoneID, sp.RowLabel), sp.ZoneID, sp.RowLabel)
		}
	case OccupancyBySlot:
		for _, s := range slots {
			g.row(string(s), "", "")
		}
	default:
		for _, d := range calendar.Range(in.period.From, in.period.To) {
			g.row(d.String(), "", "")
		}
	}

	for date, open := range in.open {
		for i, s := range slots {
			if !open[i] {
				continue
			}
			for _, sp := range in.spots {
				g.row(g.key(date, s, sp.ZoneID, sp.RowLabel), sp.ZoneID, sp.RowLabel).CapacityUnits += sp.Spots
			}
		}
	}
	isOpen := func(u store.ReportUnitCount) bool { return in.open[u.Date.String()][slotIndex(u.Slot)] }
	for _, u := range in.allocs {
		if !isOpen(u) {
			continue
		}
		r := g.row(g.key(u.Date.String(), u.Slot, u.ZoneID, u.RowLabel), u.ZoneID, u.RowLabel)
		switch u.Kind {
		case store.AllocationBlock:
			r.BlockedUnits += u.Units
			r.CapacityUnits -= u.Units
		case store.AllocationSeason:
			r.OccupiedUnits += u.Units
			r.SeasonUnits += u.Units
		default:
			r.OccupiedUnits += u.Units
		}
	}
	for _, u := range in.free {
		if isOpen(u) {
			g.row(g.key(u.Date.String(), u.Slot, u.ZoneID, u.RowLabel), u.ZoneID, u.RowLabel).ReleasedUnits += u.Units
		}
	}

	out := &Occupancy{Period: in.period, GroupBy: groupBy, Rows: make([]OccupancyRow, 0, len(g.order)), Totals: OccupancyRow{Key: "total", Label: "Totale"}}
	for _, k := range g.order {
		r := g.rows[k]
		r.CapacityUnits = max(r.CapacityUnits, 0)
		r.OccupancyBP = Ratio(r.OccupiedUnits, r.CapacityUnits)
		out.Rows = append(out.Rows, *r)
		addOccupancy(&out.Totals, r)
	}
	out.Totals.OccupancyBP = Ratio(out.Totals.OccupiedUnits, out.Totals.CapacityUnits)
	return out
}

func addOccupancy(dst, r *OccupancyRow) {
	dst.CapacityUnits += r.CapacityUnits
	dst.OccupiedUnits += r.OccupiedUnits
	dst.SeasonUnits += r.SeasonUnits
	dst.ReleasedUnits += r.ReleasedUnits
	dst.BlockedUnits += r.BlockedUnits
}

// compareRowLabels orders row labels by length then text, so that "B"
// precedes "AA" and "2" precedes "10".
func compareRowLabels(a, b string) int {
	return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b))
}
