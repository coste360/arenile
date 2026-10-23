package inventory

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/store"
)

// Automatic assignment of zone-mode lines (spec 7.5). The plan comes from
// the pure availability.AutoAssign over a fresh FreeSpots snapshot; each
// assignment is written with the same unique-index insert as a booking. A
// line whose spot was taken in the meantime (ON CONFLICT refusal) is rolled
// back on its own and re-planned with fresh occupancy, up to assignAttempts
// rounds. Pool capacity is not touched: zone-mode lines consumed the
// implicit zone pool when booked (INV-08).

// assignAttempts bounds the re-planning rounds.
const assignAttempts = 3

// AutoAssignInput selects the pending lines to assign.
type AutoAssignInput struct {
	From    calendar.Date
	To      calendar.Date
	ZoneID  string
	LineIDs []string
	DryRun  bool
	IP      string
}

// AssignedLine is a line placed on a spot (first segment when split).
type AssignedLine struct {
	BookingID string
	LineID    string
	SpotID    string
	SpotLabel string
	Segments  []availability.Segment
}

// UnassignedLine is a line left pending, with the reason of the pure engine
// (or "conflict" when it lost every retry).
type UnassignedLine struct {
	BookingID string
	LineID    string
	Reason    string
}

// AutoAssignResult is the outcome of AutoAssign.
type AutoAssignResult struct {
	Applied    bool
	Assigned   []AssignedLine
	Unassigned []UnassignedLine
}

// AutoAssign assigns the pending lines overlapping [From, To]
// (bookings.write). With DryRun nothing is written.
func (s *Service) AutoAssign(ctx context.Context, p *auth.Principal, establishmentID string, in AutoAssignInput) (*AutoAssignResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	if err := validateRange(in.From, in.To, MaxAvailabilityDays); err != nil {
		return nil, err
	}
	var res *AutoAssignResult
	if in.DryRun {
		res, err = s.assign(ctx, s.d.DB, es, in, true)
		return res, wrap("auto assign", err)
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		res, err = s.assign(ctx, tx, es, in, false)
		if err != nil {
			return err
		}
		if len(res.Assigned) == 0 {
			return nil
		}
		a := &Allocation{}
		var lineIDs []string
		for _, l := range res.Assigned {
			lineIDs = append(lineIDs, l.LineID)
			for _, sg := range l.Segments {
				a.SpotIDs = append(a.SpotIDs, sg.SpotID.String())
				a.Dates = append(a.Dates, calendar.Range(sg.Interval.From, sg.Interval.To)...)
			}
		}
		normalize(a)
		if err := s.record(ctx, tx, p, es, in.IP, ActionLinesAssigned, "booking_line", "", map[string]any{"line_ids": lineIDs}); err != nil {
			return err
		}
		return s.writeEvent(ctx, tx, es, eventAssigned, allocationChange("assigned", a, nil))
	})
	if err != nil {
		return nil, wrap("auto assign", err)
	}
	return res, nil
}

// AssignPendingInTx assigns the given pending lines inside the caller's
// transaction (auto_assign at confirmation, spec 7.5). Lines that cannot be
// placed stay pending; it never fails on a lost race.
func (s *Service) AssignPendingInTx(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineIDs []string, from, to calendar.Date) (*AutoAssignResult, error) {
	if len(lineIDs) == 0 {
		return &AutoAssignResult{Applied: true}, nil
	}
	res, err := s.assign(ctx, tx, es, AutoAssignInput{From: from, To: to, LineIDs: lineIDs}, false)
	return res, wrap("assign pending", err)
}

// assign runs the plan / write / re-plan loop on db.
func (s *Service) assign(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in AutoAssignInput, dryRun bool) (*AutoAssignResult, error) {
	res := &AutoAssignResult{Applied: !dryRun, Assigned: []AssignedLine{}, Unassigned: []UnassignedLine{}}
	var zoneIDs []string
	if in.ZoneID != "" {
		zoneIDs = []string{in.ZoneID}
	}
	todo := in.LineIDs
	for attempt := 0; attempt < assignAttempts; attempt++ {
		lines, err := store.PendingLinesInRange(ctx, db, es, in.From, in.To, ActiveStatuses, zoneIDs, todo)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			break
		}
		input, byLine, labels, err := s.assignInput(ctx, db, es, lines)
		if err != nil {
			return nil, err
		}
		plan := availability.AutoAssign(input)
		var retry []string
		for _, u := range plan.Unassigned {
			l := byLine[u.LineID]
			res.Unassigned = append(res.Unassigned, UnassignedLine{BookingID: l.BookingID, LineID: l.LineID, Reason: string(u.Reason)})
		}
		for _, as := range plan.Assignments {
			l := byLine[as.LineID]
			if !dryRun {
				ok, err := s.applyAssignment(ctx, db, es, l, as)
				if err != nil {
					return nil, err
				}
				if !ok {
					retry = append(retry, l.LineID)
					continue
				}
			}
			spot := as.SpotID().String()
			res.Assigned = append(res.Assigned, AssignedLine{
				BookingID: l.BookingID, LineID: l.LineID, SpotID: spot, SpotLabel: labels[spot], Segments: as.Segments,
			})
		}
		if len(retry) == 0 {
			return res, nil
		}
		todo = retry
		if attempt == assignAttempts-1 {
			for _, id := range retry {
				l := byLine[mustUUID(id)]
				res.Unassigned = append(res.Unassigned, UnassignedLine{BookingID: l.BookingID, LineID: id, Reason: "conflict"})
			}
		}
	}
	return res, nil
}

// assignInput loads spots, occupancy and preferences for the pure engine.
func (s *Service) assignInput(ctx context.Context, db bun.IDB, es store.EstablishmentScope, lines []store.InventoryPendingLine) (availability.AssignInput, map[uuid.UUID]store.InventoryPendingLine, map[string]string, error) {
	now := s.d.Clock.Now()
	from, to := lines[0].StartsOn, lines[0].EndsOn
	var zoneIDs, customers []string
	for _, l := range lines {
		from, to = minDate(from, l.StartsOn), maxDate(to, l.EndsOn)
		zoneIDs = append(zoneIDs, l.ZoneID)
		customers = append(customers, l.CustomerID)
	}
	zoneIDs = slices.Compact(slices.Sorted(slices.Values(zoneIDs)))
	spots, err := store.InventorySpots(ctx, db, es, store.InventorySpotFilter{ZoneIDs: zoneIDs, ActiveOnly: true})
	if err != nil {
		return availability.AssignInput{}, nil, nil, err
	}
	spotIDs := make([]string, len(spots))
	for i := range spots {
		spotIDs[i] = spots[i].ID
	}
	allocs, err := store.SpotAllocationsInRange(ctx, db, es, spotIDs, from, to, true, now)
	if err != nil {
		return availability.AssignInput{}, nil, nil, err
	}
	// Closed half-days never block a placement: every spot counts as free
	// there and no row is written for them.
	all := availability.Interval{From: from, To: to, Slots: calendar.SlotsFull}.DateSlots()
	free, ids := freeSpots(spots, all, allocs)
	prev, err := store.PreviousSpots(ctx, db, es, slices.Compact(slices.Sorted(slices.Values(customers))), from)
	if err != nil {
		return availability.AssignInput{}, nil, nil, err
	}
	labels := make(map[string]string, len(spots))
	in := availability.AssignInput{Free: free, Spots: make([]availability.Spot, 0, len(spots))}
	for _, sp := range spots {
		labels[ids[sp.ID].String()] = sp.Label
		in.Spots = append(in.Spots, availability.Spot{
			ID: ids[sp.ID], ZoneID: mustUUID(sp.ZoneID), Label: sp.Label, Row: sp.RowLabel, Number: sp.Number,
			Kind: sp.Kind, Attributes: sp.Attributes, X: sp.X, Y: sp.Y,
		})
	}
	byLine := make(map[uuid.UUID]store.InventoryPendingLine, len(lines))
	for _, l := range lines {
		id := mustUUID(l.LineID)
		byLine[id] = l
		line := availability.Line{
			ID: id, BookingID: mustUUID(l.BookingID), CustomerID: mustUUID(l.CustomerID), ZoneID: mustUUID(l.ZoneID),
			Interval:  availability.Interval{From: l.StartsOn, To: l.EndsOn, Slots: l.Slots},
			SpotKinds: l.SpotKinds, AllowSpotChange: allowSpotChange(l.PolicySnapshot),
		}
		if ps, ok := prev[l.CustomerID]; ok {
			line.PreviousSpotID = mustUUID(ps)
		}
		in.Lines = append(in.Lines, line)
	}
	return in, byLine, labels, nil
}

// allowSpotChange reads allow_spot_change_during_stay from a policy
// snapshot ({policy_id, rules}); default false.
func allowSpotChange(snapshot store.JSON) bool {
	var doc struct {
		Rules struct {
			Allow *bool `json:"allow_spot_change_during_stay"`
		} `json:"rules"`
	}
	if len(snapshot) == 0 || json.Unmarshal(snapshot, &doc) != nil || doc.Rules.Allow == nil {
		return false
	}
	return *doc.Rules.Allow
}

// applyAssignment writes one assignment: the line is switched to assigned
// only if still pending (conditional update), then its spot rows are
// inserted (closed half-days skipped). A refused row undoes this line's
// rows and reports false (re-plan); other errors abort.
func (s *Service) applyAssignment(ctx context.Context, db bun.IDB, es store.EstablishmentScope, l store.InventoryPendingLine, as availability.Assignment) (bool, error) {
	now := s.d.Clock.Now()
	ok, err := store.AssignLineSpot(ctx, db, es, l.LineID, as.SpotID().String(), now)
	if err != nil || !ok {
		return ok, err
	}
	closures, err := store.ClosureDaysInRange(ctx, db, es, l.StartsOn, l.EndsOn)
	if err != nil {
		return false, err
	}
	closed := newClosedSet(closures)
	kind, expires := assignedKind(l)
	var rows []store.SpotAllocation
	for _, sg := range as.Segments {
		for _, ds := range closed.openHalfDays(sg.Interval) {
			rows = append(rows, store.SpotAllocation{
				ID: store.NewID(), SpotID: sg.SpotID.String(), Date: ds.Date, Slot: ds.Slot, Kind: kind,
				BookingID: ptr(l.BookingID), BookingLineID: ptr(l.LineID), ExpiresAt: expires, CreatedAt: now,
			})
		}
	}
	if _, err := store.DeleteExpiredSpotHolds(ctx, db, es, uniqueStrings(rows, func(r store.SpotAllocation) string { return r.SpotID }), l.StartsOn, l.EndsOn, now); err != nil {
		return false, err
	}
	n, err := store.InsertSpotAllocations(ctx, db, es, rows)
	if err != nil {
		return false, err
	}
	if n == len(rows) {
		return true, nil
	}
	// Lost a race: undo this line only (its pending status and rows).
	if _, err := store.DeleteSpotAllocations(ctx, db, es, store.AllocationOwner{BookingID: l.BookingID, LineIDs: []string{l.LineID}}); err != nil {
		return false, err
	}
	if err := store.UnassignLineSpot(ctx, db, es, l.LineID, now); err != nil {
		return false, err
	}
	return false, nil
}

// assignedKind is the allocation kind of a line assigned now: hold (with
// the booking's expiry) while the booking holds resources temporarily,
// season for passes, booking otherwise (INV-20).
func assignedKind(l store.InventoryPendingLine) (string, *time.Time) {
	switch l.Status {
	case "held", "pending_payment", "awaiting_approval":
		if l.HoldExpiresAt != nil {
			return store.AllocationHold, ptr(l.HoldExpiresAt.UTC())
		}
	}
	if l.IsSeasonPass {
		return store.AllocationSeason, nil
	}
	return store.AllocationBooking, nil
}

func mustUUID(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(s))
	}
	return u
}
