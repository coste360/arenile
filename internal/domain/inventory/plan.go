package inventory

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the allocation: buildPlan turns an AllocateRequest and
// a snapshot of the configuration (zones, spots, pools, closures) into the
// rows to write. It performs no I/O and never reads the clock, so it is
// table-tested in plan_test.go; the imperative shell (allocate.go) loads the
// snapshot, writes the plan and lets the database constraints decide.

// Zone assignment modes (zones.assignment_mode, spec 5.2).
const (
	modeSeat     = "seat"
	modeZone     = "zone"
	modeQuantity = "quantity"
)

// quotaBound reports whether a channel is subject to online quotas: every
// self-service channel (online and api), as availability.OnlineAvailable.
func quotaBound(channel string) bool { return channel != store.ChannelDesk }

// snapshot is the configuration an allocation needs, loaded in one go.
type snapshot struct {
	zones map[string]*store.Zone
	spots map[string]*store.Spot
	pools map[string]*store.Pool
	// zonePool is the pool consumed by a zone-mode (implicit system pool) or
	// quantity zone.
	zonePool map[string]string
	// componentPool maps a tracked component code to its pool.
	componentPool map[string]string
	closed        closedSet
}

// closedSet is the set of closed half-days (closure_days).
type closedSet map[availability.DateSlot]bool

func newClosedSet(days []store.ClosureDay) closedSet {
	out := closedSet{}
	for _, d := range days {
		for _, s := range d.Slots {
			out[availability.DateSlot{Date: d.Date, Slot: s}] = true
		}
	}
	return out
}

// openHalfDays expands an interval minus the closed half-days (INV-07).
func (c closedSet) openHalfDays(iv availability.Interval) []availability.DateSlot {
	all := iv.DateSlots()
	out := make([]availability.DateSlot, 0, len(all))
	for _, ds := range all {
		if !c[ds] {
			out = append(out, ds)
		}
	}
	return out
}

// effectiveCapacity is the pool capacity on date, corrected by the first
// capacity override covering it (spec 7.3).
func effectiveCapacity(p *store.Pool, d calendar.Date) int {
	for _, o := range p.CapacityOverrides {
		if !d.Before(o.From) && !d.After(o.To) {
			return max(o.Capacity, 0)
		}
	}
	return max(p.Capacity, 0)
}

// poolDemand is the consumption of one pool by one line on every open
// half-day.
type poolDemand struct {
	lineIdx  int
	poolID   string
	quantity int
}

// plan is what Allocate writes.
type plan struct {
	halfDays []availability.DateSlot
	spotRows []store.SpotAllocation
	demands  []poolDemand
	lines    []LineAllocation
	// seatQuotaZones are the seat zones with an online quota touched by a
	// quota-bound request (checked by counting after the insert).
	seatQuotaZones []string
}

// validateOwner checks that a writing request names exactly one owner.
func validateOwner(req AllocateRequest) error {
	if (req.BookingID == "") == (req.HoldID == "") {
		return configerr.Invalid("owner", "Indicare una prenotazione oppure un hold.")
	}
	return nil
}

// validateRequest checks the shape of an AllocateRequest. The owner is
// checked separately (validateOwner): a read-only Check of a quote has none.
func validateRequest(req AllocateRequest) error {
	var ve configerr.ValidationError
	if req.BookingID != "" && req.HoldID != "" {
		ve.Add("owner", "Indicare una prenotazione oppure un hold, non entrambi.")
	}
	switch req.Kind {
	case KindHold:
		if req.ExpiresAt == nil {
			ve.Add("expires_at", "Scadenza obbligatoria per un hold.")
		}
	case KindBooking, KindSeason:
		if req.ExpiresAt != nil {
			ve.Add("expires_at", "La scadenza è ammessa solo per gli hold.")
		}
		if req.HoldID != "" {
			ve.Add("kind", "Un hold del pannello alloca solo righe di tipo hold.")
		}
	default:
		ve.Add("kind", "Tipo di allocazione non valido.")
	}
	switch req.Channel {
	case store.ChannelDesk, store.ChannelOnline, store.ChannelAPI:
	default:
		ve.Add("channel", "Canale non valido.")
	}
	if req.StartsOn.IsZero() || req.EndsOn.IsZero() || req.EndsOn.Before(req.StartsOn) {
		ve.Add("ends_on", "Intervallo di date non valido.")
	}
	if !req.Slots.Valid() {
		ve.Add("slots", "Fasce non valide.")
	}
	if len(req.Lines) == 0 {
		ve.Add("lines", "Nessuna risorsa da allocare.")
	}
	for i, l := range req.Lines {
		f := fmt.Sprintf("lines[%d]", i)
		if req.BookingID != "" && l.LineID == "" {
			ve.Add(f+".line_id", "Riga della prenotazione obbligatoria.")
		}
		if l.Quantity < 0 {
			ve.Add(f+".quantity", "La quantità non può essere negativa.")
		}
		if l.PoolID != "" && (l.SpotID != "" || l.ZoneID != "") {
			ve.Add(f+".pool_id", "Un pool si indica da solo, senza zona o postazione.")
		}
		if l.PoolID == "" && l.SpotID == "" && l.ZoneID == "" {
			ve.Add(f, "Indicare postazione, zona o pool.")
		}
		for code, n := range l.Components {
			if n < 0 {
				ve.Add(f+".components."+code, "La quantità non può essere negativa.")
			}
		}
	}
	return ve.OrNil()
}

// buildPlan resolves the lines of req against snap. Inactive or unknown
// spots surface as conflicts (reason inactive) so that the caller reports
// them like a taken spot; configuration mistakes are validation errors.
func buildPlan(req AllocateRequest, snap *snapshot, newID func() string, now time.Time) (*plan, []availability.Conflict, error) {
	if err := validateRequest(req); err != nil {
		return nil, nil, err
	}
	iv := availability.Interval{From: req.StartsOn, To: req.EndsOn, Slots: req.Slots}
	p := &plan{halfDays: snap.closed.openHalfDays(iv)}
	if len(req.Skip) > 0 {
		skip := make(map[availability.DateSlot]bool, len(req.Skip))
		for _, ds := range req.Skip {
			skip[ds] = true
		}
		p.halfDays = slices.DeleteFunc(p.halfDays, func(ds availability.DateSlot) bool { return skip[ds] })
	}
	var (
		conflicts []availability.Conflict
		ve        configerr.ValidationError
		quotaSeen = map[string]bool{}
	)
	kind := string(req.Kind)
	for i, l := range req.Lines {
		f := fmt.Sprintf("lines[%d]", i)
		qty := max(l.Quantity, 1)
		la := LineAllocation{LineID: l.LineID, AssignmentStatus: store.AssignmentNotApplicable}
		zoneID := l.ZoneID
		if l.SpotID != "" {
			sp, ok := snap.spots[l.SpotID]
			if !ok {
				ve.Add(f+".spot_id", "Postazione sconosciuta.")
				continue
			}
			if zoneID == "" {
				zoneID = sp.ZoneID
			} else if zoneID != sp.ZoneID {
				ve.Add(f+".spot_id", "La postazione non appartiene alla zona indicata.")
				continue
			}
			if !sp.Active {
				for _, ds := range p.halfDays {
					conflicts = append(conflicts, availability.Conflict{SpotID: sp.ID, Date: ds.Date, Slot: ds.Slot, Reason: availability.ReasonInactive})
				}
			}
			// Spots excluded from online sale (spec 30.11, INV-105) refuse
			// every self-service channel (online and api, as the online
			// quotas) like a taken spot, whatever the zone mode; the desk
			// sells and moves onto them all.
			if sp.OnlineExcluded && quotaBound(req.Channel) && !req.ByDesk {
				for _, ds := range p.halfDays {
					conflicts = append(conflicts, availability.Conflict{SpotID: sp.ID, Date: ds.Date, Slot: ds.Slot, Reason: availability.ReasonSpotUnavailable})
				}
			}
		}
		// zoneQty is the demand on the zone pool; components may add to it.
		lineDemand := map[string]int{}
		if l.PoolID != "" {
			if _, ok := snap.pools[l.PoolID]; !ok {
				ve.Add(f+".pool_id", "Pool sconosciuto.")
				continue
			}
			lineDemand[l.PoolID] = qty
		}
		if zoneID != "" {
			z, ok := snap.zones[zoneID]
			if !ok {
				ve.Add(f+".zone_id", "Zona sconosciuta.")
				continue
			}
			switch z.AssignmentMode {
			case modeSeat:
				if l.SpotID == "" {
					ve.Add(f+".spot_id", "Le zone a posto assegnato richiedono la postazione.")
					continue
				}
				la.AssignmentStatus = store.AssignmentAssigned
				if quotaBound(req.Channel) && z.OnlineQuota != nil && !quotaSeen[z.ID] {
					quotaSeen[z.ID] = true
					p.seatQuotaZones = append(p.seatQuotaZones, z.ID)
				}
			case modeZone, modeQuantity:
				poolID, ok := snap.zonePool[z.ID]
				if !ok {
					ve.Add(f+".zone_id", "La zona non ha un pool di capacità.")
					continue
				}
				if z.AssignmentMode == modeQuantity && l.SpotID != "" {
					ve.Add(f+".spot_id", "Le zone a quantità non hanno postazioni.")
					continue
				}
				lineDemand[poolID] = max(lineDemand[poolID], qty)
				if z.AssignmentMode == modeZone {
					la.AssignmentStatus = store.AssignmentPending
					if l.SpotID != "" {
						la.AssignmentStatus = store.AssignmentAssigned
					}
				}
			default:
				ve.Add(f+".zone_id", "Modalità di assegnazione sconosciuta.")
				continue
			}
		}
		// Tracked components consume their pools; a component bound to the
		// pool already consumed by the zone describes the same units (max).
		for _, code := range sortedKeys(l.Components) {
			n := l.Components[code]
			poolID, ok := snap.componentPool[code]
			if !ok || n == 0 {
				continue
			}
			if l.PoolID == poolID || snap.zonePool[zoneID] == poolID {
				lineDemand[poolID] = max(lineDemand[poolID], n)
				continue
			}
			lineDemand[poolID] += n
		}
		if l.SpotID != "" {
			la.SpotID = l.SpotID
			for _, ds := range p.halfDays {
				p.spotRows = append(p.spotRows, newSpotRow(req, l, ds, kind, newID, now))
			}
		}
		for _, poolID := range sortedKeys(lineDemand) {
			p.demands = append(p.demands, poolDemand{lineIdx: i, poolID: poolID, quantity: lineDemand[poolID]})
		}
		p.lines = append(p.lines, la)
	}
	if err := ve.OrNil(); err != nil {
		return nil, nil, err
	}
	// Stable lock order on PostgreSQL: rows are inserted sorted by key.
	slices.SortFunc(p.spotRows, func(a, b store.SpotAllocation) int {
		return cmp.Or(cmp.Compare(a.SpotID, b.SpotID), cmp.Compare(a.Date.String(), b.Date.String()), cmp.Compare(a.Slot, b.Slot))
	})
	return p, conflicts, nil
}

func newSpotRow(req AllocateRequest, l LineRequest, ds availability.DateSlot, kind string, newID func() string, now time.Time) store.SpotAllocation {
	row := store.SpotAllocation{ID: newID(), SpotID: l.SpotID, Date: ds.Date, Slot: ds.Slot, Kind: kind, CreatedAt: now}
	if req.BookingID != "" {
		row.BookingID, row.BookingLineID = ptr(req.BookingID), ptr(l.LineID)
	} else {
		row.HoldID = ptr(req.HoldID)
	}
	if req.Kind == KindHold {
		row.ExpiresAt = ptr(req.ExpiresAt.UTC())
	}
	return row
}

// consumption is the aggregated demand on one counter.
type consumption struct {
	key      store.PoolKey
	quantity int
}

// aggregate sums the demands of p per (pool, date, slot), sorted by key
// (stable lock order).
func (p *plan) aggregate() []consumption {
	sum := map[store.PoolKey]int{}
	for _, d := range p.demands {
		for _, ds := range p.halfDays {
			sum[store.PoolKey{PoolID: d.poolID, Date: ds.Date, Slot: ds.Slot}] += d.quantity
		}
	}
	out := make([]consumption, 0, len(sum))
	for k, n := range sum {
		out = append(out, consumption{key: k, quantity: n})
	}
	slices.SortFunc(out, func(a, b consumption) int { return comparePoolKey(a.key, b.key) })
	return out
}

func comparePoolKey(a, b store.PoolKey) int {
	return cmp.Or(cmp.Compare(a.PoolID, b.PoolID), cmp.Compare(a.Date.String(), b.Date.String()), cmp.Compare(a.Slot, b.Slot))
}

// poolRows builds the pool_allocations detail rows of p.
func (p *plan) poolRows(req AllocateRequest, lines []LineRequest, newID func() string, now time.Time) []store.PoolAllocation {
	var out []store.PoolAllocation
	kind := store.AllocationBooking
	if req.Kind == KindHold {
		kind = store.AllocationHold
	}
	for _, d := range p.demands {
		for _, ds := range p.halfDays {
			row := store.PoolAllocation{
				ID: newID(), PoolID: d.poolID, Date: ds.Date, Slot: ds.Slot, Kind: kind,
				Channel: req.Channel, Quantity: d.quantity, CreatedAt: now,
			}
			if req.BookingID != "" {
				row.BookingID, row.BookingLineID = ptr(req.BookingID), ptr(lines[d.lineIdx].LineID)
			} else {
				row.HoldID = ptr(req.HoldID)
			}
			if req.Kind == KindHold {
				row.ExpiresAt = ptr(req.ExpiresAt.UTC())
			}
			out = append(out, row)
		}
	}
	return out
}

// onlineLimit is the quota limit of a pool on a date for a request, nil
// when no quota applies (desk, pools without zone, zones without quota).
func onlineLimit(snap *snapshot, poolID, channel string, d calendar.Date) *int {
	if !quotaBound(channel) {
		return nil
	}
	pl := snap.pools[poolID]
	if pl == nil || pl.ZoneID == nil {
		return nil
	}
	z := snap.zones[*pl.ZoneID]
	if z == nil || z.OnlineQuota == nil || snap.zonePool[z.ID] != poolID {
		return nil
	}
	q := availability.OnlineQuota{Type: availability.QuotaType(z.OnlineQuota.Type), Value: z.OnlineQuota.Value}
	return ptr(q.Limit(effectiveCapacity(pl, d)))
}

// poolDeltas turns detail rows into counter deltas (sign -1 releases them),
// sorted by key.
func poolDeltas(rows []store.PoolAllocation, sign int) []store.PoolDelta {
	m := map[store.PoolKey]*store.PoolDelta{}
	for _, r := range rows {
		k := store.PoolKey{PoolID: r.PoolID, Date: r.Date, Slot: r.Slot}
		d := m[k]
		if d == nil {
			d = &store.PoolDelta{Key: k}
			m[k] = d
		}
		if r.Kind == store.AllocationHold {
			d.Held += sign * r.Quantity
		} else {
			d.Used += sign * r.Quantity
		}
		if quotaBound(r.Channel) {
			d.Online += sign * r.Quantity
		}
	}
	out := make([]store.PoolDelta, 0, len(m))
	for _, d := range m {
		out = append(out, *d)
	}
	slices.SortFunc(out, func(a, b store.PoolDelta) int { return comparePoolKey(a.Key, b.Key) })
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func ptr[T any](v T) *T { return &v }
