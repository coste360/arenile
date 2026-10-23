package inventory

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Manual spot blocks (maintenance, reserved for staff): one spot_blocks row
// per spot and its allocations of kind block, subject to the same unique
// index as bookings.

// SpotBlockInput blocks spots on an interval.
type SpotBlockInput struct {
	SpotIDs  []string
	StartsOn calendar.Date
	EndsOn   calendar.Date
	Slots    calendar.Slots
	Reason   string
	IP       string
}

// maxBlockSpots bounds one request.
const maxBlockSpots = 500

func validateBlock(in *SpotBlockInput) error {
	var ve configerr.ValidationError
	in.SpotIDs = slices.Compact(slices.Sorted(slices.Values(in.SpotIDs)))
	if len(in.SpotIDs) == 0 || len(in.SpotIDs) > maxBlockSpots {
		ve.Add("spot_ids", fmt.Sprintf("Da 1 a %d postazioni.", maxBlockSpots))
	}
	if in.StartsOn.IsZero() || in.EndsOn.IsZero() || in.EndsOn.Before(in.StartsOn) {
		ve.Add("ends_on", "La data finale non può precedere quella iniziale.")
	} else if in.StartsOn.DaysUntil(in.EndsOn) >= maxHoldDays {
		ve.Add("ends_on", fmt.Sprintf("Un blocco copre al massimo %d giorni.", maxHoldDays))
	}
	if !in.Slots.Valid() {
		ve.Add("slots", "Fasce non valide.")
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > maxNoteLen {
		ve.Add("reason", fmt.Sprintf("Al massimo %d caratteri.", maxNoteLen))
	}
	return ve.OrNil()
}

// ListSpotBlocks lists blocks (bookings.read).
func (s *Service) ListSpotBlocks(ctx context.Context, p *auth.Principal, establishmentID string, f store.SpotBlockFilter, cursor string, limit int) (store.Page[store.SpotBlock], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.SpotBlock]{}, err
	}
	if !f.From.IsZero() && !f.To.IsZero() && f.To.Before(f.From) {
		return store.Page[store.SpotBlock]{}, configerr.Invalid("to", "La data finale non può precedere quella iniziale.")
	}
	page, err := store.ListSpotBlocks(ctx, s.d.DB, es, f, cursor, limit)
	return page, wrap("list spot blocks", err)
}

// CreateSpotBlocks blocks each spot on the interval (operations.run). Every
// refused half-day of every spot is reported in one
// *availability.ConflictError (409 spot_unavailable); nothing is written
// then.
func (s *Service) CreateSpotBlocks(ctx context.Context, p *auth.Principal, establishmentID string, in SpotBlockInput) ([]store.SpotBlock, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermOperationsRun)
	if err != nil {
		return nil, err
	}
	if err := validateBlock(&in); err != nil {
		return nil, err
	}
	var out []store.SpotBlock
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		spots, err := store.InventorySpots(ctx, tx, es, store.InventorySpotFilter{SpotIDs: in.SpotIDs})
		if err != nil {
			return err
		}
		if len(spots) != len(in.SpotIDs) {
			return configerr.Invalid("spot_ids", "Postazione sconosciuta.")
		}
		now := s.d.Clock.Now()
		if _, err := store.DeleteExpiredSpotHolds(ctx, tx, es, in.SpotIDs, in.StartsOn, in.EndsOn, now); err != nil {
			return err
		}
		iv := availability.Interval{From: in.StartsOn, To: in.EndsOn, Slots: in.Slots}
		var rows []store.SpotAllocation
		out = make([]store.SpotBlock, 0, len(in.SpotIDs))
		for _, spotID := range in.SpotIDs {
			b := store.SpotBlock{
				ID: store.NewID(), SpotID: spotID, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots,
				Reason: in.Reason, CreatedAt: now, UpdatedAt: now,
			}
			if p != nil && p.UserID != "" {
				b.CreatedByUserID = ptr(p.UserID)
			}
			if err := store.InsertSpotBlock(ctx, tx, es, &b); err != nil {
				return err
			}
			for _, ds := range iv.DateSlots() {
				rows = append(rows, store.SpotAllocation{
					ID: store.NewID(), SpotID: spotID, Date: ds.Date, Slot: ds.Slot, Kind: store.AllocationBlock,
					BlockID: ptr(b.ID), CreatedAt: now,
				})
			}
			out = append(out, b)
		}
		conflicts, err := s.insertSpotRows(ctx, tx, es, rows, in.StartsOn, in.EndsOn)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			return &availability.ConflictError{Err: availability.ErrSpotUnavailable, Conflicts: conflicts}
		}
		for _, b := range out {
			a := &Allocation{SpotIDs: []string{b.SpotID}}
			for _, ds := range iv.DateSlots() {
				a.Dates = append(a.Dates, ds.Date)
			}
			normalize(a)
			if err := s.record(ctx, tx, p, es, in.IP, ActionSpotBlockCreated, "spot_block", b.ID, map[string]any{
				"spot_id": b.SpotID, "starts_on": b.StartsOn, "ends_on": b.EndsOn, "slots": b.Slots, "reason": b.Reason,
			}); err != nil {
				return err
			}
			if err := s.writeEvent(ctx, tx, es, eventSpotBlockCreated, allocationChange("blocked", a, func(c *changeIDs) { c.blockID = b.ID })); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, wrap("create spot blocks", err)
	}
	return out, nil
}

// DeleteSpotBlock removes a block and frees its half-days (operations.run).
func (s *Service) DeleteSpotBlock(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermOperationsRun)
	if err != nil {
		return err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		b, err := store.GetSpotBlock(ctx, tx, es, id)
		if err != nil {
			return err
		}
		a, err := s.releaseOwner(ctx, tx, es, store.AllocationOwner{BlockID: b.ID})
		if err != nil {
			return err
		}
		if err := store.DeleteSpotBlock(ctx, tx, es, b.ID); err != nil {
			return err
		}
		if err := s.record(ctx, tx, p, es, ip, ActionSpotBlockDeleted, "spot_block", b.ID, map[string]any{
			"spot_id": b.SpotID, "starts_on": b.StartsOn, "ends_on": b.EndsOn, "slots": b.Slots,
		}); err != nil {
			return err
		}
		return s.writeEvent(ctx, tx, es, eventSpotBlockDeleted, allocationChange("unblocked", a, func(c *changeIDs) { c.blockID = b.ID }))
	})
	return wrap("delete spot block", err)
}
