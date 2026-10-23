package pass

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

// MarkResold is called by the booking service, in its transaction, after a
// booking line has been allocated on spotID for the given half-days: every
// active (non revoked, not yet resold) release of those half-days gets
// resold_booking_line_id = lineID and, when the policy snapshot of the pass
// enables season_release_credit, a credit to the pass holder: percent of
// the resale line's share for that half-day (line total spread on the
// half-days of its booking), one customer_credits row per release (source
// season_release). Half-days without a release are ignored.
func (r *Resale) MarkResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineID, spotID string, dates []calendar.Date, slots calendar.Slots) error {
	releases, err := store.ResellableSeasonReleases(ctx, tx, es, spotID, dates, slots.Expand())
	if err != nil {
		return fmt.Errorf("pass: mark resold: %w", err)
	}
	if len(releases) == 0 {
		return nil
	}
	line, err := store.PassGetBookingLine(ctx, tx, es, lineID)
	if err != nil {
		return fmt.Errorf("pass: mark resold: line: %w", err)
	}
	resale, err := store.PassGetBooking(ctx, tx, es, line.BookingID)
	if err != nil {
		return fmt.Errorf("pass: mark resold: booking: %w", err)
	}
	per := resaleAmountPerHalfDay(line, resale)
	passes := map[string]*store.Booking{}
	now := r.d.Clock.Now().UTC()
	for i := range releases {
		rel := &releases[i]
		if rel.BookingID == resale.ID {
			continue // the holder re-booking their own day is not a resale
		}
		pb, err := cachedBooking(ctx, tx, es, passes, rel.BookingID)
		if err != nil {
			return fmt.Errorf("pass: mark resold: pass: %w", err)
		}
		credit := releaseCredit(per, pb.PolicySnapshot)
		ok, err := store.MarkSeasonReleaseResold(ctx, tx, es, rel.ID, lineID, credit, now)
		if err != nil {
			return fmt.Errorf("pass: mark resold: %w", err)
		}
		if !ok || credit <= 0 {
			continue
		}
		releaseID, passID := rel.ID, pb.ID
		if err := r.d.Credits.GrantCredit(ctx, tx, es, &store.CustomerCredit{
			CustomerID: pb.CustomerID, Amount: credit, Source: store.CreditSourceSeasonRelease,
			BookingID: &passID, SeasonReleaseID: &releaseID, Note: "Credito per giorno rilasciato e rivenduto", CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("pass: mark resold: credit: %w", err)
		}
	}
	return nil
}

// UnmarkResold clears resold_booking_line_id when the resale line releases
// its allocations (cancel, move): the released half-day becomes resellable
// again and a credit granted for it is reversed with a negative movement
// (source season_release) so that the ledger keeps its history.
func (r *Resale) UnmarkResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineID string) error {
	releases, err := store.SeasonReleasesResoldBy(ctx, tx, es, lineID)
	if err != nil {
		return fmt.Errorf("pass: unmark resold: %w", err)
	}
	passes := map[string]*store.Booking{}
	now := r.d.Clock.Now().UTC()
	for i := range releases {
		rel := &releases[i]
		ok, err := store.UnmarkSeasonReleaseResold(ctx, tx, es, rel.ID, lineID, now)
		if err != nil {
			return fmt.Errorf("pass: unmark resold: %w", err)
		}
		if !ok || rel.CreditAmount <= 0 {
			continue
		}
		pb, err := cachedBooking(ctx, tx, es, passes, rel.BookingID)
		if err != nil {
			return fmt.Errorf("pass: unmark resold: pass: %w", err)
		}
		releaseID, passID := rel.ID, pb.ID
		if err := r.d.Credits.GrantCredit(ctx, tx, es, &store.CustomerCredit{
			CustomerID: pb.CustomerID, Amount: -rel.CreditAmount, Source: store.CreditSourceSeasonRelease,
			BookingID: &passID, SeasonReleaseID: &releaseID, Note: "Storno credito: rivendita annullata", CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("pass: unmark resold: credit: %w", err)
		}
	}
	return nil
}

func cachedBooking(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, cache map[string]*store.Booking, id string) (*store.Booking, error) {
	if b, ok := cache[id]; ok {
		return b, nil
	}
	b, err := store.PassGetBooking(ctx, tx, es, id)
	if err != nil {
		return nil, err
	}
	cache[id] = b
	return b, nil
}
