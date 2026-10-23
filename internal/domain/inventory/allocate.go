package inventory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Imperative shell of the allocation (spec 7.2-7.7). Every method here
// runs in the caller's transaction and never opens its own.

// ErrHoldClosed: the panel hold was already released or converted.
var ErrHoldClosed = fmt.Errorf("%w: l'hold non è più attivo", configerr.ErrConflict)

// ErrSeasonDaysMismatch: a release names half-days the pass does not occupy.
var ErrSeasonDaysMismatch = fmt.Errorf("%w: l'abbonamento non occupa tutte le mezze giornate indicate", configerr.ErrConflict)

// loadSnapshot loads the configuration needed by req (few queries).
func (s *Service) loadSnapshot(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req AllocateRequest) (*snapshot, error) {
	snap := &snapshot{
		zones: map[string]*store.Zone{}, spots: map[string]*store.Spot{}, pools: map[string]*store.Pool{},
		zonePool: map[string]string{}, componentPool: map[string]string{},
	}
	var spotIDs, zoneIDs, codes []string
	for _, l := range req.Lines {
		if l.SpotID != "" {
			spotIDs = append(spotIDs, l.SpotID)
		}
		if l.ZoneID != "" {
			zoneIDs = append(zoneIDs, l.ZoneID)
		}
		for c := range l.Components {
			codes = append(codes, c)
		}
	}
	if len(spotIDs) > 0 {
		spots, err := store.InventorySpots(ctx, db, es, store.InventorySpotFilter{SpotIDs: spotIDs})
		if err != nil {
			return nil, err
		}
		for i := range spots {
			snap.spots[spots[i].ID] = &spots[i]
			zoneIDs = append(zoneIDs, spots[i].ZoneID)
		}
	}
	if len(zoneIDs) > 0 {
		zones, err := store.InventoryZones(ctx, db, es, zoneIDs)
		if err != nil {
			return nil, err
		}
		for i := range zones {
			snap.zones[zones[i].ID] = &zones[i]
		}
	}
	cp, err := store.TrackedComponentPools(ctx, db, es, codes)
	if err != nil {
		return nil, err
	}
	snap.componentPool = cp
	// Pools: all pools of the establishment are few (tens); one query.
	pools, err := store.InventoryPools(ctx, db, es, nil)
	if err != nil {
		return nil, err
	}
	for i := range pools {
		pl := &pools[i]
		snap.pools[pl.ID] = pl
		if pl.ZoneID != nil {
			if z, ok := snap.zones[*pl.ZoneID]; ok {
				if _, taken := snap.zonePool[z.ID]; !taken {
					// ordered system managed first: the implicit pool of a
					// zone-mode zone wins over hand-made ones.
					snap.zonePool[z.ID] = pl.ID
				}
			}
		}
	}
	closures, err := store.ClosureDaysInRange(ctx, db, es, req.StartsOn, req.EndsOn)
	if err != nil {
		return nil, err
	}
	snap.closed = newClosedSet(closures)
	return snap, nil
}

// Allocate occupies the resources of req in tx. Conflicts fail with
// *availability.ConflictError (ErrSpotUnavailable, ErrPoolExhausted,
// ErrQuotaExceeded) listing every refused half-day; the caller must roll
// back its transaction. Closure half-days are skipped (INV-07); expired
// holds on the touched resources are reclaimed first (INV-05).
func (s *Service) Allocate(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req AllocateRequest) (*Allocation, error) {
	if err := validateOwner(req); err != nil {
		return nil, err
	}
	snap, err := s.loadSnapshot(ctx, tx, es, req)
	if err != nil {
		return nil, fmt.Errorf("inventory: allocate: %w", err)
	}
	now := s.d.Clock.Now()
	p, conflicts, err := buildPlan(req, snap, store.NewID, now)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, &availability.ConflictError{Err: availability.ErrSpotUnavailable, Conflicts: conflicts}
	}
	if err := s.reclaimExpired(ctx, tx, es, p, req.StartsOn, req.EndsOn, now); err != nil {
		return nil, fmt.Errorf("inventory: allocate: %w", err)
	}
	if conflicts, err = s.insertSpotRows(ctx, tx, es, p.spotRows, req.StartsOn, req.EndsOn); err != nil {
		return nil, fmt.Errorf("inventory: allocate: %w", err)
	}
	if len(conflicts) > 0 {
		return nil, &availability.ConflictError{Err: availability.ErrSpotUnavailable, Conflicts: conflicts}
	}
	poolConflicts, err := s.consumePools(ctx, tx, es, p, snap, req, now)
	if err != nil {
		return nil, fmt.Errorf("inventory: allocate: %w", err)
	}
	if len(poolConflicts) > 0 {
		kind := availability.ErrPoolExhausted
		if !slices.ContainsFunc(poolConflicts, func(c availability.Conflict) bool { return c.Reason == availability.ReasonPoolExhausted }) {
			kind = availability.ErrQuotaExceeded
		}
		return nil, &availability.ConflictError{Err: kind, Conflicts: poolConflicts}
	}
	if err := store.InsertPoolAllocations(ctx, tx, es, p.poolRows(req, req.Lines, store.NewID, now)); err != nil {
		return nil, fmt.Errorf("inventory: allocate: %w", err)
	}
	if qc, err := s.checkSeatQuotas(ctx, tx, es, p.seatQuotaZones, snap, req.StartsOn, req.EndsOn, now); err != nil {
		return nil, fmt.Errorf("inventory: allocate: %w", err)
	} else if len(qc) > 0 {
		return nil, &availability.ConflictError{Err: availability.ErrQuotaExceeded, Conflicts: qc}
	}
	return summarize(p), nil
}

// reclaimExpired deletes the expired hold rows on the spots and pools of p
// and gives their units back (spec 7.2: local cleanup at conflict time).
func (s *Service) reclaimExpired(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, p *plan, from, to calendar.Date, now time.Time) error {
	spotIDs := uniqueStrings(p.spotRows, func(r store.SpotAllocation) string { return r.SpotID })
	if _, err := store.DeleteExpiredSpotHolds(ctx, tx, es, spotIDs, from, to, now); err != nil {
		return err
	}
	poolIDs := uniqueStrings(p.demands, func(d poolDemand) string { return d.poolID })
	expired, err := store.DeleteExpiredPoolHolds(ctx, tx, es, poolIDs, from, to, now)
	if err != nil {
		return err
	}
	return store.ApplyPoolDeltas(ctx, tx, es, poolDeltas(expired, -1), now)
}

// insertSpotRows inserts rows relying on the unique index; when the
// database refuses some, it reads the occupying rows and returns one
// conflict per refused half-day.
func (s *Service) insertSpotRows(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, rows []store.SpotAllocation, from, to calendar.Date) ([]availability.Conflict, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	n, err := store.InsertSpotAllocations(ctx, tx, es, rows)
	if err != nil {
		return nil, err
	}
	if n == len(rows) {
		return nil, nil
	}
	return s.spotConflicts(ctx, tx, es, rows, from, to)
}

// spotConflicts lists the half-days of rows occupied by other rows.
func (s *Service) spotConflicts(ctx context.Context, db bun.IDB, es store.EstablishmentScope, rows []store.SpotAllocation, from, to calendar.Date) ([]availability.Conflict, error) {
	ours := make(map[string]bool, len(rows))
	wanted := map[store.SpotAllocation]bool{}
	for _, r := range rows {
		ours[r.ID] = true
		wanted[spotKey(r)] = true
	}
	spotIDs := uniqueStrings(rows, func(r store.SpotAllocation) string { return r.SpotID })
	existing, err := store.SpotAllocationsInRange(ctx, db, es, spotIDs, from, to, false, time.Time{})
	if err != nil {
		return nil, err
	}
	var out []availability.Conflict
	for _, e := range existing {
		if ours[e.ID] || !wanted[spotKey(e)] {
			continue
		}
		out = append(out, availability.Conflict{SpotID: e.SpotID, Date: e.Date, Slot: e.Slot, Reason: availability.ReasonSpotUnavailable})
	}
	sortConflicts(out)
	return out, nil
}

func spotKey(r store.SpotAllocation) store.SpotAllocation {
	return store.SpotAllocation{SpotID: r.SpotID, Date: r.Date, Slot: r.Slot}
}

// consumePools applies the conditional UPDATE of every counter of p and
// returns a conflict for each refused half-day (pool_exhausted or
// quota_exceeded). The UPDATEs never fail the statement, so all refusals
// are collected before the caller rolls back.
func (s *Service) consumePools(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, p *plan, snap *snapshot, req AllocateRequest, now time.Time) ([]availability.Conflict, error) {
	cons := p.aggregate()
	if len(cons) == 0 {
		return nil, nil
	}
	keys := make([]store.PoolKey, len(cons))
	for i, c := range cons {
		keys[i] = c.key
	}
	if err := store.EnsurePoolUsageRows(ctx, tx, es, keys, now); err != nil {
		return nil, err
	}
	var out []availability.Conflict
	for _, c := range cons {
		pl := snap.pools[c.key.PoolID]
		capacity := effectiveCapacity(pl, c.key.Date)
		limit := onlineLimit(snap, pl.ID, req.Channel, c.key.Date)
		ok, err := store.ConsumePool(ctx, tx, es, store.PoolConsumption{
			Key: c.key, Quantity: c.quantity, Held: req.Kind == KindHold, Capacity: capacity,
			Online: quotaBound(req.Channel), OnlineLimit: limit,
		}, now)
		if err != nil {
			return nil, err
		}
		if ok {
			continue
		}
		u, err := store.GetPoolUsage(ctx, tx, es, c.key)
		if err != nil {
			return nil, err
		}
		reason := availability.ReasonPoolExhausted
		if u.Used+u.Held+c.quantity <= capacity && limit != nil {
			reason = availability.ReasonQuotaExceeded
		}
		cf := availability.Conflict{PoolID: pl.ID, Date: c.key.Date, Slot: c.key.Slot, Reason: reason}
		if reason == availability.ReasonQuotaExceeded && pl.ZoneID != nil {
			cf.PoolID, cf.ZoneID = "", *pl.ZoneID
		}
		out = append(out, cf)
	}
	return out, nil
}

// checkSeatQuotas enforces the online quota of seat zones after the insert:
// the zones are locked (serializing quota-bound allocations of the zone),
// then the valid online rows are counted per half-day against the limit.
func (s *Service) checkSeatQuotas(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, zoneIDs []string, snap *snapshot, from, to calendar.Date, now time.Time) ([]availability.Conflict, error) {
	if len(zoneIDs) == 0 {
		return nil, nil
	}
	sorted := slices.Sorted(slices.Values(zoneIDs))
	if err := store.LockZones(ctx, tx, es, sorted); err != nil {
		return nil, err
	}
	active, err := store.MappingActiveSpotCountsByZone(ctx, tx, es, sorted)
	if err != nil {
		return nil, err
	}
	counts, err := store.OnlineSpotCounts(ctx, tx, es, sorted, from, to, now)
	if err != nil {
		return nil, err
	}
	var out []availability.Conflict
	for _, c := range counts {
		z := snap.zones[c.ZoneID]
		q := availability.OnlineQuota{Type: availability.QuotaType(z.OnlineQuota.Type), Value: z.OnlineQuota.Value}
		if c.N > q.Limit(active[c.ZoneID]) {
			out = append(out, availability.Conflict{ZoneID: c.ZoneID, Date: c.Date, Slot: c.Slot, Reason: availability.ReasonQuotaExceeded})
		}
	}
	sortConflicts(out)
	return out, nil
}

// Check reports the conflicts Allocate would hit, without writing (quotes,
// dry runs). Expired holds count as free; the rows of req.BookingID /
// req.HoldID are ignored (re-allocation of the same owner). The owner is
// optional: a quote for a booking not created yet has none.
func (s *Service) Check(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req AllocateRequest) ([]availability.Conflict, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	snap, err := s.loadSnapshot(ctx, db, es, req)
	if err != nil {
		return nil, fmt.Errorf("inventory: check: %w", err)
	}
	now := s.d.Clock.Now()
	p, conflicts, err := buildPlan(req, snap, func() string { return "" }, now)
	if err != nil {
		return nil, err
	}
	// spots
	spotIDs := uniqueStrings(p.spotRows, func(r store.SpotAllocation) string { return r.SpotID })
	existing, err := store.SpotAllocationsInRange(ctx, db, es, spotIDs, req.StartsOn, req.EndsOn, true, now)
	if err != nil {
		return nil, fmt.Errorf("inventory: check: %w", err)
	}
	wanted := map[store.SpotAllocation]bool{}
	for _, r := range p.spotRows {
		wanted[spotKey(r)] = true
	}
	for _, e := range existing {
		if !wanted[spotKey(e)] || ownedBy(e.BookingID, req.BookingID) || ownedBy(e.HoldID, req.HoldID) {
			continue
		}
		conflicts = append(conflicts, availability.Conflict{SpotID: e.SpotID, Date: e.Date, Slot: e.Slot, Reason: availability.ReasonSpotUnavailable})
	}
	// pools
	cons := p.aggregate()
	poolIDs := uniqueStrings(cons, func(c consumption) string { return c.key.PoolID })
	occ, err := store.PoolOccupancyInRange(ctx, db, es, poolIDs, req.StartsOn, req.EndsOn, now, req.BookingID, req.HoldID)
	if err != nil {
		return nil, fmt.Errorf("inventory: check: %w", err)
	}
	byKey := make(map[store.PoolKey]store.PoolOccupancy, len(occ))
	for _, o := range occ {
		byKey[store.PoolKey{PoolID: o.PoolID, Date: o.Date, Slot: o.Slot}] = o
	}
	for _, c := range cons {
		pl := snap.pools[c.key.PoolID]
		o := byKey[c.key]
		if o.Used+o.Held+c.quantity > effectiveCapacity(pl, c.key.Date) {
			conflicts = append(conflicts, availability.Conflict{PoolID: pl.ID, Date: c.key.Date, Slot: c.key.Slot, Reason: availability.ReasonPoolExhausted})
			continue
		}
		if limit := onlineLimit(snap, pl.ID, req.Channel, c.key.Date); limit != nil && o.Online+c.quantity > *limit {
			conflicts = append(conflicts, availability.Conflict{ZoneID: *pl.ZoneID, Date: c.key.Date, Slot: c.key.Slot, Reason: availability.ReasonQuotaExceeded})
		}
	}
	// seat quotas
	if len(p.seatQuotaZones) > 0 {
		active, err := store.MappingActiveSpotCountsByZone(ctx, db, es, p.seatQuotaZones)
		if err != nil {
			return nil, fmt.Errorf("inventory: check: %w", err)
		}
		counts, err := store.OnlineSpotCounts(ctx, db, es, p.seatQuotaZones, req.StartsOn, req.EndsOn, now)
		if err != nil {
			return nil, fmt.Errorf("inventory: check: %w", err)
		}
		cur := map[string]int{}
		for _, c := range counts {
			cur[c.ZoneID+"/"+c.Date.String()+"/"+string(c.Slot)] = c.N
		}
		add := map[string]int{}
		for _, r := range p.spotRows {
			add[snap.spots[r.SpotID].ZoneID+"/"+r.Date.String()+"/"+string(r.Slot)]++
		}
		for _, zid := range p.seatQuotaZones {
			z := snap.zones[zid]
			q := availability.OnlineQuota{Type: availability.QuotaType(z.OnlineQuota.Type), Value: z.OnlineQuota.Value}
			for _, ds := range p.halfDays {
				k := zid + "/" + ds.Date.String() + "/" + string(ds.Slot)
				if add[k] > 0 && cur[k]+add[k] > q.Limit(active[zid]) {
					conflicts = append(conflicts, availability.Conflict{ZoneID: zid, Date: ds.Date, Slot: ds.Slot, Reason: availability.ReasonQuotaExceeded})
				}
			}
		}
	}
	sortConflicts(conflicts)
	return conflicts, nil
}

func ownedBy(owner *string, id string) bool { return id != "" && owner != nil && *owner == id }

// Release deletes the allocations of a booking (all lines, LineIDs, from
// From) in tx: spot rows and pool detail rows, decrementing pool_usage by
// exactly what was deleted.
func (s *Service) Release(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req ReleaseRequest) (*Allocation, error) {
	if req.BookingID == "" {
		return nil, configerr.Invalid("booking_id", "Prenotazione obbligatoria.")
	}
	owner := store.AllocationOwner{BookingID: req.BookingID, LineIDs: req.LineIDs, From: req.From}
	a, err := s.releaseOwner(ctx, tx, es, owner)
	if err != nil {
		return nil, fmt.Errorf("inventory: release: %w", err)
	}
	return a, nil
}

// releaseOwner deletes the spot and pool rows of owner and fixes the counters.
func (s *Service) releaseOwner(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, owner store.AllocationOwner) (*Allocation, error) {
	spots, err := store.DeleteSpotAllocations(ctx, tx, es, owner)
	if err != nil {
		return nil, err
	}
	pools, err := store.DeletePoolAllocations(ctx, tx, es, owner)
	if err != nil {
		return nil, err
	}
	if err := store.ApplyPoolDeltas(ctx, tx, es, poolDeltas(pools, -1), s.d.Clock.Now()); err != nil {
		return nil, err
	}
	return summarizeRows(spots, pools), nil
}

// Confirm turns the hold allocations of a booking into stable ones in tx:
// kind hold -> booking (season for passes), expires_at NULL; pool held ->
// used. A hold already expired (rows or bookings.hold_expires_at) fails with
// availability.ErrHoldExpired.
func (s *Service) Confirm(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error {
	now := s.d.Clock.Now()
	b, err := store.GetBookingHoldState(ctx, tx, es, bookingID)
	if err != nil {
		return fmt.Errorf("inventory: confirm: %w", err)
	}
	if b.HoldExpiresAt != nil && !b.HoldExpiresAt.After(now) {
		return availability.ErrHoldExpired
	}
	n, err := store.CountExpiredHoldRows(ctx, tx, es, bookingID, now)
	if err != nil {
		return fmt.Errorf("inventory: confirm: %w", err)
	}
	if n > 0 {
		return availability.ErrHoldExpired
	}
	kind := store.AllocationBooking
	if b.IsSeasonPass {
		kind = store.AllocationSeason
	}
	if _, err := store.ConfirmSpotHolds(ctx, tx, es, bookingID, kind, now); err != nil {
		return fmt.Errorf("inventory: confirm: %w", err)
	}
	rows, err := store.ConfirmPoolHolds(ctx, tx, es, bookingID, now)
	if err != nil {
		return fmt.Errorf("inventory: confirm: %w", err)
	}
	deltas := poolDeltas(rows, 1) // as booking rows now: used += q
	for i := range deltas {
		deltas[i].Held = -deltas[i].Used
		deltas[i].Online = 0
	}
	if err := store.ApplyPoolDeltas(ctx, tx, es, deltas, now); err != nil {
		return fmt.Errorf("inventory: confirm: %w", err)
	}
	return nil
}

// ExtendHold moves expires_at of the hold allocations of a booking (payment
// started, approval requested) in tx. Holds already expired fail with
// availability.ErrHoldExpired (they may have been reclaimed).
func (s *Service) ExtendHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, expiresAt time.Time) error {
	now := s.d.Clock.Now()
	if !expiresAt.After(now) {
		return configerr.Invalid("expires_at", "La nuova scadenza deve essere futura.")
	}
	n, err := store.CountExpiredHoldRows(ctx, tx, es, bookingID, now)
	if err != nil {
		return fmt.Errorf("inventory: extend hold: %w", err)
	}
	if n > 0 {
		return availability.ErrHoldExpired
	}
	if _, err := store.ExtendHoldRows(ctx, tx, es, bookingID, expiresAt, now); err != nil {
		return fmt.Errorf("inventory: extend hold: %w", err)
	}
	return nil
}

// ConvertHold turns a panel hold into the allocations of a booking in tx.
// The hold is closed first (conditional update: exactly one conversion or
// release wins), its rows are released and the booking's lines allocated
// over the hold interval and channel. Inside one transaction the freed
// rows stay locked by it, so no concurrent sale can slip in between; if the
// lines ask for more than the hold covered, the extra may conflict.
func (s *Service) ConvertHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req ConvertHoldRequest) (*Allocation, error) {
	if req.HoldID == "" || req.BookingID == "" {
		return nil, configerr.Invalid("hold_id", "Hold e prenotazione obbligatori.")
	}
	now := s.d.Clock.Now()
	h, err := store.GetHold(ctx, tx, es, req.HoldID)
	if err != nil {
		return nil, fmt.Errorf("inventory: convert hold: %w", err)
	}
	if h.ReleasedAt != nil || h.ConvertedBookingID != nil {
		return nil, ErrHoldClosed
	}
	if !h.ExpiresAt.After(now) {
		return nil, availability.ErrHoldExpired
	}
	ok, err := store.CloseHold(ctx, tx, es, h.ID, req.BookingID, now)
	if err != nil {
		return nil, fmt.Errorf("inventory: convert hold: %w", err)
	}
	if !ok {
		return nil, ErrHoldClosed
	}
	if _, err := s.releaseOwner(ctx, tx, es, store.AllocationOwner{HoldID: h.ID}); err != nil {
		return nil, fmt.Errorf("inventory: convert hold: %w", err)
	}
	kind := req.Kind
	if kind == "" {
		kind = KindBooking
	}
	a, err := s.Allocate(ctx, tx, es, AllocateRequest{
		BookingID: req.BookingID, Channel: h.Channel, StartsOn: h.StartsOn, EndsOn: h.EndsOn, Slots: h.Slots,
		Kind: kind, ExpiresAt: req.ExpiresAt, Lines: req.Lines,
	})
	if err != nil {
		return nil, err
	}
	if err := s.writeEvent(ctx, tx, es, eventHoldConverted, allocationChange("hold_converted", a, func(c *changeIDs) {
		c.holdID, c.bookingID = h.ID, req.BookingID
	})); err != nil {
		return nil, fmt.Errorf("inventory: convert hold: %w", err)
	}
	return a, nil
}

// RemoveSeasonDays deletes the season allocations of the given half-days
// (pass release, spec 7.7) in tx. Every half-day must be occupied by the
// line on the spot, otherwise ErrSeasonDaysMismatch.
func (s *Service) RemoveSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days SeasonDays) error {
	dateSlots, n := groupDays(days.Days)
	if days.BookingID == "" || days.BookingLineID == "" || days.SpotID == "" || n == 0 {
		return configerr.Invalid("days", "Abbonamento, riga, postazione e giorni obbligatori.")
	}
	rows, err := store.DeleteSpotAllocations(ctx, tx, es, store.AllocationOwner{
		BookingID: days.BookingID, LineIDs: []string{days.BookingLineID}, SpotID: days.SpotID,
		Kind: store.AllocationSeason, DateSlots: dateSlots,
	})
	if err != nil {
		return fmt.Errorf("inventory: remove season days: %w", err)
	}
	if len(rows) != n {
		return ErrSeasonDaysMismatch
	}
	return nil
}

// RestoreSeasonDays recreates the season allocations of the given half-days
// (release revoked) in tx; a half-day sold meanwhile fails with
// *availability.ConflictError{Err: ErrSpotUnavailable}.
func (s *Service) RestoreSeasonDays(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, days SeasonDays) error {
	dateSlots, n := groupDays(days.Days)
	if days.BookingID == "" || days.BookingLineID == "" || days.SpotID == "" || n == 0 {
		return configerr.Invalid("days", "Abbonamento, riga, postazione e giorni obbligatori.")
	}
	now := s.d.Clock.Now()
	rows := make([]store.SpotAllocation, 0, n)
	from, to := days.Days[0].Date, days.Days[0].Date
	for _, slot := range []calendar.Slot{calendar.AM, calendar.PM} {
		for _, d := range dateSlots[slot] {
			rows = append(rows, store.SpotAllocation{
				ID: store.NewID(), SpotID: days.SpotID, Date: d, Slot: slot, Kind: store.AllocationSeason,
				BookingID: ptr(days.BookingID), BookingLineID: ptr(days.BookingLineID), CreatedAt: now,
			})
			from, to = minDate(from, d), maxDate(to, d)
		}
	}
	if _, err := store.DeleteExpiredSpotHolds(ctx, tx, es, []string{days.SpotID}, from, to, now); err != nil {
		return fmt.Errorf("inventory: restore season days: %w", err)
	}
	conflicts, err := s.insertSpotRows(ctx, tx, es, rows, from, to)
	if err != nil {
		return fmt.Errorf("inventory: restore season days: %w", err)
	}
	if len(conflicts) > 0 {
		return &availability.ConflictError{Err: availability.ErrSpotUnavailable, Conflicts: conflicts}
	}
	return nil
}

// groupDays dedupes half-days and groups dates by slot.
func groupDays(days []availability.DateSlot) (map[calendar.Slot][]calendar.Date, int) {
	seen := map[availability.DateSlot]bool{}
	out := map[calendar.Slot][]calendar.Date{}
	for _, d := range days {
		if seen[d] || (d.Slot != calendar.AM && d.Slot != calendar.PM) || d.Date.IsZero() {
			continue
		}
		seen[d] = true
		out[d.Slot] = append(out[d.Slot], d.Date)
	}
	return out, len(seen)
}

// summarize builds the Allocation of a written plan.
func summarize(p *plan) *Allocation {
	a := &Allocation{Lines: p.lines, SpotRows: len(p.spotRows)}
	for _, d := range p.demands {
		a.PoolRows += len(p.halfDays)
		a.PoolIDs = append(a.PoolIDs, d.poolID)
	}
	for _, r := range p.spotRows {
		a.SpotIDs = append(a.SpotIDs, r.SpotID)
	}
	if len(p.spotRows) > 0 || len(p.demands) > 0 {
		for _, ds := range p.halfDays {
			a.Dates = append(a.Dates, ds.Date)
		}
	}
	return normalize(a)
}

// summarizeRows builds the Allocation of deleted rows.
func summarizeRows(spots []store.SpotAllocation, pools []store.PoolAllocation) *Allocation {
	a := &Allocation{SpotRows: len(spots), PoolRows: len(pools)}
	for _, r := range spots {
		a.SpotIDs = append(a.SpotIDs, r.SpotID)
		a.Dates = append(a.Dates, r.Date)
	}
	for _, r := range pools {
		a.PoolIDs = append(a.PoolIDs, r.PoolID)
		a.Dates = append(a.Dates, r.Date)
	}
	return normalize(a)
}

// normalize sorts and dedupes the hint lists.
func normalize(a *Allocation) *Allocation {
	a.SpotIDs = slices.Compact(slices.Sorted(slices.Values(a.SpotIDs)))
	a.PoolIDs = slices.Compact(slices.Sorted(slices.Values(a.PoolIDs)))
	slices.SortFunc(a.Dates, func(x, y calendar.Date) int { return x.Time().Compare(y.Time()) })
	a.Dates = slices.CompactFunc(a.Dates, func(x, y calendar.Date) bool { return x.Equal(y) })
	return a
}

func sortConflicts(cs []availability.Conflict) {
	slices.SortFunc(cs, func(a, b availability.Conflict) int {
		return cmp.Or(
			cmp.Compare(a.SpotID+a.PoolID+a.ZoneID, b.SpotID+b.PoolID+b.ZoneID),
			a.Date.Time().Compare(b.Date.Time()), cmp.Compare(a.Slot, b.Slot))
	})
}

func uniqueStrings[T any](items []T, key func(T) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		k := key(it)
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func minDate(a, b calendar.Date) calendar.Date {
	if b.Before(a) {
		return b
	}
	return a
}

func maxDate(a, b calendar.Date) calendar.Date {
	if b.After(a) {
		return b
	}
	return a
}
