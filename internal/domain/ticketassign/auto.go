package ticketassign

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/store"
)

// AutoAssign fills the free spots of the zones of the tickets still to
// assign on date, one ticket at a time by ascending number, with the greedy
// engine of spec 7.5 (availability.AutoAssign: compact row filling, ties by
// row and number). The plan is computed on the occupancy read from the
// database, releasing in memory the provisional spot of each seat-mode
// ticket as it is placed; applying runs one Move per ticket (own
// transaction), so a spot taken meanwhile leaves that ticket unassigned with
// reason conflict and is never double-booked. DryRun only computes the plan.
// Requires auth.PermBookingsWrite.
func (s *Service) AutoAssign(ctx context.Context, p *auth.Principal, establishmentID string, in AutoInput) (*AutoResult, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	if in.Date.IsZero() {
		return nil, configerr.Invalid("date", "Campo obbligatorio.")
	}
	if in.ZoneID != "" {
		if _, err := uuid.Parse(in.ZoneID); err != nil {
			return nil, configerr.Invalid("zone_id", "Zona non valida.")
		}
	}
	tickets, err := store.ListUnassignedTicketRows(ctx, s.d.DB, es, in.Date, in.ZoneID)
	if err != nil {
		return nil, err
	}
	out := &AutoResult{Assigned: []AutoAssigned{}, Unassigned: []AutoUnassigned{}}
	if len(tickets) == 0 {
		out.Applied = !in.DryRun
		return out, nil
	}
	steps, err := s.autoPlan(ctx, es, tickets)
	if err != nil {
		return nil, err
	}
	for _, st := range steps {
		t := st.target.ticket
		if st.spot == nil {
			out.Unassigned = append(out.Unassigned, AutoUnassigned{TicketID: t.ID, TicketNumber: t.TicketNumber, Reason: ReasonNoFreeSpot})
			continue
		}
		if !in.DryRun {
			err := s.apply(ctx, p, establishmentID, st.target, st.spot)
			switch {
			case errors.Is(err, ticketing.ErrAssignmentConflict), errors.Is(err, ticketing.ErrVoided), errors.Is(err, configerr.ErrVersionConflict):
				out.Unassigned = append(out.Unassigned, AutoUnassigned{TicketID: t.ID, TicketNumber: t.TicketNumber, Reason: ReasonConflict})
				continue
			case err != nil:
				return nil, err
			}
		}
		out.Assigned = append(out.Assigned, AutoAssigned{TicketID: t.ID, TicketNumber: t.TicketNumber, SpotID: st.spot.ID, SpotLabel: st.spot.Label})
	}
	out.Applied = !in.DryRun
	return out, nil
}

// autoStep is the planned spot of one ticket (nil when none is free).
type autoStep struct {
	target *target
	spot   *store.Spot
}

// autoPlan places the tickets in number order on an in-memory copy of the
// free half-days of their zones.
func (s *Service) autoPlan(ctx context.Context, es store.EstablishmentScope, tickets []store.Ticket) ([]autoStep, error) {
	targets := make([]*target, 0, len(tickets))
	var zoneIDs []string
	var from, to calendar.Date
	for i := range tickets {
		tg, err := s.loadFor(ctx, es, &tickets[i])
		if err != nil {
			return nil, err
		}
		targets = append(targets, tg)
		if !slices.Contains(zoneIDs, tg.ticket.ZoneID) {
			zoneIDs = append(zoneIDs, tg.ticket.ZoneID)
		}
		if from.IsZero() || tg.bk.StartsOn.Before(from) {
			from = tg.bk.StartsOn
		}
		if to.IsZero() || tg.bk.EndsOn.After(to) {
			to = tg.bk.EndsOn
		}
	}
	spots, err := store.ListZoneSpots(ctx, s.d.DB, es, zoneIDs)
	if err != nil {
		return nil, err
	}
	packs, err := store.ListAllPacks(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("ticketassign: packs: %w", err)
	}
	kinds := map[string][]string{}
	for _, pk := range packs {
		kinds[pk.ID] = pk.RequiresSpotKind
	}
	byID := make(map[uuid.UUID]*store.Spot, len(spots))
	spotIDs := make([]string, len(spots))
	engineSpots := make([]availability.Spot, 0, len(spots))
	free := availability.NewFreeSpots()
	all := availability.Interval{From: from, To: to, Slots: calendar.SlotsFull}
	for i := range spots {
		sp := &spots[i]
		id, err := uuid.Parse(sp.ID)
		if err != nil {
			return nil, fmt.Errorf("ticketassign: spot id: %w", err)
		}
		zid, err := uuid.Parse(sp.ZoneID)
		if err != nil {
			return nil, fmt.Errorf("ticketassign: zone id: %w", err)
		}
		byID[id], spotIDs[i] = sp, sp.ID
		engineSpots = append(engineSpots, availability.Spot{
			ID: id, ZoneID: zid, Label: sp.Label, Row: sp.RowLabel, Number: sp.Number, Kind: sp.Kind, Attributes: sp.Attributes,
		})
		free.AddInterval(all, id)
	}
	occupied, err := store.OccupiedHalfDays(ctx, s.d.DB, es, spotIDs, from, to, s.now())
	if err != nil {
		return nil, err
	}
	for _, o := range occupied {
		if id, err := uuid.Parse(o.SpotID); err == nil {
			free.Remove(o.Date, o.Slot, id)
		}
	}
	steps := make([]autoStep, 0, len(targets))
	for _, tg := range targets {
		iv := availability.Interval{From: tg.bk.StartsOn, To: tg.bk.EndsOn, Slots: tg.bk.Slots}
		// A seat-mode ticket may keep (or leave) its own provisional spot.
		var own uuid.UUID
		if tg.line.SpotID != nil {
			if id, err := uuid.Parse(*tg.line.SpotID); err == nil && byID[id] != nil {
				own = id
				free.AddInterval(iv, own)
			}
		}
		line, err := engineLine(tg, iv, kinds)
		if err != nil {
			return nil, err
		}
		res := availability.AutoAssign(availability.AssignInput{Lines: []availability.Line{line}, Spots: engineSpots, Free: free})
		step := autoStep{target: tg}
		if len(res.Assignments) == 1 && !res.Assignments[0].Split() {
			chosen := res.Assignments[0].SpotID()
			step.spot = byID[chosen]
			for _, ds := range iv.DateSlots() {
				free.Remove(ds.Date, ds.Slot, chosen)
			}
		} else if own != uuid.Nil {
			// Not placed: the provisional spot stays taken.
			for _, ds := range iv.DateSlots() {
				free.Remove(ds.Date, ds.Slot, own)
			}
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func engineLine(tg *target, iv availability.Interval, kinds map[string][]string) (availability.Line, error) {
	lid, err := uuid.Parse(tg.line.ID)
	if err != nil {
		return availability.Line{}, fmt.Errorf("ticketassign: line id: %w", err)
	}
	bid, err := uuid.Parse(tg.bk.ID)
	if err != nil {
		return availability.Line{}, fmt.Errorf("ticketassign: booking id: %w", err)
	}
	zid, _ := uuid.Parse(tg.ticket.ZoneID)
	cid, _ := uuid.Parse(tg.bk.CustomerID)
	return availability.Line{
		ID: lid, BookingID: bid, CustomerID: cid, ZoneID: zid, Interval: iv, SpotKinds: kinds[tg.line.PackID],
	}, nil
}
