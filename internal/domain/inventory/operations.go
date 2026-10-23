package inventory

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Operational views (spec 11.3, 15.1): the state of every spot on one day
// (map "Oggi") and the planning grid spots x days. Both load the range with
// a fixed number of queries and fold rows in memory.

// Operational spot statuses (spec 11.3).
const (
	SpotFree       = "free"
	SpotBooked     = "booked"
	SpotCheckedIn  = "checked_in"
	SpotHeld       = "held"
	SpotBlocked    = "blocked"
	SpotPassHolder = "pass_holder"
	SpotReleased   = "released"
	SpotClosed     = "closed"
	SpotInactive   = "inactive"
)

// OperationsQuery selects a day.
type OperationsQuery struct {
	Date  calendar.Date
	Slot  calendar.Slots // default FULL
	MapID string
}

// OperationalBooking is the booking summary shown on a spot.
type OperationalBooking struct {
	BookingID           string
	Code                string
	Status              string
	Channel             string
	CustomerID          string
	CustomerDisplayName string
	// OccupantLabel is the short name drawn on the map (OccupantLabel func).
	OccupantLabel string
	// CustomerAnonymized: GDPR-anonymised customer, no name to show.
	CustomerAnonymized bool
	CustomerTags       []string
	StartsOn           calendar.Date
	EndsOn             calendar.Date
	Slots              calendar.Slots
	IsSeasonPass       bool
	LineID             string
	PackCodes          []string
	TotalAmount        money.Cents
	PaidAmount         money.Cents
	BalanceDue         money.Cents
	HasExtras          bool
	HasNotes           bool
	// Version is the If-Match version for the actions offered on the map.
	Version int64
}

// OperationalSpot is the state of a spot on the requested slot(s).
type OperationalSpot struct {
	SpotID        string
	Label         string
	MapID         string
	ZoneID        string
	Status        string
	AMStatus      string // set when Slot is FULL
	PMStatus      string
	Booking       *OperationalBooking
	HoldID        string
	HoldExpiresAt *time.Time
	BlockID       string
	BlockReason   string
	ReleaseID     string
}

// PendingAssignment is a zone-mode line still without a spot.
type PendingAssignment struct {
	BookingID           string
	LineID              string
	Code                string
	CustomerDisplayName string
	ZoneID              string
	StartsOn            calendar.Date
	EndsOn              calendar.Date
	Slots               calendar.Slots
}

// OperationsTotals are the counters of the day header.
type OperationsTotals struct {
	Spots              int
	Occupied           int
	Free               int
	Arrivals           int
	Departures         int
	CheckedIn          int
	PendingAssignments int
	BalanceDue         money.Cents
}

// OperationsDay is the answer of GetOperationsDay.
type OperationsDay struct {
	Date               calendar.Date
	Slot               calendar.Slots
	Closed             bool
	Spots              []OperationalSpot
	PendingAssignments []PendingAssignment
	Pools              []PoolAvailability
	Totals             OperationsTotals
}

// DisplayName is the short customer name of the operational views
// ("Rossi M."). It is personal data: admin API only.
func DisplayName(first, last string) string {
	last, first = strings.TrimSpace(last), strings.TrimSpace(first)
	if first == "" {
		return last
	}
	r, _ := utf8.DecodeRuneInString(first)
	return strings.TrimSpace(last + " " + string(r) + ".")
}

// OccupantLabel is the label of the occupant drawn on the operational map
// (spec 11.3): the surname, or the first name when there is no surname;
// empty for anonymised customers (the client shows "Cliente anonimo").
// Personal data: admin API only, never in SSE payloads or logs.
func OccupantLabel(first, last string, anonymized bool) string {
	if anonymized {
		return ""
	}
	if last = strings.TrimSpace(last); last != "" {
		return last
	}
	return strings.TrimSpace(first)
}

// halfDayState is the occupant of one spot on one half-day.
type halfDayState struct {
	status    string
	alloc     *store.SpotAllocation
	releaseID string
}

// GetOperationsDay returns the state of every spot on a day (bookings.read).
func (s *Service) GetOperationsDay(ctx context.Context, p *auth.Principal, establishmentID string, q OperationsQuery) (*OperationsDay, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if q.Slot == "" {
		q.Slot = calendar.SlotsFull
	}
	if q.Date.IsZero() {
		return nil, configerr.Invalid("date", "Data obbligatoria.")
	}
	if !q.Slot.Valid() {
		return nil, configerr.Invalid("slot", "Fascia non valida.")
	}
	out, err := s.operationsDay(ctx, es, q)
	return out, wrap("operations day", err)
}

func (s *Service) operationsDay(ctx context.Context, es store.EstablishmentScope, q OperationsQuery) (*OperationsDay, error) {
	now := s.d.Clock.Now()
	spots, err := store.InventorySpots(ctx, s.d.DB, es, store.InventorySpotFilter{MapID: q.MapID})
	if err != nil {
		return nil, err
	}
	cal, err := s.loadCalendar(ctx, es, q.Date, q.Date)
	if err != nil {
		return nil, err
	}
	allocs, err := store.SpotAllocationsInRange(ctx, s.d.DB, es, nil, q.Date, q.Date, true, now)
	if err != nil {
		return nil, err
	}
	releases, err := store.ActiveSeasonReleasesInRange(ctx, s.d.DB, es, q.Date, q.Date)
	if err != nil {
		return nil, err
	}
	present, err := store.BookingSummariesOn(ctx, s.d.DB, es, q.Date, ActiveStatuses)
	if err != nil {
		return nil, err
	}
	bookings := map[string]*store.BookingSummary{}
	var bookingIDs, blockIDs []string
	for i := range present {
		bookings[present[i].ID] = &present[i]
		bookingIDs = append(bookingIDs, present[i].ID)
	}
	for _, a := range allocs {
		if a.BookingID != nil && bookings[*a.BookingID] == nil {
			bookingIDs = append(bookingIDs, *a.BookingID)
		}
		if a.BlockID != nil {
			blockIDs = append(blockIDs, *a.BlockID)
		}
	}
	if missing := missingIDs(bookingIDs, bookings); len(missing) > 0 {
		extra, err := store.BookingSummariesByID(ctx, s.d.DB, es, missing)
		if err != nil {
			return nil, err
		}
		for i := range extra {
			bookings[extra[i].ID] = &extra[i]
		}
	}
	lines, err := store.LinePacksByBooking(ctx, s.d.DB, es, slices.Compact(slices.Sorted(slices.Values(bookingIDs))))
	if err != nil {
		return nil, err
	}
	packCodes := map[string][]string{}
	for _, l := range lines {
		if !slices.Contains(packCodes[l.BookingID], l.PackCode) {
			packCodes[l.BookingID] = append(packCodes[l.BookingID], l.PackCode)
		}
	}
	blocks, err := store.SpotBlocksByID(ctx, s.d.DB, es, blockIDs)
	if err != nil {
		return nil, err
	}
	blockReason := map[string]string{}
	for _, b := range blocks {
		blockReason[b.ID] = b.Reason
	}
	bySpot := map[string]map[calendar.Slot]*store.SpotAllocation{}
	for i := range allocs {
		a := &allocs[i]
		if bySpot[a.SpotID] == nil {
			bySpot[a.SpotID] = map[calendar.Slot]*store.SpotAllocation{}
		}
		bySpot[a.SpotID][a.Slot] = a
	}
	released := map[string]map[calendar.Slot]string{}
	for _, r := range releases {
		if r.ResoldBookingLineID != nil {
			continue
		}
		if released[r.SpotID] == nil {
			released[r.SpotID] = map[calendar.Slot]string{}
		}
		released[r.SpotID][r.Slot] = r.ID
	}

	slotsWanted := q.Slot.Expand()
	day := &OperationsDay{Date: q.Date, Slot: q.Slot, Spots: make([]OperationalSpot, 0, len(spots))}
	day.Closed = true
	for _, sl := range slotsWanted {
		if !cal.closed[availability.DateSlot{Date: q.Date, Slot: sl}] {
			day.Closed = false
		}
	}
	for i := range spots {
		sp := &spots[i]
		os := OperationalSpot{SpotID: sp.ID, Label: sp.Label, MapID: sp.MapID, ZoneID: sp.ZoneID}
		states := make([]halfDayState, len(slotsWanted))
		for k, sl := range slotsWanted {
			st := halfDayState{status: SpotFree}
			a := bySpot[sp.ID][sl]
			switch {
			case !sp.Active:
				st.status = SpotInactive
			case a != nil:
				st.alloc = a
				st.status = allocationStatus(a, bookings)
			case cal.closed[availability.DateSlot{Date: q.Date, Slot: sl}]:
				st.status = SpotClosed
			case released[sp.ID][sl] != "":
				st.status, st.releaseID = SpotReleased, released[sp.ID][sl]
			}
			states[k] = st
		}
		os.Status = combineStatus(states)
		if len(states) == 2 {
			os.AMStatus, os.PMStatus = states[0].status, states[1].status
		}
		for _, st := range states {
			if st.releaseID != "" && os.ReleaseID == "" {
				os.ReleaseID = st.releaseID
			}
			a := st.alloc
			if a == nil {
				continue
			}
			if a.BookingID != nil && os.Booking == nil {
				if b := bookings[*a.BookingID]; b != nil {
					os.Booking = operationalBooking(b, a, packCodes[b.ID])
				}
			}
			if a.HoldID != nil && os.HoldID == "" {
				os.HoldID, os.HoldExpiresAt = *a.HoldID, a.ExpiresAt
			}
			if a.Kind == store.AllocationHold && a.BookingID != nil && os.HoldExpiresAt == nil {
				os.HoldExpiresAt = a.ExpiresAt
			}
			if a.BlockID != nil && os.BlockID == "" {
				os.BlockID, os.BlockReason = *a.BlockID, blockReason[*a.BlockID]
			}
		}
		if sp.Active {
			day.Totals.Spots++
			if occupied(os.Status) {
				day.Totals.Occupied++
			} else if os.Status == SpotFree || os.Status == SpotReleased {
				day.Totals.Free++
			}
		}
		day.Spots = append(day.Spots, os)
	}

	pending, err := store.PendingLinesInRange(ctx, s.d.DB, es, q.Date, q.Date, ActiveStatuses, nil, nil)
	if err != nil {
		return nil, err
	}
	day.PendingAssignments = make([]PendingAssignment, 0, len(pending))
	for _, l := range pending {
		if !slotsOverlap(l.Slots, q.Slot) {
			continue
		}
		day.PendingAssignments = append(day.PendingAssignments, PendingAssignment{
			BookingID: l.BookingID, LineID: l.LineID, Code: l.Code, CustomerDisplayName: DisplayName(l.FirstName, l.LastName),
			ZoneID: l.ZoneID, StartsOn: l.StartsOn, EndsOn: l.EndsOn, Slots: l.Slots,
		})
	}
	day.Totals.PendingAssignments = len(day.PendingAssignments)
	mv, err := store.DayMovementsOn(ctx, s.d.DB, es, q.Date, ActiveStatuses)
	if err != nil {
		return nil, err
	}
	day.Totals.Arrivals, day.Totals.Departures, day.Totals.CheckedIn = mv.Arrivals, mv.Departures, mv.CheckedIn
	for i := range present {
		day.Totals.BalanceDue += present[i].BalanceDue()
	}

	pools, err := store.InventoryPools(ctx, s.d.DB, es, nil)
	if err != nil {
		return nil, err
	}
	if q.MapID != "" {
		zones, err := store.InventoryZones(ctx, s.d.DB, es, nil)
		if err != nil {
			return nil, err
		}
		onMap := map[string]bool{}
		for _, z := range zones {
			onMap[z.ID] = z.MapID != nil && *z.MapID == q.MapID
		}
		pools = slices.DeleteFunc(pools, func(pl store.Pool) bool { return pl.ZoneID != nil && !onMap[*pl.ZoneID] })
	}
	poolIDs := make([]string, len(pools))
	for i := range pools {
		poolIDs[i] = pools[i].ID
	}
	occ, err := store.PoolOccupancyInRange(ctx, s.d.DB, es, poolIDs, q.Date, q.Date, now, "", "")
	if err != nil {
		return nil, err
	}
	occByKey := map[store.PoolKey]store.PoolOccupancy{}
	for _, o := range occ {
		occByKey[store.PoolKey{PoolID: o.PoolID, Date: o.Date, Slot: o.Slot}] = o
	}
	open := cal.closed.openHalfDays(availability.Interval{From: q.Date, To: q.Date, Slots: q.Slot})
	day.Pools = make([]PoolAvailability, 0, len(pools))
	for i := range pools {
		day.Pools = append(day.Pools, poolAvailability(&pools[i], open, occByKey))
	}
	return day, nil
}

// allocationStatus maps an allocation row to its operational status.
func allocationStatus(a *store.SpotAllocation, bookings map[string]*store.BookingSummary) string {
	switch a.Kind {
	case store.AllocationBlock:
		return SpotBlocked
	case store.AllocationHold:
		return SpotHeld
	case store.AllocationSeason:
		return SpotPassHolder
	}
	if a.BookingID != nil {
		if b := bookings[*a.BookingID]; b != nil && b.Status == "checked_in" {
			return SpotCheckedIn
		}
	}
	return SpotBooked
}

// statusRank orders statuses for the combined status of a full day: the
// most "occupied" half wins.
var statusRank = map[string]int{
	SpotInactive: 9, SpotCheckedIn: 8, SpotBooked: 7, SpotPassHolder: 6, SpotHeld: 5,
	SpotBlocked: 4, SpotReleased: 2, SpotClosed: 1, SpotFree: 0,
}

func combineStatus(states []halfDayState) string {
	best := states[0].status
	for _, st := range states[1:] {
		if statusRank[st.status] > statusRank[best] {
			best = st.status
		}
	}
	return best
}

func occupied(status string) bool {
	switch status {
	case SpotBooked, SpotCheckedIn, SpotHeld, SpotPassHolder, SpotBlocked:
		return true
	}
	return false
}

func slotsOverlap(a, b calendar.Slots) bool {
	for _, x := range a.Expand() {
		if slices.Contains(b.Expand(), x) {
			return true
		}
	}
	return false
}

func operationalBooking(b *store.BookingSummary, a *store.SpotAllocation, packs []string) *OperationalBooking {
	ob := &OperationalBooking{
		BookingID: b.ID, Code: b.Code, Status: b.Status, Channel: b.Channel, CustomerID: b.CustomerID,
		CustomerDisplayName: DisplayName(b.FirstName, b.LastName), CustomerTags: b.Tags,
		OccupantLabel:      OccupantLabel(b.FirstName, b.LastName, b.AnonymizedAt != nil),
		CustomerAnonymized: b.AnonymizedAt != nil,
		StartsOn:           b.StartsOn, EndsOn: b.EndsOn, Slots: b.Slots, IsSeasonPass: b.IsSeasonPass,
		PackCodes: packs, TotalAmount: b.TotalAmount, PaidAmount: b.PaidAmount, BalanceDue: b.BalanceDue(),
		HasExtras: b.HasExtras, HasNotes: b.NotesCustomer != "" || b.NotesInternal != "", Version: b.Version,
	}
	if a.BookingLineID != nil {
		ob.LineID = *a.BookingLineID
	}
	if ob.CustomerTags == nil {
		ob.CustomerTags = []string{}
	}
	if ob.PackCodes == nil {
		ob.PackCodes = []string{}
	}
	return ob
}

func missingIDs(ids []string, have map[string]*store.BookingSummary) []string {
	var out []string
	for _, id := range ids {
		if have[id] == nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// ---- planning --------------------------------------------------------------

// PlanningQuery selects the planning grid.
type PlanningQuery struct {
	From   calendar.Date
	To     calendar.Date
	MapID  string
	ZoneID string
}

// Planning entry kinds.
const (
	EntryBooking = "booking"
	EntrySeason  = "season"
	EntryHold    = "hold"
	EntryBlock   = "block"
	EntryRelease = "release"
)

// PlanningSpot is a row header of the grid.
type PlanningSpot struct {
	SpotID   string
	Label    string
	MapID    string
	ZoneID   string
	RowLabel string
}

// PlanningEntry is an occupant referenced by segments.
type PlanningEntry struct {
	Kind       string
	BookingID  string
	HoldID     string
	BlockID    string
	Code       string
	Status     string
	Channel    string
	Label      string
	BalanceDue *bool
}

// PlanningSegment is a run of consecutive days of one entry on one spot,
// From/To indexes in Days.
type PlanningSegment struct {
	Entry int
	From  int
	To    int
	Slots calendar.Slots
}

// PlanningRow lists the segments of one spot (index in Spots).
type PlanningRow struct {
	Spot     int
	Segments []PlanningSegment
}

// Planning is the compact grid spots x days.
type Planning struct {
	From       calendar.Date
	To         calendar.Date
	Days       []calendar.Date
	ClosedDays []int
	Spots      []PlanningSpot
	Entries    []PlanningEntry
	Rows       []PlanningRow
}

// GetPlanning returns the planning grid (bookings.read, at most
// MaxPlanningDays days).
func (s *Service) GetPlanning(ctx context.Context, p *auth.Principal, establishmentID string, q PlanningQuery) (*Planning, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if err := validateRange(q.From, q.To, MaxPlanningDays); err != nil {
		return nil, err
	}
	out, err := s.planning(ctx, es, q)
	return out, wrap("planning", err)
}

func (s *Service) planning(ctx context.Context, es store.EstablishmentScope, q PlanningQuery) (*Planning, error) {
	now := s.d.Clock.Now()
	f := store.InventorySpotFilter{MapID: q.MapID, ActiveOnly: true}
	if q.ZoneID != "" {
		f.ZoneIDs = []string{q.ZoneID}
	}
	spots, err := store.InventorySpots(ctx, s.d.DB, es, f)
	if err != nil {
		return nil, err
	}
	cal, err := s.loadCalendar(ctx, es, q.From, q.To)
	if err != nil {
		return nil, err
	}
	var spotFilter []string
	if q.MapID != "" || q.ZoneID != "" {
		spotFilter = make([]string, len(spots))
		for i := range spots {
			spotFilter[i] = spots[i].ID
		}
	}
	allocs, err := store.SpotAllocationsInRange(ctx, s.d.DB, es, spotFilter, q.From, q.To, true, now)
	if err != nil {
		return nil, err
	}
	releases, err := store.ActiveSeasonReleasesInRange(ctx, s.d.DB, es, q.From, q.To)
	if err != nil {
		return nil, err
	}
	var bookingIDs, blockIDs []string
	for _, a := range allocs {
		if a.BookingID != nil {
			bookingIDs = append(bookingIDs, *a.BookingID)
		}
		if a.BlockID != nil {
			blockIDs = append(blockIDs, *a.BlockID)
		}
	}
	for _, r := range releases {
		bookingIDs = append(bookingIDs, r.BookingID)
	}
	bookingIDs = slices.Compact(slices.Sorted(slices.Values(bookingIDs)))
	blockIDs = slices.Compact(slices.Sorted(slices.Values(blockIDs)))
	summaries, err := store.BookingSummariesByID(ctx, s.d.DB, es, bookingIDs)
	if err != nil {
		return nil, err
	}
	bookings := make(map[string]*store.BookingSummary, len(summaries))
	for i := range summaries {
		bookings[summaries[i].ID] = &summaries[i]
	}
	blocks, err := store.SpotBlocksByID(ctx, s.d.DB, es, blockIDs)
	if err != nil {
		return nil, err
	}
	blockReason := map[string]string{}
	for _, b := range blocks {
		blockReason[b.ID] = b.Reason
	}

	days := calendar.Range(q.From, q.To)
	pl := &Planning{
		From: q.From, To: q.To, Days: days, ClosedDays: []int{}, Spots: make([]PlanningSpot, len(spots)),
		Entries: []PlanningEntry{}, Rows: []PlanningRow{},
	}
	dayIdx := make(map[calendar.Date]int, len(days))
	for i, d := range days {
		dayIdx[d] = i
		if cal.closed[availability.DateSlot{Date: d, Slot: calendar.AM}] && cal.closed[availability.DateSlot{Date: d, Slot: calendar.PM}] {
			pl.ClosedDays = append(pl.ClosedDays, i)
		}
	}
	spotIdx := make(map[string]int, len(spots))
	for i, sp := range spots {
		spotIdx[sp.ID] = i
		pl.Spots[i] = PlanningSpot{SpotID: sp.ID, Label: sp.Label, MapID: sp.MapID, ZoneID: sp.ZoneID, RowLabel: sp.RowLabel}
	}
	entryIdx := map[string]int{}
	entryOf := func(key string, mk func() PlanningEntry) int {
		if i, ok := entryIdx[key]; ok {
			return i
		}
		entryIdx[key] = len(pl.Entries)
		pl.Entries = append(pl.Entries, mk())
		return entryIdx[key]
	}
	cells := make(map[int][]planCell)
	put := func(si, di int, slot calendar.Slot, e int) {
		row := cells[si]
		if row == nil {
			row = make([]planCell, len(days))
			for k := range row {
				row[k] = planCell{-1, -1}
			}
			cells[si] = row
		}
		if slot == calendar.AM {
			row[di].am = e
		} else {
			row[di].pm = e
		}
	}
	for i := range allocs {
		a := &allocs[i]
		si, ok := spotIdx[a.SpotID]
		if !ok {
			continue
		}
		key, mk := planningEntry(a, bookings, blockReason)
		put(si, dayIdx[a.Date], a.Slot, entryOf(key, mk))
	}
	for _, r := range releases {
		si, ok := spotIdx[r.SpotID]
		if !ok || r.ResoldBookingLineID != nil {
			continue
		}
		di := dayIdx[r.Date]
		if occupiedCell(cells[si], di, r.Slot) {
			continue
		}
		b := bookings[r.BookingID]
		e := entryOf("release:"+r.BookingID, func() PlanningEntry {
			pe := PlanningEntry{Kind: EntryRelease, BookingID: r.BookingID}
			if b != nil {
				pe.Code, pe.Status, pe.Channel, pe.Label = b.Code, b.Status, b.Channel, DisplayName(b.FirstName, b.LastName)
			}
			return pe
		})
		put(si, di, r.Slot, e)
	}
	rowSpots := make([]int, 0, len(cells))
	for si := range cells {
		rowSpots = append(rowSpots, si)
	}
	slices.Sort(rowSpots)
	for _, si := range rowSpots {
		pl.Rows = append(pl.Rows, PlanningRow{Spot: si, Segments: segments(func(di int) (int, int) {
			c := cells[si][di]
			return c.am, c.pm
		}, len(days))})
	}
	return pl, nil
}

// planCell is the entry of one spot on one day per slot (-1 = empty).
type planCell struct{ am, pm int }

// occupiedCell reports whether the slot of day di is taken in row.
func occupiedCell(row []planCell, di int, slot calendar.Slot) bool {
	if di >= len(row) {
		return false
	}
	if slot == calendar.AM {
		return row[di].am >= 0
	}
	return row[di].pm >= 0
}

// planningEntry returns the entry key and constructor of an allocation.
func planningEntry(a *store.SpotAllocation, bookings map[string]*store.BookingSummary, blockReason map[string]string) (string, func() PlanningEntry) {
	switch {
	case a.BlockID != nil:
		id := *a.BlockID
		return "block:" + id, func() PlanningEntry {
			return PlanningEntry{Kind: EntryBlock, BlockID: id, Label: blockReason[id]}
		}
	case a.HoldID != nil:
		id := *a.HoldID
		return "hold:" + id, func() PlanningEntry { return PlanningEntry{Kind: EntryHold, HoldID: id} }
	case a.BookingID != nil:
		id := *a.BookingID
		kind := EntryBooking
		switch a.Kind {
		case store.AllocationSeason:
			kind = EntrySeason
		case store.AllocationHold:
			kind = EntryHold
		}
		return kind + ":" + id, func() PlanningEntry {
			pe := PlanningEntry{Kind: kind, BookingID: id}
			if b := bookings[id]; b != nil {
				due := b.BalanceDue() > 0
				pe.Code, pe.Status, pe.Channel, pe.Label, pe.BalanceDue = b.Code, b.Status, b.Channel, DisplayName(b.FirstName, b.LastName), &due
			}
			return pe
		}
	}
	return "unknown:" + a.ID, func() PlanningEntry { return PlanningEntry{Kind: EntryBooking} }
}

// segments folds the cells of one spot (entry per AM/PM, -1 empty) into
// runs of consecutive days with the same entry and slots. A day whose AM
// and PM belong to the same entry is FULL; otherwise each half is its own
// AM / PM segment. Pure, tested in operations_test.go.
func segments(cellAt func(day int) (am, pm int), days int) []PlanningSegment {
	type key struct {
		entry int
		slots calendar.Slots
	}
	var out []PlanningSegment
	open := map[key]int{} // key -> index in out of the segment ending yesterday
	for di := range days {
		am, pm := cellAt(di)
		var today []key
		switch {
		case am >= 0 && am == pm:
			today = append(today, key{am, calendar.SlotsFull})
		default:
			if am >= 0 {
				today = append(today, key{am, calendar.SlotsAM})
			}
			if pm >= 0 {
				today = append(today, key{pm, calendar.SlotsPM})
			}
		}
		next := map[key]int{}
		for _, k := range today {
			if i, ok := open[k]; ok && out[i].To == di-1 {
				out[i].To = di
				next[k] = i
				continue
			}
			out = append(out, PlanningSegment{Entry: k.entry, From: di, To: di, Slots: k.slots})
			next[k] = len(out) - 1
		}
		open = next
	}
	slices.SortStableFunc(out, func(a, b PlanningSegment) int { return cmp.Compare(a.From, b.From) })
	return out
}
