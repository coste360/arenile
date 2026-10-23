package pass

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
)

// ReleaseInput is a release request (api.SeasonReleaseCreate): the holder
// frees the given days, for Slots (empty = the slots of the pass).
type ReleaseInput struct {
	Dates []calendar.Date
	Slots calendar.Slots
	Note  string
}

// ListReleases returns every release of a pass (active, resold, revoked).
func (s *Service) ListReleases(ctx context.Context, p *auth.Principal, establishmentID, passID string) (*ReleaseList, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if _, err := loadPass(ctx, s.d.DB, es, passID); err != nil {
		return nil, err
	}
	rl, err := releaseList(ctx, s.d.DB, es, passID)
	if err != nil {
		return nil, err
	}
	return &rl, nil
}

// Release frees half-days of an active pass for resale (spec 7.7, INV-10):
// in one transaction it deletes the season allocations of those half-days
// (Inventory.RemoveSeasonDays), inserts one season_releases row per
// half-day, writes the audit log and season.day_released. Half-days already
// released fail with ErrAlreadyReleased (the partial unique index on active
// releases is the arbiter under concurrency). Requires operations.run.
func (s *Service) Release(ctx context.Context, p *auth.Principal, ip, establishmentID, passID string, in ReleaseInput) ([]store.SeasonRelease, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermOperationsRun)
	if err != nil {
		return nil, err
	}
	note := strings.TrimSpace(in.Note)
	var out []store.SeasonRelease
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		v, err := loadPass(ctx, tx, es, passID)
		if err != nil {
			return err
		}
		if !activeStatuses[v.Booking.Status] || v.LineID == "" || v.SpotID == "" {
			return ErrNotActive
		}
		today, err := s.today(ctx, tx, es)
		if err != nil {
			return err
		}
		days, err := planRelease(&v.Booking, in.Dates, in.Slots, today)
		if err != nil {
			return err
		}
		active, err := store.ActiveSeasonReleases(ctx, tx, es, v.LineID, uniqueDates(days))
		if err != nil {
			return fmt.Errorf("pass: release: %w", err)
		}
		if dup := overlapping(days, active); len(dup) > 0 {
			return fmt.Errorf("%w (%s)", ErrAlreadyReleased, strings.Join(dateStrings(dup), ", "))
		}
		if err := s.d.Inventory.RemoveSeasonDays(ctx, tx, es, inventory.SeasonDays{
			BookingID: passID, BookingLineID: v.LineID, SpotID: v.SpotID, Days: days,
		}); err != nil {
			return err
		}
		now := s.now()
		actor := userID(p)
		rows := make([]store.SeasonRelease, len(days))
		ids := make([]string, len(days))
		for i, d := range days {
			rows[i] = store.SeasonRelease{
				ID: store.NewID(), EstablishmentID: es.ID(), BookingID: passID, BookingLineID: v.LineID, SpotID: v.SpotID,
				Date: d.Date, Slot: d.Slot, Note: note, CreatedByUserID: actor, CreatedAt: now, UpdatedAt: now,
			}
			ids[i] = rows[i].ID
		}
		if err := store.InsertSeasonReleases(ctx, tx, es, rows); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrAlreadyReleased
			}
			return fmt.Errorf("pass: release: %w", err)
		}
		out = rows
		dates := dateStrings(days)
		return s.record(ctx, tx, p, ip, es, "season_pass.days_released", passID,
			map[string]any{"release_ids": ids, "dates": dates, "slots": slotsOf(days)},
			event.SeasonDayReleased, event.AllocationChange{
				Reason: "day_released", BookingID: passID, ReleaseIDs: ids, SpotIDs: []string{v.SpotID}, Dates: dates,
			})
	})
	return out, err
}

// Revoke cancels a release that has not been resold: the season allocation
// is recreated (Inventory.RestoreSeasonDays; a spot taken meanwhile by a
// hold or block fails with *availability.ConflictError) and revoked_at is
// set, with audit and season.day_release_revoked. A resold half-day cannot
// be revoked (ErrReleaseResold). Requires operations.run.
func (s *Service) Revoke(ctx context.Context, p *auth.Principal, ip, establishmentID, passID, releaseID string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermOperationsRun)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		v, err := loadPass(ctx, tx, es, passID)
		if err != nil {
			return err
		}
		r, err := store.GetSeasonRelease(ctx, tx, es, passID, releaseID)
		if errors.Is(err, store.ErrNotFound) {
			return ErrReleaseNotFound
		}
		if err != nil {
			return fmt.Errorf("pass: revoke: %w", err)
		}
		if err := revocable(r); err != nil {
			return err
		}
		if !activeStatuses[v.Booking.Status] {
			return ErrNotActive
		}
		today, err := s.today(ctx, tx, es)
		if err != nil {
			return err
		}
		if r.Date.Before(today) {
			return ErrReleasePast
		}
		// The conditional UPDATE decides against a concurrent resale or
		// revocation; the unique spot allocation decides against any other
		// occupant of the half-day.
		ok, err := store.RevokeSeasonRelease(ctx, tx, es, r.ID, s.now())
		if err != nil {
			return fmt.Errorf("pass: revoke: %w", err)
		}
		if !ok {
			cur, err := store.GetSeasonRelease(ctx, tx, es, passID, releaseID)
			if err != nil {
				return fmt.Errorf("pass: revoke: %w", err)
			}
			if err := revocable(cur); err != nil {
				return err
			}
			return ErrReleaseRevoked
		}
		if err := s.d.Inventory.RestoreSeasonDays(ctx, tx, es, inventory.SeasonDays{
			BookingID: passID, BookingLineID: r.BookingLineID, SpotID: r.SpotID,
			Days: []availability.DateSlot{{Date: r.Date, Slot: r.Slot}},
		}); err != nil {
			return err
		}
		return s.record(ctx, tx, p, ip, es, "season_pass.release_revoked", passID,
			map[string]any{"release_id": r.ID, "date": r.Date.String(), "slot": r.Slot},
			event.SeasonDayReleaseRevoked, event.AllocationChange{
				Reason: "release_revoked", BookingID: passID, ReleaseIDs: []string{r.ID},
				SpotIDs: []string{r.SpotID}, Dates: []string{r.Date.String()},
			})
	})
}

func revocable(r *store.SeasonRelease) error {
	switch {
	case r.ResoldBookingLineID != nil:
		return ErrReleaseResold
	case r.RevokedAt != nil:
		return ErrReleaseRevoked
	}
	return nil
}

func uniqueDates(days []availability.DateSlot) []calendar.Date {
	var out []calendar.Date
	seen := map[string]bool{}
	for _, d := range days {
		if k := d.Date.String(); !seen[k] {
			seen[k] = true
			out = append(out, d.Date)
		}
	}
	return out
}

func slotsOf(days []availability.DateSlot) []calendar.Slot {
	var out []calendar.Slot
	seen := map[calendar.Slot]bool{}
	for _, d := range days {
		if !seen[d.Slot] {
			seen[d.Slot] = true
			out = append(out, d.Slot)
		}
	}
	return out
}

// userID is the user id of a user principal (nil for API keys).
func userID(p *auth.Principal) *string {
	if p == nil || p.Kind != auth.PrincipalUser || p.UserID == "" {
		return nil
	}
	id := p.UserID
	return &id
}
