package inventory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Availability query of the panel (spec 7.8): per spot for seat and zone
// zones, minimum quantity for zone and quantity zones, per pool and
// half-day. Data is loaded with a handful of range queries (zones, spots,
// pools, closures, seasons, valid allocations, pool occupancy, online
// counts) and aggregated in memory with the pure availability package.

// Range limits of the queries.
const (
	MaxAvailabilityDays = 200
	MaxPlanningDays     = 62
)

// ActiveStatuses are the booking statuses that own allocations (INV-20).
var ActiveStatuses = []string{"held", "pending_payment", "awaiting_approval", "confirmed", "checked_in"}

// AvailabilityQuery selects the availability to compute.
type AvailabilityQuery struct {
	From    calendar.Date
	To      calendar.Date
	Slots   calendar.Slots // default FULL
	Channel string         // default desk; online/api apply quotas
	MapID   string
	ZoneID  string
	PackID  string
}

// SpotAvailability is the state of one spot over the interval.
type SpotAvailability struct {
	SpotID    string
	Label     string
	Available bool
	Conflicts []availability.Conflict
}

// ZoneAvailability aggregates one zone.
type ZoneAvailability struct {
	ZoneID    string
	Code      string
	Mode      string
	Capacity  int
	Available int
	Spots     []SpotAvailability // seat and zone zones only
}

// PoolDay is one counter of a pool.
type PoolDay struct {
	Date      calendar.Date
	Slot      calendar.Slot
	Capacity  int
	Used      int
	Held      int
	Available int
}

// PoolAvailability aggregates one pool.
type PoolAvailability struct {
	PoolID        string
	ZoneID        *string
	ComponentCode string
	Capacity      int // minimum effective capacity over the open half-days
	Available     int // minimum over the open half-days
	Days          []PoolDay
}

// AvailabilityResult is the answer of GetAvailability.
type AvailabilityResult struct {
	From        calendar.Date
	To          calendar.Date
	Slots       calendar.Slots
	Channel     string
	ClosedDates []calendar.Date
	Zones       []ZoneAvailability
	Pools       []PoolAvailability
}

// calendarState is the closed half-days of a range: closure days plus the
// dates outside every season.
type calendarState struct {
	closed closedSet
}

func (s *Service) loadCalendar(ctx context.Context, es store.EstablishmentScope, from, to calendar.Date) (calendarState, error) {
	closures, err := store.ClosureDaysInRange(ctx, s.d.DB, es, from, to)
	if err != nil {
		return calendarState{}, err
	}
	seasons, err := store.SeasonsOverlapping(ctx, s.d.DB, es, from, to)
	if err != nil {
		return calendarState{}, err
	}
	cs := calendarState{closed: newClosedSet(closures)}
	for _, d := range calendar.Range(from, to) {
		in := false
		for _, sn := range seasons {
			if !d.Before(sn.OpensOn) && !d.After(sn.ClosesOn) {
				in = true
				break
			}
		}
		if !in {
			cs.closed[availability.DateSlot{Date: d, Slot: calendar.AM}] = true
			cs.closed[availability.DateSlot{Date: d, Slot: calendar.PM}] = true
		}
	}
	return cs, nil
}

// closedDates lists the dates whose requested slots are all closed.
func (c calendarState) closedDates(from, to calendar.Date, slots calendar.Slots) []calendar.Date {
	var out []calendar.Date
	for _, d := range calendar.Range(from, to) {
		all := true
		for _, sl := range slots.Expand() {
			if !c.closed[availability.DateSlot{Date: d, Slot: sl}] {
				all = false
			}
		}
		if all {
			out = append(out, d)
		}
	}
	return out
}

// GetAvailability computes the availability of a range (bookings.read).
func (s *Service) GetAvailability(ctx context.Context, p *auth.Principal, establishmentID string, q AvailabilityQuery) (*AvailabilityResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if q.Slots == "" {
		q.Slots = calendar.SlotsFull
	}
	if q.Channel == "" {
		q.Channel = store.ChannelDesk
	}
	if err := validateRange(q.From, q.To, MaxAvailabilityDays); err != nil {
		return nil, err
	}
	if !q.Slots.Valid() {
		return nil, configerr.Invalid("slots", "Fasce non valide.")
	}
	switch q.Channel {
	case store.ChannelDesk, store.ChannelOnline, store.ChannelAPI:
	default:
		return nil, configerr.Invalid("channel", "Canale non valido.")
	}
	res, err := s.availability(ctx, es, q)
	return res, wrap("availability", err)
}

func validateRange(from, to calendar.Date, maxDays int) error {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return configerr.Invalid("to", "La data finale non può precedere quella iniziale.")
	}
	if from.DaysUntil(to) >= maxDays {
		return configerr.Invalid("to", "Intervallo troppo lungo.")
	}
	return nil
}

func (s *Service) availability(ctx context.Context, es store.EstablishmentScope, q AvailabilityQuery) (*AvailabilityResult, error) {
	now := s.d.Clock.Now()
	var pack *store.Pack
	if q.PackID != "" {
		pk, err := store.GetPackForInventory(ctx, s.d.DB, es, q.PackID)
		if err != nil {
			return nil, err
		}
		pack = pk
	}
	var zoneFilter []string
	if q.ZoneID != "" {
		zoneFilter = []string{q.ZoneID}
	}
	zones, err := store.InventoryZones(ctx, s.d.DB, es, zoneFilter)
	if err != nil {
		return nil, err
	}
	if q.ZoneID != "" && len(zones) == 0 {
		return nil, store.ErrNotFound
	}
	zones = slices.DeleteFunc(zones, func(z store.Zone) bool {
		if q.MapID != "" && (z.MapID == nil || *z.MapID != q.MapID) {
			return true
		}
		return pack != nil && pack.AllowedZoneIDs != nil && !slices.Contains(pack.AllowedZoneIDs, z.ID)
	})
	zoneIDs := make([]string, len(zones))
	for i, z := range zones {
		zoneIDs[i] = z.ID
	}
	spots, err := store.InventorySpots(ctx, s.d.DB, es, store.InventorySpotFilter{ZoneIDs: zoneIDs, ActiveOnly: true})
	if err != nil {
		return nil, err
	}
	if pack != nil && pack.RequiresSpotKind != nil {
		spots = slices.DeleteFunc(spots, func(sp store.Spot) bool { return !slices.Contains(pack.RequiresSpotKind, sp.Kind) })
	}
	cal, err := s.loadCalendar(ctx, es, q.From, q.To)
	if err != nil {
		return nil, err
	}
	iv := availability.Interval{From: q.From, To: q.To, Slots: q.Slots}
	open := cal.closed.openHalfDays(iv)
	spotIDs := make([]string, len(spots))
	for i := range spots {
		spotIDs[i] = spots[i].ID
	}
	allocs, err := store.SpotAllocationsInRange(ctx, s.d.DB, es, spotIDs, q.From, q.To, true, now)
	if err != nil {
		return nil, err
	}
	free, ids := freeSpots(spots, open, allocs)

	// pools: of the selected zones, of the pack components, or all pools
	// when nothing narrows the query.
	allPools, err := store.InventoryPools(ctx, s.d.DB, es, nil)
	if err != nil {
		return nil, err
	}
	var packPools map[string]string
	if pack != nil {
		codes := make([]string, 0, len(pack.Composition))
		for c := range pack.Composition {
			codes = append(codes, c)
		}
		if packPools, err = store.TrackedComponentPools(ctx, s.d.DB, es, codes); err != nil {
			return nil, err
		}
	}
	selectedZone := map[string]bool{}
	for _, z := range zones {
		selectedZone[z.ID] = true
	}
	narrowed := q.MapID != "" || q.ZoneID != "" || q.PackID != ""
	var pools []store.Pool
	zonePool := map[string]*store.Pool{}
	for i := range allPools {
		pl := &allPools[i]
		inZone := pl.ZoneID != nil && selectedZone[*pl.ZoneID]
		if inZone {
			if _, ok := zonePool[*pl.ZoneID]; !ok {
				zonePool[*pl.ZoneID] = pl
			}
		}
		inPack := false
		for _, id := range packPools {
			inPack = inPack || id == pl.ID
		}
		if !narrowed || inZone || inPack {
			pools = append(pools, *pl)
		}
	}
	poolIDs := make([]string, len(pools))
	for i := range pools {
		poolIDs[i] = pools[i].ID
	}
	occ, err := store.PoolOccupancyInRange(ctx, s.d.DB, es, poolIDs, q.From, q.To, now, "", "")
	if err != nil {
		return nil, err
	}
	occByKey := make(map[store.PoolKey]store.PoolOccupancy, len(occ))
	for _, o := range occ {
		occByKey[store.PoolKey{PoolID: o.PoolID, Date: o.Date, Slot: o.Slot}] = o
	}
	var online map[string]int
	if quotaBound(q.Channel) {
		online, err = s.onlineCounts(ctx, es, zones, q.From, q.To, now)
		if err != nil {
			return nil, err
		}
	}

	res := &AvailabilityResult{
		From: q.From, To: q.To, Slots: q.Slots, Channel: q.Channel,
		ClosedDates: cal.closedDates(q.From, q.To, q.Slots),
		Zones:       make([]ZoneAvailability, 0, len(zones)), Pools: make([]PoolAvailability, 0, len(pools)),
	}
	spotsByZone := map[string][]*store.Spot{}
	for i := range spots {
		spotsByZone[spots[i].ZoneID] = append(spotsByZone[spots[i].ZoneID], &spots[i])
	}
	for i := range zones {
		z := &zones[i]
		za := ZoneAvailability{ZoneID: z.ID, Code: z.Code, Mode: z.AssignmentMode}
		var quota *availability.OnlineQuota
		if z.OnlineQuota != nil {
			quota = &availability.OnlineQuota{Type: availability.QuotaType(z.OnlineQuota.Type), Value: z.OnlineQuota.Value}
		}
		zs := spotsByZone[z.ID]
		// perHalfDay: units the channel may book on each open half-day.
		perHalfDay := map[availability.DateSlot]int{}
		switch z.AssignmentMode {
		case modeSeat:
			za.Capacity = len(zs)
			for _, ds := range open {
				n := 0
				for _, sp := range zs {
					if free.IsFree(ids[sp.ID], ds.Date, ds.Slot) {
						n++
					}
				}
				perHalfDay[ds] = availability.OnlineAvailable(availability.Channel(q.Channel), availability.QuotaState{
					Capacity: len(zs), Free: n, SoldOnline: online[z.ID+"/"+ds.Date.String()+"/"+string(ds.Slot)],
				}, quota)
			}
		case modeZone, modeQuantity:
			pl := zonePool[z.ID]
			if pl == nil {
				break
			}
			za.Capacity = pl.Capacity
			for _, ds := range open {
				o := occByKey[store.PoolKey{PoolID: pl.ID, Date: ds.Date, Slot: ds.Slot}]
				capacity := effectiveCapacity(pl, ds.Date)
				perHalfDay[ds] = availability.OnlineAvailable(availability.Channel(q.Channel), availability.QuotaState{
					Capacity: capacity, Free: capacity - o.Used - o.Held, SoldOnline: o.Online,
				}, quota)
			}
		}
		minAvail := -1
		for _, ds := range open {
			if minAvail < 0 || perHalfDay[ds] < minAvail {
				minAvail = perHalfDay[ds]
			}
		}
		za.Available = max(minAvail, 0)
		if z.AssignmentMode != modeQuantity {
			za.Spots = make([]SpotAvailability, 0, len(zs))
			wholeFree := 0
			for _, sp := range zs {
				sa := SpotAvailability{SpotID: sp.ID, Label: sp.Label, Available: len(open) > 0}
				for _, ds := range open {
					switch {
					case !free.IsFree(ids[sp.ID], ds.Date, ds.Slot):
						sa.Available = false
						sa.Conflicts = append(sa.Conflicts, availability.Conflict{SpotID: sp.ID, Date: ds.Date, Slot: ds.Slot, Reason: availability.ReasonSpotUnavailable})
					case perHalfDay[ds] == 0:
						sa.Available = false
						sa.Conflicts = append(sa.Conflicts, availability.Conflict{SpotID: sp.ID, Date: ds.Date, Slot: ds.Slot, Reason: availability.ReasonQuotaExceeded})
					}
				}
				if sa.Available {
					wholeFree++
				}
				za.Spots = append(za.Spots, sa)
			}
			if z.AssignmentMode == modeSeat {
				za.Available = min(za.Available, wholeFree)
			}
		}
		res.Zones = append(res.Zones, za)
	}
	for i := range pools {
		res.Pools = append(res.Pools, poolAvailability(&pools[i], open, occByKey))
	}
	return res, nil
}

// poolAvailability builds the per half-day view of a pool (no quota).
func poolAvailability(pl *store.Pool, open []availability.DateSlot, occ map[store.PoolKey]store.PoolOccupancy) PoolAvailability {
	pa := PoolAvailability{PoolID: pl.ID, ZoneID: pl.ZoneID, ComponentCode: pl.ComponentCode, Capacity: pl.Capacity, Days: make([]PoolDay, 0, len(open))}
	minCap, minAvail := -1, -1
	for _, ds := range open {
		o := occ[store.PoolKey{PoolID: pl.ID, Date: ds.Date, Slot: ds.Slot}]
		capacity := effectiveCapacity(pl, ds.Date)
		avail := max(capacity-o.Used-o.Held, 0)
		pa.Days = append(pa.Days, PoolDay{Date: ds.Date, Slot: ds.Slot, Capacity: capacity, Used: o.Used, Held: o.Held, Available: avail})
		if minCap < 0 || capacity < minCap {
			minCap = capacity
		}
		if minAvail < 0 || avail < minAvail {
			minAvail = avail
		}
	}
	if minCap >= 0 {
		pa.Capacity = minCap
	}
	pa.Available = max(minAvail, 0)
	return pa
}

// freeSpots builds the pure FreeSpots of the active spots on the open
// half-days minus the valid allocations (INV-04). ids maps spot ids to the
// uuid keys of the pure engine.
func freeSpots(spots []store.Spot, open []availability.DateSlot, allocs []store.SpotAllocation) (*availability.FreeSpots, map[string]uuid.UUID) {
	ids := make(map[string]uuid.UUID, len(spots))
	all := make([]uuid.UUID, 0, len(spots))
	for _, sp := range spots {
		u, err := uuid.Parse(sp.ID)
		if err != nil {
			u = uuid.NewSHA1(uuid.NameSpaceOID, []byte(sp.ID))
		}
		ids[sp.ID] = u
		all = append(all, u)
	}
	free := availability.NewFreeSpots()
	for _, ds := range open {
		free.Add(ds.Date, ds.Slot, all...)
	}
	for _, a := range allocs {
		if u, ok := ids[a.SpotID]; ok {
			free.Remove(a.Date, a.Slot, u)
		}
	}
	return free, ids
}

// onlineCounts returns the online spot counts of the seat zones with a
// quota, keyed "zone/date/slot".
func (s *Service) onlineCounts(ctx context.Context, es store.EstablishmentScope, zones []store.Zone, from, to calendar.Date, now time.Time) (map[string]int, error) {
	var ids []string
	for _, z := range zones {
		if z.AssignmentMode == modeSeat && z.OnlineQuota != nil {
			ids = append(ids, z.ID)
		}
	}
	rows, err := store.OnlineSpotCounts(ctx, s.d.DB, es, ids, from, to, now)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.ZoneID+"/"+r.Date.String()+"/"+string(r.Slot)] = r.N
	}
	return out, nil
}
