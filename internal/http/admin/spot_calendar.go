package admin

import (
	"bytes"
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/printout"
)

// Operational map additions (spec 30.4, 30.5, 30.10, ADR 0028).
//
//	GetSpotCalendar  -> Inventory.SpotCalendar  -> api.SpotCalendar
//	GetLifeguardList -> Inventory.LifeguardList -> api.LifeguardList (json | html)

// GetSpotCalendar implements GET /spots/{spotId}/calendar.
func (h *Handlers) GetSpotCalendar(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, spotID api.SpotID, params api.GetSpotCalendarParams) {
	c, err := h.Inventory.SpotCalendar(r.Context(), principalOf(r), establishmentID, spotID, invDeref(params.SeasonId))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, spotCalendarToAPI(c))
}

func spotCalendarToAPI(c *inventory.SpotCalendar) api.SpotCalendar {
	out := api.SpotCalendar{
		SpotId: c.SpotID, Label: c.Label, SeasonId: c.SeasonID, From: c.From, To: c.To,
		Days: make([]api.SpotCalendarDay, len(c.Days)), Bookings: make([]api.SpotCalendarBooking, len(c.Bookings)),
	}
	cell := func(x inventory.CalendarCell) api.SpotCalendarCell {
		return api.SpotCalendarCell{Status: api.SpotCalendarStatus(x.Status), BookingId: invOpt(x.BookingID)}
	}
	for i, d := range c.Days {
		out.Days[i] = api.SpotCalendarDay{Date: d.Date, Am: cell(d.AM), Pm: cell(d.PM)}
	}
	for i, b := range c.Bookings {
		out.Bookings[i] = api.SpotCalendarBooking{
			BookingId: b.BookingID, Code: b.Code, Status: api.BookingStatus(b.Status), OccupantLabel: invOpt(b.OccupantLabel),
			StartsOn: b.StartsOn, EndsOn: b.EndsOn, IsSeasonPass: b.IsSeasonPass, BalanceDue: int64(b.BalanceDue),
		}
	}
	return out
}

// GetLifeguardList implements GET /operations/lifeguard-list.
func (h *Handlers) GetLifeguardList(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetLifeguardListParams) {
	format := "json"
	if params.Format != nil && *params.Format != "" {
		format = *params.Format
	}
	if format != "json" && format != "html" {
		problem.Error(w, r, h.Log, configerr.Invalid("format", "Valori ammessi: json, html."))
		return
	}
	var slots calendar.Slots
	if params.Slot != nil {
		slots = calendar.Slots(*params.Slot)
	}
	p := principalOf(r)
	lg, err := h.Inventory.LifeguardList(r.Context(), p, establishmentID, params.Date, slots, invDeref(params.MapId))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if format == "json" {
		writeJSON(w, http.StatusOK, lifeguardToAPI(lg))
		return
	}
	in := printout.LifeguardList{
		Locale: lg.Locale, Establishment: lg.EstablishmentName, Date: lg.Date.String(), Slot: string(lg.Slot),
		Occupied: lg.Totals.Occupied, Free: lg.Totals.Free, Arrivals: lg.Totals.Arrivals, Departures: lg.Totals.Departures,
	}
	if p != nil && p.Locale != "" {
		in.Locale = p.Locale
	}
	for _, row := range lg.Rows {
		in.Rows = append(in.Rows, printout.LifeguardRow{
			Row: row.RowLabel, Spot: row.SpotLabel, Occupant: row.OccupantLabel, Setup: row.SetupLabel, Group: row.GroupName,
			Arriving: row.Arriving, Departing: row.Departing, BalanceDue: row.BalanceDue,
		})
	}
	var buf bytes.Buffer
	if err := printout.RenderLifeguardList(&buf, in); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Security-Policy", printout.ContentSecurityPolicy)
	hd.Set("Cache-Control", "no-store")
	hd.Set("Content-Disposition", `inline; filename="bagnini-`+lg.Date.String()+`.html"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func lifeguardToAPI(lg *inventory.Lifeguard) api.LifeguardList {
	out := api.LifeguardList{
		Date: lg.Date, Slot: api.Slots(lg.Slot), Rows: make([]api.LifeguardRow, len(lg.Rows)),
		Totals: api.OperationsTotals{
			Spots: lg.Totals.Spots, Occupied: lg.Totals.Occupied, Free: lg.Totals.Free, Arrivals: lg.Totals.Arrivals,
			Departures: lg.Totals.Departures, CheckedIn: lg.Totals.CheckedIn, PendingAssignments: lg.Totals.PendingAssignments,
			BalanceDueAmount: int64(lg.Totals.BalanceDue),
		},
	}
	for i, row := range lg.Rows {
		out.Rows[i] = api.LifeguardRow{
			RowLabel: row.RowLabel, SpotId: row.SpotID, SpotLabel: row.SpotLabel, OccupantLabel: row.OccupantLabel,
			SetupLabel: invOpt(row.SetupLabel), GroupName: invOpt(row.GroupName),
			Arriving: row.Arriving, Departing: row.Departing, BalanceDue: row.BalanceDue,
		}
	}
	return out
}

// operationalMarksToAPI copies the M8 fields of an operational booking
// (spec 30.4) into its API form.
func operationalMarksToAPI(m *inventory.OperationalMarks, ob *api.OperationalBooking) {
	first, last := m.IsFirstDay, m.IsLastDay
	ob.IsFirstDay, ob.IsLastDay = &first, &last
	ob.SegmentStartsOn, ob.SegmentEndsOn = m.SegmentStartsOn, m.SegmentEndsOn
	if len(m.Setup) > 0 {
		items := make([]api.SetupItem, len(m.Setup))
		for i, it := range m.Setup {
			items[i] = api.SetupItem{ExtraId: it.ExtraID, MapLabel: it.MapLabel, Quantity: it.Quantity}
		}
		ob.Setup = &items
	}
	ob.SetupLabel = invOpt(m.SetupLabel)
	if a := m.Agreement; a != nil {
		ob.Agreement = &api.AgreementRef{
			Id: a.ID, Name: a.Name, Code: invOpt(a.Code), PartnerId: invOpt(a.PartnerID),
			PartnerName: invOpt(a.PartnerName), Requirement: invOpt(a.Requirement),
		}
	}
	if pr := m.Partner; pr != nil {
		ob.Partner = &api.PartnerRef{Id: pr.ID, Name: pr.Name, Kind: api.PartnerKind(pr.Kind)}
	}
	ob.GroupName = invOpt(m.GroupName)
	ob.PartySize = m.PartySize
}

// spotAvailabilityDays fills the range view counts of a spot (spec 30.4).
func spotAvailabilityDays(sp *inventory.SpotAvailability, out *api.SpotAvailability) {
	free, total := len(sp.FreeDates), sp.TotalDays
	dates := sp.FreeDates
	if dates == nil {
		dates = []calendar.Date{}
	}
	out.FreeDays, out.TotalDays, out.FreeDates = &free, &total, &dates
}
