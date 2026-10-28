package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/store"
)

// Agreement quota (M8, spec 30.2.2, ADR 0028, INV-103), enforced after the
// allocation rows are written, in the same transaction, never by an
// application pre-check:
//
//   - the cap of an agreement (max_per_day) is the counter
//     agreement_quota_usage.used, incremented with a conditional UPDATE
//     against the cap of the agreement as configured now (the row is
//     refreshed with it, and a CHECK used <= max_units holds): a refusal
//     is a conflict agreement_quota, and a cap edited after the first sale
//     of a half-day is enforced from the next sale on;
//   - the capacity reserved to an agreement (reserved_per_day) is held
//     back from every other sale until its release (release_days_before
//     days before the date, default 3): with the agreements locked, the
//     free units of the agreement zones are recounted after the insert
//     and must stay >= the reserve still unused; otherwise a conflict
//     agreement_reserved. The release is implicit in the date: no job.
//
// Overlapping reserves of two agreements on the same zones are each
// guaranteed separately (not summed): configure disjoint zones for
// independent reserves (ADR 0028).

// quotaConfig is the part of api.AgreementQuota the inventory needs.
type quotaConfig struct {
	MaxPerDay         *int     `json:"max_per_day,omitempty"`
	ReservedPerDay    *int     `json:"reserved_per_day,omitempty"`
	ReleaseDaysBefore *int     `json:"release_days_before,omitempty"`
	ZoneIDs           []string `json:"zone_ids,omitempty"`
}

const defaultReleaseDays = 3

type quotaAgreement struct {
	id    string
	cfg   quotaConfig
	zones map[string]bool // nil = every zone
}

func (q quotaAgreement) reserved() int {
	if q.cfg.ReservedPerDay == nil {
		return 0
	}
	return *q.cfg.ReservedPerDay
}

func (q quotaAgreement) releaseOn(d calendar.Date) calendar.Date {
	n := defaultReleaseDays
	if q.cfg.ReleaseDaysBefore != nil {
		n = *q.cfg.ReleaseDaysBefore
	}
	return d.AddDays(-n)
}

func (q quotaAgreement) covers(zoneID string) bool { return q.zones == nil || q.zones[zoneID] }

func loadQuotaAgreements(ctx context.Context, db bun.IDB, es store.EstablishmentScope) ([]quotaAgreement, error) {
	rows, err := store.ListQuotaAgreements(ctx, db, es)
	if err != nil {
		return nil, err
	}
	out := make([]quotaAgreement, 0, len(rows))
	for _, r := range rows {
		var c quotaConfig
		if err := json.Unmarshal(r.Quota, &c); err != nil {
			return nil, fmt.Errorf("agreement %s quota: %w", r.ID, err)
		}
		q := quotaAgreement{id: r.ID, cfg: c}
		if len(c.ZoneIDs) > 0 {
			q.zones = map[string]bool{}
			for _, z := range c.ZoneIDs {
				q.zones[z] = true
			}
		}
		out = append(out, q)
	}
	return out, nil
}

// lineUnits is what one line of a request occupies on a half-day.
type lineUnits struct {
	lineID string
	zoneID string
	qty    int
}

// enforceAgreementQuota applies the agreement quota to an allocation just
// written (see the package comment of this file).
func (s *Service) enforceAgreementQuota(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req AllocateRequest, p *plan, snap *snapshot, now time.Time) error {
	if len(p.halfDays) == 0 {
		return nil
	}
	aqs, err := loadQuotaAgreements(ctx, tx, es)
	if err != nil || len(aqs) == 0 {
		return err
	}
	var lines []lineUnits
	touched := map[string]bool{}
	for _, l := range req.Lines {
		zone := l.ZoneID
		if zone == "" && l.SpotID != "" {
			if sp, ok := snap.spots[l.SpotID]; ok {
				zone = sp.ZoneID
			}
		}
		if zone == "" {
			continue // a pool item of a panel hold: not a spot of a zone
		}
		lines = append(lines, lineUnits{lineID: l.LineID, zoneID: zone, qty: max(l.Quantity, 1)})
		touched[zone] = true
	}
	if len(lines) == 0 {
		return nil
	}
	var mine *quotaAgreement
	var reserves []quotaAgreement
	for i := range aqs {
		q := aqs[i]
		if q.id == req.AgreementID {
			mine = &aqs[i]
		}
		if q.reserved() == 0 {
			continue
		}
		for z := range touched {
			if q.covers(z) {
				reserves = append(reserves, q)
				break
			}
		}
	}
	if mine == nil && len(reserves) == 0 {
		return nil
	}
	lockIDs := make([]string, 0, len(reserves)+1)
	for _, q := range reserves {
		lockIDs = append(lockIDs, q.id)
	}
	if mine != nil {
		lockIDs = append(lockIDs, mine.id)
	}
	if err := store.LockAgreements(ctx, tx, es, lockIDs); err != nil {
		return err
	}
	var conflicts []availability.Conflict
	if mine != nil && req.BookingID != "" {
		cs, err := s.consumeAgreementQuota(ctx, tx, es, req, *mine, lines, p.halfDays, now)
		if err != nil {
			return err
		}
		conflicts = append(conflicts, cs...)
	}
	if len(conflicts) == 0 && len(reserves) > 0 {
		cs, err := s.checkReserves(ctx, tx, es, req, reserves, touched, p, now)
		if err != nil {
			return err
		}
		conflicts = append(conflicts, cs...)
	}
	if len(conflicts) > 0 {
		sortConflicts(conflicts)
		return &availability.ConflictError{Err: availability.ErrAgreementQuotaExceeded, Conflicts: conflicts}
	}
	return nil
}

// consumeAgreementQuota increments the counter of the agreement for every
// half-day of the lines in its zones and writes the detail rows.
func (s *Service) consumeAgreementQuota(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req AllocateRequest, q quotaAgreement, lines []lineUnits, halfDays []availability.DateSlot, now time.Time) ([]availability.Conflict, error) {
	var mineLines []lineUnits
	n := 0
	for _, l := range lines {
		if q.covers(l.zoneID) {
			mineLines = append(mineLines, l)
			n += l.qty
		}
	}
	if n == 0 {
		return nil, nil
	}
	ensure := make([]store.AgreementQuotaUsage, 0, len(halfDays))
	for _, hd := range halfDays {
		ensure = append(ensure, store.AgreementQuotaUsage{
			AgreementID: q.id, Date: hd.Date, Slot: hd.Slot, Reserved: q.reserved(), MaxUnits: q.cfg.MaxPerDay,
			ReleaseOn: q.releaseOn(hd.Date), UpdatedAt: now.UTC(),
		})
	}
	if err := store.EnsureAgreementQuotaRows(ctx, tx, es, ensure); err != nil {
		return nil, err
	}
	var conflicts []availability.Conflict
	var detail []store.AgreementQuotaAllocation
	for _, hd := range halfDays {
		lim := store.QuotaLimits{MaxUnits: q.cfg.MaxPerDay, Reserved: q.reserved(), ReleaseOn: q.releaseOn(hd.Date)}
		ok, err := store.ConsumeAgreementQuota(ctx, tx, es, store.QuotaKey{AgreementID: q.id, Date: hd.Date, Slot: hd.Slot}, n, lim, now)
		if err != nil {
			return nil, err
		}
		if !ok {
			conflicts = append(conflicts, availability.Conflict{ZoneID: mineLines[0].zoneID, Date: hd.Date, Slot: hd.Slot, Reason: availability.ReasonAgreementQuota})
			continue
		}
		for _, l := range mineLines {
			if l.lineID == "" {
				continue
			}
			detail = append(detail, store.AgreementQuotaAllocation{
				ID: store.NewID(), AgreementID: q.id, BookingID: req.BookingID, BookingLineID: l.lineID,
				Date: hd.Date, Slot: hd.Slot, Quantity: l.qty, CreatedAt: now.UTC(),
			})
		}
	}
	if len(conflicts) > 0 {
		return conflicts, nil
	}
	return nil, store.InsertAgreementQuotaAllocations(ctx, tx, es, detail)
}

// checkReserves recounts, for each agreement with a reserve on the zones
// touched, the free units of its zones on every half-day not yet released:
// free >= reserve - used, otherwise the sale took reserved capacity.
func (s *Service) checkReserves(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req AllocateRequest, reserves []quotaAgreement, touched map[string]bool, p *plan, now time.Time) ([]availability.Conflict, error) {
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("timezone %q: %w", est.Timezone, err)
	}
	today := calendar.In(now, loc)
	var open []availability.DateSlot
	for _, hd := range p.halfDays {
		for _, q := range reserves {
			if today.Before(q.releaseOn(hd.Date)) {
				open = append(open, hd)
				break
			}
		}
	}
	if len(open) == 0 {
		return nil, nil
	}
	free, err := s.freeUnits(ctx, tx, es, req.StartsOn, req.EndsOn, now)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(reserves))
	for i, q := range reserves {
		ids[i] = q.id
	}
	used, err := store.AgreementUsedOn(ctx, tx, es, ids, req.StartsOn, req.EndsOn)
	if err != nil {
		return nil, err
	}
	var out []availability.Conflict
	for _, hd := range open {
		for _, q := range reserves {
			if !today.Before(q.releaseOn(hd.Date)) {
				continue
			}
			remaining := availability.ReservedRemaining(q.reserved(), used[store.QuotaKey{AgreementID: q.id, Date: hd.Date, Slot: hd.Slot}], false)
			if remaining == 0 {
				continue
			}
			f, zone := 0, ""
			for z, byHD := range free {
				if q.covers(z) {
					f += byHD[hd]
					if touched[z] && zone == "" {
						zone = z
					}
				}
			}
			if f < remaining {
				out = append(out, availability.Conflict{ZoneID: zone, Date: hd.Date, Slot: hd.Slot, Reason: availability.ReasonAgreementReserved})
				break
			}
		}
	}
	return out, nil
}

// freeUnits returns the free units per zone and half-day between from and
// to: active spots minus valid allocations for seat zones, capacity minus
// used and held for the pool of zone-mode and quantity zones.
func (s *Service) freeUnits(ctx context.Context, db bun.IDB, es store.EstablishmentScope, from, to calendar.Date, now time.Time) (map[string]map[availability.DateSlot]int, error) {
	zones, err := store.InventoryZones(ctx, db, es, nil)
	if err != nil {
		return nil, err
	}
	pools, err := store.InventoryPools(ctx, db, es, nil)
	if err != nil {
		return nil, err
	}
	zonePool := map[string]*store.Pool{}
	for i := range pools {
		if z := pools[i].ZoneID; z != nil {
			if _, ok := zonePool[*z]; !ok {
				zonePool[*z] = &pools[i]
			}
		}
	}
	days := calendar.Range(from, to)
	out := map[string]map[availability.DateSlot]int{}
	var seatZones, poolIDs []string
	for _, z := range zones {
		if z.AssignmentMode == "seat" {
			seatZones = append(seatZones, z.ID)
			continue
		}
		if pl, ok := zonePool[z.ID]; ok {
			poolIDs = append(poolIDs, pl.ID)
		}
	}
	if len(seatZones) > 0 {
		active, err := store.MappingActiveSpotCountsByZone(ctx, db, es, seatZones)
		if err != nil {
			return nil, err
		}
		occ, err := store.SeatOccupancy(ctx, db, es, seatZones, from, to, now)
		if err != nil {
			return nil, err
		}
		for _, z := range seatZones {
			m := make(map[availability.DateSlot]int, 2*len(days))
			for _, d := range days {
				for _, sl := range []calendar.Slot{calendar.AM, calendar.PM} {
					m[availability.DateSlot{Date: d, Slot: sl}] = active[z]
				}
			}
			out[z] = m
		}
		for _, o := range occ {
			out[o.ZoneID][availability.DateSlot{Date: o.Date, Slot: o.Slot}] -= o.N
		}
	}
	if len(poolIDs) > 0 {
		usage, err := store.ListPoolUsage(ctx, db, es, poolIDs, from, to)
		if err != nil {
			return nil, err
		}
		taken := map[string]map[availability.DateSlot]int{}
		for _, u := range usage {
			if taken[u.PoolID] == nil {
				taken[u.PoolID] = map[availability.DateSlot]int{}
			}
			taken[u.PoolID][availability.DateSlot{Date: u.Date, Slot: u.Slot}] = u.Used + u.Held
		}
		for zid, pl := range zonePool {
			if !slices.Contains(poolIDs, pl.ID) {
				continue
			}
			m := make(map[availability.DateSlot]int, 2*len(days))
			for _, d := range days {
				for _, sl := range []calendar.Slot{calendar.AM, calendar.PM} {
					hd := availability.DateSlot{Date: d, Slot: sl}
					m[hd] = effectiveCapacity(pl, d) - taken[pl.ID][hd]
				}
			}
			out[zid] = m
		}
	}
	return out, nil
}

// releaseAgreementQuota gives back the agreement quota of the released
// allocation rows (cancellations, moves, expired holds).
func releaseAgreementQuota(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, spots []store.SpotAllocation, pools []store.PoolAllocation, now time.Time) error {
	keys := map[store.LineHalfDay]bool{}
	var bookings []string
	add := func(b, l *string, d calendar.Date, sl calendar.Slot) {
		if b == nil || l == nil {
			return
		}
		bookings = append(bookings, *b)
		keys[store.LineHalfDay{LineID: *l, Date: d, Slot: sl}] = true
	}
	for _, r := range spots {
		add(r.BookingID, r.BookingLineID, r.Date, r.Slot)
	}
	for _, r := range pools {
		add(r.BookingID, r.BookingLineID, r.Date, r.Slot)
	}
	if len(bookings) == 0 {
		return nil
	}
	_, err := store.ReleaseAgreementQuota(ctx, tx, es, bookings, keys, now)
	return err
}

// wrapAgreementErr keeps a quota conflict as is and wraps other errors.
func wrapAgreementErr(err error) error {
	var ce *availability.ConflictError
	if errors.As(err, &ce) {
		return err
	}
	return fmt.Errorf("inventory: agreement quota: %w", err)
}
