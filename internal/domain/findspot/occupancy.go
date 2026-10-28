package findspot

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Zone assignment modes with spots (a quantity zone has none).
const (
	modeSeat = "seat"
	modeZone = "zone"
)

// StoreOccupancy is the Occupancy of the database: the active spots of the
// seat and zone-mode zones requested (all of them by default) compatible
// with the pack (allowed zones, required spot kind), and their free
// half-days: every allocation blocks except holds already expired (the
// snapshot of the auto-assignment, spec 7.5; INV-04, INV-05).
type StoreOccupancy struct {
	DB    bun.IDB
	Clock clock.Clock
}

// FindSpotCandidates implements Occupancy.
func (o *StoreOccupancy) FindSpotCandidates(ctx context.Context, es store.EstablishmentScope, q Query) (*Candidates, error) {
	var (
		zones []store.Zone
		err   error
	)
	if len(q.ZoneIDs) > 0 {
		zones, err = store.ListZonesByIDs(ctx, o.DB, es, q.ZoneIDs)
	} else {
		zones, err = store.CatalogListZones(ctx, o.DB, es)
	}
	if err != nil {
		return nil, fmt.Errorf("findspot: zones: %w", err)
	}
	kinds := q.SpotKinds
	var allowedZones []string
	if q.PackID != nil && *q.PackID != "" {
		packs, err := store.ListAllPacks(ctx, o.DB, es)
		if err != nil {
			return nil, fmt.Errorf("findspot: packs: %w", err)
		}
		i := slices.IndexFunc(packs, func(p store.Pack) bool { return p.ID == *q.PackID })
		if i < 0 {
			return nil, configerr.Invalid("pack_id", "Pack inesistente.")
		}
		allowedZones = packs[i].AllowedZoneIDs
		if req := packs[i].RequiresSpotKind; len(req) > 0 {
			if len(kinds) == 0 {
				kinds = req
			} else {
				kinds = slices.DeleteFunc(slices.Clone(kinds), func(k string) bool { return !slices.Contains(req, k) })
				if len(kinds) == 0 {
					kinds = []string{"-"} // no kind satisfies both: no spot
				}
			}
		}
	}
	zoneByID := map[string]store.Zone{}
	var zoneIDs []string
	for _, z := range zones {
		if z.AssignmentMode != modeSeat && z.AssignmentMode != modeZone {
			continue
		}
		if len(allowedZones) > 0 && !slices.Contains(allowedZones, z.ID) {
			continue
		}
		zoneByID[z.ID] = z
		zoneIDs = append(zoneIDs, z.ID)
	}
	spots, err := store.ListZoneSpots(ctx, o.DB, es, zoneIDs)
	if err != nil {
		return nil, err
	}
	out := &Candidates{Free: availability.NewFreeSpots(), Info: map[string]SpotInfo{}}
	all := availability.Interval{From: q.StartsOn, To: q.EndsOn, Slots: q.Slots}
	var ids []string
	for _, sp := range spots {
		if len(kinds) > 0 && !slices.Contains(kinds, sp.Kind) {
			continue
		}
		id, err := uuid.Parse(sp.ID)
		if err != nil {
			continue
		}
		zid, _ := uuid.Parse(sp.ZoneID)
		out.Spots = append(out.Spots, availability.Spot{
			ID: id, ZoneID: zid, Label: sp.Label, Row: sp.RowLabel, Number: sp.Number, Kind: sp.Kind,
			Attributes: sp.Attributes, X: sp.X, Y: sp.Y,
		})
		cat := zoneByID[sp.ZoneID].PriceCategory
		if sp.PriceCategoryOverride != nil && *sp.PriceCategoryOverride != "" {
			cat = *sp.PriceCategoryOverride
		}
		out.Info[sp.ID] = SpotInfo{Label: sp.Label, ZoneID: sp.ZoneID, PriceCategory: cat}
		out.Free.AddInterval(all, id)
		ids = append(ids, sp.ID)
	}
	occupied, err := store.OccupiedHalfDays(ctx, o.DB, es, ids, q.StartsOn, q.EndsOn, o.Clock.Now())
	if err != nil {
		return nil, err
	}
	for _, a := range occupied {
		if id, err := uuid.Parse(a.SpotID); err == nil {
			out.Free.Remove(a.Date, a.Slot, id)
		}
	}
	if q.CustomerID != nil && *q.CustomerID != "" {
		prev, err := store.PreviousSpots(ctx, o.DB, es, []string{*q.CustomerID}, q.StartsOn)
		if err != nil {
			return nil, err
		}
		out.PreviousSpotID = prev[*q.CustomerID]
	}
	return out, nil
}
