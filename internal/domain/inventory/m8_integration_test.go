//go:build integration

package inventory_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// opsKey is an API key of the establishment with operations.run only: it
// may read the spot calendar but never the occupants' names.
func (f *fixture) opsKey() *auth.Principal {
	return &auth.Principal{
		Kind: auth.PrincipalAPIKey, APIKeyID: store.NewID(), APIKeyEstablishmentID: f.est.ID,
		APIKeyOrganizationID: f.est.OrganizationID, Scopes: []auth.Permission{auth.PermOperationsRun},
	}
}

// setPII gives the fixture customer an email and a phone that must never
// appear in the M8 payloads.
func (f *fixture) setPII(t *testing.T) (email, phone string) {
	t.Helper()
	email, phone = "mario.rossi@example.it", "+393331112233"
	_, err := f.db.NewUpdate().Table("customers").Set("email = ?", email).Set("phone = ?", phone).Where("id = ?", f.customer.ID).Exec(context.Background())
	must(t, err)
	return email, phone
}

// TestSpotCalendar_States (spec 30.5): every half-day of the season of a
// spot with its state (free, booked, season pass, released, held, blocked,
// closed), the bookings with balance due, occupant labels only with
// bookings.read, no contact data, never across tenants.
func TestSpotCalendar_States(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		email, phone := f.setPII(t)
		a1 := f.spots["A1"]
		booked, err := f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-03", lines: []lineSpec{{zone: f.seat, spot: a1}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-05", to: "2027-07-05", slots: calendar.SlotsAM, expires: ptr(t0.Add(time.Hour)), status: "held", lines: []lineSpec{{zone: f.seat, spot: a1}}}, nil)
		must(t, err)
		season, err := f.book(t, bookingSpec{from: "2027-07-10", to: "2027-07-12", season: true, lines: []lineSpec{{zone: f.seat, spot: a1}}}, nil)
		must(t, err)
		// 2027-07-11 AM released by the holder: no season row, a release row.
		_, err = db.NewDelete().Model((*store.SpotAllocation)(nil)).Where("booking_line_id = ? AND date = ? AND slot = 'AM'", season.lineIDs[0], d("2027-07-11")).Exec(ctx)
		must(t, err)
		must(t, store.InsertSeasonReleases(ctx, db, f.es, []store.SeasonRelease{{
			ID: store.NewID(), EstablishmentID: f.est.ID, BookingID: season.id, BookingLineID: season.lineIDs[0], SpotID: a1.ID,
			Date: d("2027-07-11"), Slot: calendar.AM, CreatedAt: t0, UpdatedAt: t0,
		}}))
		_, err = f.svc.CreateSpotBlocks(ctx, f.owner, f.est.ID, inventory.SpotBlockInput{SpotIDs: []string{a1.ID}, StartsOn: d("2027-07-15"), EndsOn: d("2027-07-15"), Slots: calendar.SlotsFull, Reason: "manutenzione"})
		must(t, err)

		cal, err := f.svc.SpotCalendar(ctx, f.viewer, f.est.ID, a1.ID, "")
		must(t, err)
		if cal.From.String() != "2027-05-15" || cal.To.String() != "2027-09-30" || len(cal.Days) != 139 || cal.Label != "A1" {
			t.Fatalf("calendar bounds %s..%s days %d", cal.From, cal.To, len(cal.Days))
		}
		day := func(s string) inventory.CalendarDay {
			for _, x := range cal.Days {
				if x.Date.String() == s {
					return x
				}
			}
			t.Fatalf("no day %s", s)
			return inventory.CalendarDay{}
		}
		for _, tc := range []struct {
			date   string
			am, pm string
			amBook string
		}{
			{"2027-07-01", inventory.CalendarFree, inventory.CalendarFree, ""},
			{"2027-07-02", inventory.CalendarBooked, inventory.CalendarBooked, booked.id},
			{"2027-07-05", inventory.CalendarHeld, inventory.CalendarFree, ""},
			{"2027-07-10", inventory.CalendarSeasonPass, inventory.CalendarSeasonPass, season.id},
			{"2027-07-11", inventory.CalendarReleased, inventory.CalendarSeasonPass, ""},
			{"2027-07-15", inventory.CalendarBlocked, inventory.CalendarBlocked, ""},
			{"2027-07-20", inventory.CalendarFree, inventory.CalendarClosed, ""},
		} {
			got := day(tc.date)
			if got.AM.Status != tc.am || got.PM.Status != tc.pm || (tc.amBook != "" && got.AM.BookingID != tc.amBook) {
				t.Fatalf("%s = %+v / %+v, want %s / %s", tc.date, got.AM, got.PM, tc.am, tc.pm)
			}
		}
		if len(cal.Bookings) != 3 {
			t.Fatalf("bookings %+v", cal.Bookings)
		}
		for _, b := range cal.Bookings {
			if b.OccupantLabel != "Rossi" || b.BalanceDue <= 0 && b.Status != "held" {
				t.Fatalf("booking %+v", b)
			}
		}

		// operations.run without bookings.read: states, no names.
		noNames, err := f.svc.SpotCalendar(ctx, f.opsKey(), f.est.ID, a1.ID, "")
		must(t, err)
		for _, b := range noNames.Bookings {
			if b.OccupantLabel != "" {
				t.Fatalf("label without bookings.read: %+v", b)
			}
		}
		// An unknown season or spot, another tenant: not found.
		if _, err := f.svc.SpotCalendar(ctx, f.viewer, f.est.ID, a1.ID, store.NewID()); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("unknown season: %v", err)
		}
		if _, err := f.svc.SpotCalendar(ctx, f.viewer, f.est.ID, store.NewID(), ""); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("unknown spot: %v", err)
		}
		if _, err := f.svc.SpotCalendar(ctx, f.ownerB, f.est.ID, a1.ID, ""); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		reports := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: store.NewID(), APIKeyEstablishmentID: f.est.ID, APIKeyOrganizationID: f.est.OrganizationID, Scopes: []auth.Permission{auth.PermReportsRead}}
		if _, err := f.svc.SpotCalendar(ctx, reports, f.est.ID, a1.ID, ""); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("reports-only key: %v", err)
		}

		// HTTP: the label with bookings.read, never email or phone; none
		// at all for the operations key.
		h := &admin.Handlers{Inventory: f.svc, Log: slog.New(slog.DiscardHandler)}
		get := func(p *auth.Principal) string {
			r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, p))
			w := httptest.NewRecorder()
			h.GetSpotCalendar(w, r, f.est.ID, a1.ID, api.GetSpotCalendarParams{})
			if w.Code != http.StatusOK {
				t.Fatalf("http: %d %s", w.Code, w.Body)
			}
			return w.Body.String()
		}
		body := get(f.viewer)
		if strings.Contains(body, email) || strings.Contains(body, phone) || !strings.Contains(body, `"occupant_label":"Rossi"`) {
			t.Fatalf("viewer payload: %s", body[:min(len(body), 400)])
		}
		var sc api.SpotCalendar
		must(t, json.Unmarshal([]byte(body), &sc))
		if len(sc.Days) != 139 || sc.Days[0].Am.Status != api.SpotCalendarStatusFree {
			t.Fatalf("api days %d", len(sc.Days))
		}
		if body := get(f.opsKey()); strings.Contains(body, "occupant_label") || strings.Contains(body, "Rossi") {
			t.Fatalf("operations key payload carries names")
		}
	})
}

// TestOperationsDay_M8Marks (spec 30.4): arrival and departure flags,
// setup from the extras shown on the map, agreement and partner badges,
// group and party size, the segment of a split line; the lifeguard list
// built from them (json and printable html without contact data).
func TestOperationsDay_M8Marks(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		email, phone := f.setPII(t)
		extra := func(code, label string, show bool, attach string) *store.Extra {
			x := &store.Extra{
				ID: store.NewID(), EstablishmentID: f.est.ID, Code: code, Name: store.I18nText{"it": code}, Description: store.I18nText{},
				PricingUnit: "per_day", MaxQuantity: 5, AttachTo: attach, Channels: []string{"desk"}, Active: true, Version: 1,
				CreatedAt: t0, UpdatedAt: t0, ShowOnMap: show, MapLabel: label,
			}
			must(t, store.InsertExtra(ctx, db, f.es, x))
			return x
		}
		sunbed, chair := extra("lettino", "L", true, "line"), extra("sdraio", "S", true, "booking")
		towel := extra("telo", "", false, "line")
		partner := &store.Partner{ID: store.NewID(), EstablishmentID: f.est.ID, Name: "Hotel Miramare", Kind: "struttura_ricettiva", Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		_, err := db.NewInsert().Model(partner).Exec(ctx)
		must(t, err)
		ag := &store.Agreement{
			ID: store.NewID(), EstablishmentID: f.est.ID, Code: ptr("MIRA"), Name: "Ospiti Miramare", Action: store.JSON(`{"type":"percent","value":10}`),
			Conditions: store.JSON(`{}`), Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0, PartnerID: &partner.ID, Requirement: "conferma hotel",
		}
		_, err = db.NewInsert().Model(ag).Exec(ctx)
		must(t, err)

		// A1: arrives today, two sunbeds and a chair, agreement + partner,
		// group of 3; A3: last day of a stay, no badge.
		b1, err := f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-04", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, nil)
		must(t, err)
		_, err = db.NewUpdate().Table("bookings").Set("agreement_id = ?", ag.ID).Set("partner_id = ?", partner.ID).
			Set("group_name = ?", "Famiglia Rossi").Set("party_adults = 2").Set("party_children = 1").Where("id = ?", b1.id).Exec(ctx)
		must(t, err)
		must(t, store.InsertBookingLineExtras(ctx, db, f.es, []store.BookingLineExtra{
			{ID: store.NewID(), BookingID: b1.id, BookingLineID: &b1.lineIDs[0], ExtraID: sunbed.ID, Quantity: 2, Total: 1000, CreatedAt: t0, UpdatedAt: t0},
			{ID: store.NewID(), BookingID: b1.id, ExtraID: chair.ID, Quantity: 1, Total: 500, CreatedAt: t0, UpdatedAt: t0},
			{ID: store.NewID(), BookingID: b1.id, BookingLineID: &b1.lineIDs[0], ExtraID: towel.ID, Quantity: 1, Total: 200, CreatedAt: t0, UpdatedAt: t0},
		}))
		_, err = f.book(t, bookingSpec{from: "2027-06-28", to: "2027-07-02", lines: []lineSpec{{zone: f.seat, spot: f.spots["A3"]}}}, nil)
		must(t, err)
		// A4 then A5: a split line (A4 on 07-01..02, A5 on 07-03).
		split := bookingSpec{from: "2027-07-01", to: "2027-07-03", lines: []lineSpec{{zone: f.seat, spot: f.spots["A4"]}}}
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			tb := f.insertBooking(t, tx, split)
			first := f.allocRequest(bookingSpec{from: "2027-07-01", to: "2027-07-02", lines: split.lines}, tb, nil)
			if _, err := f.svc.Allocate(ctx, tx, f.es, first); err != nil {
				return err
			}
			second := f.allocRequest(bookingSpec{from: "2027-07-03", to: "2027-07-03", lines: []lineSpec{{zone: f.seat, spot: f.spots["A5"]}}}, tb, nil)
			_, err := f.svc.Allocate(ctx, tx, f.es, second)
			return err
		})
		must(t, err)

		day, err := f.svc.GetOperationsDay(ctx, f.viewer, f.est.ID, inventory.OperationsQuery{Date: d("2027-07-02"), MapID: f.mapID})
		must(t, err)
		by := map[string]*inventory.OperationalBooking{}
		for _, sp := range day.Spots {
			by[sp.Label] = sp.Booking
		}
		a1 := by["A1"]
		if a1 == nil || !a1.IsFirstDay || a1.IsLastDay || a1.SetupLabel != "2L 1S" || len(a1.Setup) != 2 {
			t.Fatalf("A1 marks %+v", a1)
		}
		if a1.Agreement == nil || a1.Agreement.Name != "Ospiti Miramare" || a1.Agreement.PartnerName != "Hotel Miramare" || a1.Agreement.Code != "MIRA" {
			t.Fatalf("A1 agreement %+v", a1.Agreement)
		}
		if a1.Partner == nil || a1.Partner.Kind != "struttura_ricettiva" || a1.GroupName != "Famiglia Rossi" || a1.PartySize == nil || *a1.PartySize != 3 {
			t.Fatalf("A1 partner/group %+v %v", a1.Partner, a1.PartySize)
		}
		if a1.SegmentStartsOn != nil {
			t.Fatalf("A1 is not split: %v", a1.SegmentStartsOn)
		}
		if a3 := by["A3"]; a3 == nil || a3.IsFirstDay || !a3.IsLastDay || a3.SetupLabel != "" || a3.Agreement != nil || a3.PartySize != nil {
			t.Fatalf("A3 marks %+v", a3)
		}
		if a4 := by["A4"]; a4 == nil || a4.SegmentStartsOn == nil || a4.SegmentStartsOn.String() != "2027-07-01" || a4.SegmentEndsOn.String() != "2027-07-02" || a4.EndsOn.String() != "2027-07-03" {
			t.Fatalf("A4 segment %+v", a4)
		}

		// HTTP payload of the day: the new fields, never contact data.
		h := &admin.Handlers{Inventory: f.svc, Log: slog.New(slog.DiscardHandler)}
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, f.viewer))
		w := httptest.NewRecorder()
		h.GetOperationsDay(w, r, f.est.ID, api.GetOperationsDayParams{Date: d("2027-07-02"), MapId: &f.mapID})
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), email) || strings.Contains(w.Body.String(), phone) {
			t.Fatalf("http day: %d", w.Code)
		}
		var od api.OperationsDay
		must(t, json.Unmarshal(w.Body.Bytes(), &od))
		for _, sp := range od.Spots {
			if sp.Label == "A1" {
				b := sp.Booking
				if b.SetupLabel == nil || *b.SetupLabel != "2L 1S" || b.IsFirstDay == nil || !*b.IsFirstDay || b.Partner == nil || b.Agreement == nil || *b.PartySize != 3 {
					t.Fatalf("http A1 %+v", b)
				}
			}
		}

		// Lifeguard list: occupied spots in map order, json and html.
		lg, err := f.svc.LifeguardList(ctx, f.cashier, f.est.ID, d("2027-07-02"), "", f.mapID)
		must(t, err)
		labels := make([]string, len(lg.Rows))
		for i, row := range lg.Rows {
			labels[i] = row.SpotLabel
		}
		if !slices.Equal(labels, []string{"A1", "A3", "A4"}) || !lg.Rows[0].Arriving || lg.Rows[0].SetupLabel != "2L 1S" || !lg.Rows[1].Departing || lg.Rows[0].RowLabel != "A" {
			t.Fatalf("lifeguard rows %+v", lg.Rows)
		}
		for _, format := range []string{"json", "html"} {
			r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, f.viewer))
			w := httptest.NewRecorder()
			h.GetLifeguardList(w, r, f.est.ID, api.GetLifeguardListParams{Date: d("2027-07-02"), MapId: &f.mapID, Format: &format})
			body := w.Body.String()
			if w.Code != http.StatusOK || strings.Contains(body, email) || strings.Contains(body, phone) || !strings.Contains(body, "Rossi") {
				t.Fatalf("lifeguard %s: %d", format, w.Code)
			}
			if format == "html" && (!strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(body, "2L 1S") || strings.Contains(body, "<script")) {
				t.Fatalf("lifeguard html: %s", w.Header())
			}
		}
		reports := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: store.NewID(), APIKeyEstablishmentID: f.est.ID, APIKeyOrganizationID: f.est.OrganizationID, Scopes: []auth.Permission{auth.PermReportsRead}}
		if _, err := f.svc.LifeguardList(ctx, reports, f.est.ID, d("2027-07-02"), "", ""); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("reports-only key: %v", err)
		}
	})
}

// TestAvailability_PartiallyFree (spec 30.4 range view): each spot reports
// the dates of the range on which it is free for every slot and the open
// dates (closures excluded), "N giorni su M".
func TestAvailability_PartiallyFree(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, err := f.book(t, bookingSpec{from: "2027-07-18", to: "2027-07-19", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-21", to: "2027-07-21", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, nil)
		must(t, err)
		res, err := f.svc.GetAvailability(ctx, f.viewer, f.est.ID, inventory.AvailabilityQuery{From: d("2027-07-17"), To: d("2027-07-22"), ZoneID: f.seat.ID})
		must(t, err)
		var a1, a2 *inventory.SpotAvailability
		for i := range res.Zones[0].Spots {
			switch sp := &res.Zones[0].Spots[i]; sp.Label {
			case "A1":
				a1 = sp
			case "A2":
				a2 = sp
			}
		}
		// 07-20 PM is a closure: the date stays open (AM) and counts.
		dates := func(ds []calendar.Date) string {
			out := make([]string, len(ds))
			for i, x := range ds {
				out[i] = x.String()[5:]
			}
			return strings.Join(out, ",")
		}
		if a1.Available || a1.TotalDays != 6 || dates(a1.FreeDates) != "07-17,07-20,07-22" {
			t.Fatalf("A1 %v %d %s", a1.Available, a1.TotalDays, dates(a1.FreeDates))
		}
		if !a2.Available || a2.TotalDays != 6 || len(a2.FreeDates) != 6 {
			t.Fatalf("A2 %+v", a2)
		}
		h := &admin.Handlers{Inventory: f.svc, Log: slog.New(slog.DiscardHandler)}
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, f.viewer))
		w := httptest.NewRecorder()
		h.GetAvailability(w, r, f.est.ID, api.GetAvailabilityParams{From: d("2027-07-17"), To: d("2027-07-22"), ZoneId: &f.seat.ID})
		var out api.AvailabilityResult
		must(t, json.Unmarshal(w.Body.Bytes(), &out))
		for _, sp := range *out.Zones[0].Spots {
			if sp.Label == "A1" && (*sp.FreeDays != 3 || *sp.TotalDays != 6 || len(*sp.FreeDates) != 3) {
				t.Fatalf("http A1 %+v", sp)
			}
		}
	})
}

// TestOnlineBookable_Spot (spec 30.11): a spot excluded from online sale
// never appears in the online availability nor counts in it, and an online
// allocation on it is refused like a taken spot; the desk sees and sells it.
func TestOnlineBookable_Spot(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		a2 := f.spots["A2"]
		_, err := db.NewUpdate().Table("spots").Set("online_excluded = ?", true).Where("id = ?", a2.ID).Exec(ctx)
		must(t, err)
		q := inventory.AvailabilityQuery{From: d("2027-07-02"), To: d("2027-07-02"), ZoneID: f.seat.ID}
		pub, err := f.svc.PublicAvailability(ctx, f.es, q)
		must(t, err)
		for _, sp := range pub.Zones[0].Spots {
			if sp.SpotID == a2.ID {
				t.Fatal("excluded spot in the online availability")
			}
		}
		if n := len(pub.Zones[0].Spots); n != 4 {
			t.Fatalf("online spots %d", n)
		}
		desk, err := f.svc.GetAvailability(ctx, f.viewer, f.est.ID, q)
		must(t, err)
		if n := len(desk.Zones[0].Spots); n != 5 {
			t.Fatalf("desk spots %d", n)
		}
		_, err = f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-02", to: "2027-07-02", lines: []lineSpec{{zone: f.seat, spot: a2}}}, nil)
		var ce *availability.ConflictError
		if !errors.As(err, &ce) || ce.Conflicts[0].SpotID != a2.ID || ce.Conflicts[0].Reason != availability.ReasonSpotUnavailable {
			t.Fatalf("online sale of an excluded spot: %v", err)
		}
		if _, err := f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-02", to: "2027-07-02", lines: []lineSpec{{zone: f.seat, spot: f.spots["A3"]}}}, nil); err != nil {
			t.Fatalf("online sale of a sellable spot: %v", err)
		}
		if _, err := f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-02", lines: []lineSpec{{zone: f.seat, spot: a2}}}, nil); err != nil {
			t.Fatalf("desk sale of an excluded spot: %v", err)
		}
		// The api channel follows the online rules (as the online quotas,
		// INV-09, INV-62).
		if _, err := f.book(t, bookingSpec{channel: store.ChannelAPI, from: "2027-07-03", to: "2027-07-03", lines: []lineSpec{{zone: f.seat, spot: a2}}}, nil); !errors.As(err, &ce) {
			t.Fatalf("api sale of an excluded spot: %v", err)
		}
		// A zone-mode spot is excluded the same way: an online line placed
		// on it (assignment at confirmation, auto-assign) is refused...
		z1 := f.spots["Z1"]
		_, err = db.NewUpdate().Table("spots").Set("online_excluded = ?", true).Where("id = ?", z1.ID).Exec(ctx)
		must(t, err)
		if _, err := f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-03", to: "2027-07-03", lines: []lineSpec{{zone: f.zoneMode, spot: z1}}}, nil); !errors.As(err, &ce) || ce.Conflicts[0].SpotID != z1.ID {
			t.Fatalf("online line on an excluded zone-mode spot: %v", err)
		}
		// ...while the cash desk may move an online booking onto it ("la
		// cassa le vede tutte", spec 30.11).
		if _, err := f.book(t, bookingSpec{channel: store.ChannelOnline, byDesk: true, from: "2027-07-03", to: "2027-07-03", lines: []lineSpec{{zone: f.zoneMode, spot: z1}}}, nil); err != nil {
			t.Fatalf("desk move of an online booking onto an excluded spot: %v", err)
		}
		if _, err := f.book(t, bookingSpec{channel: store.ChannelOnline, byDesk: true, from: "2027-07-04", to: "2027-07-04", lines: []lineSpec{{zone: f.seat, spot: a2}}}, nil); err != nil {
			t.Fatalf("desk move of an online booking onto an excluded seat: %v", err)
		}
		f.checkInvariants(t)
	})
}
