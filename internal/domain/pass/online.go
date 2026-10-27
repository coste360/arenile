package pass

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/store"
)

// Season day releases asked by the pass holder from the manage area (M4,
// spec 7.7, 12.4). Ownership: the "checkout" builder; implements
// checkout.SeasonPasses (asserted in internal/app/m4.go). Same unit of work
// as Release (lock of the pass booking, inventory, season_releases rows,
// audit + season.day_released in one transaction) with the customer as
// actor and the policy snapshot as the gate.

// ErrCustomerReleaseNotAllowed: the policy snapshot of the pass does not
// let the holder release days (customer_can_release_season_days).
var ErrCustomerReleaseNotAllowed = fmt.Errorf("%w: il rilascio dei giorni dall'area cliente non è previsto dalle condizioni dell'abbonamento", configerr.ErrConflict)

// errPreview rolls back the transaction of a preview.
var errPreview = errors.New("pass: preview")

// ReleaseAsCustomer releases days of a season pass from the manage area
// (spec 7.7, 12.4): allowed only when the policy snapshot has
// customer_can_release_season_days; credit per season_release_credit when
// the day is resold. actor is audit.ActorCustomer (magic link); es is
// resolved by the caller, which authenticated the magic link of passID.
func (s *Service) ReleaseAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, passID string, in ReleaseInput) ([]store.SeasonRelease, error) {
	return s.releaseAs(ctx, actor, es, passID, in, false)
}

// PreviewReleaseAsCustomer runs ReleaseAsCustomer and rolls it back: the
// preview applies exactly the checks and constraints of the release.
func (s *Service) PreviewReleaseAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, passID string, in ReleaseInput) ([]store.SeasonRelease, error) {
	return s.releaseAs(ctx, actor, es, passID, in, true)
}

func (s *Service) releaseAs(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, passID string, in ReleaseInput, preview bool) ([]store.SeasonRelease, error) {
	note := strings.TrimSpace(in.Note)
	if len([]rune(note)) > 500 {
		return nil, configerr.Invalid("note", "Al massimo 500 caratteri.")
	}
	var out []store.SeasonRelease
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		v, err := lockPass(ctx, tx, es, passID)
		if err != nil {
			return err
		}
		snap, err := policy.ParseSnapshot(v.Booking.PolicySnapshot)
		if err != nil || !snap.Rules.CustomerCanRelease() {
			return ErrCustomerReleaseNotAllowed
		}
		if !activeStatuses[v.Booking.Status] || v.LineID == "" || v.SpotID == "" {
			return ErrNotActive
		}
		today, err := s.today(ctx, tx, es)
		if err != nil {
			return err
		}
		days, err := s.planReleaseDays(ctx, tx, es, v, in, today)
		if err != nil {
			return err
		}
		if len(days) == 0 {
			return nil // the range is already released: nothing to do
		}
		active, err := store.ActiveSeasonReleases(ctx, tx, es, v.LineID, uniqueDates(days))
		if err != nil {
			return fmt.Errorf("pass: release: %w", err)
		}
		if dup := overlapping(days, active); len(dup) > 0 {
			return fmt.Errorf("%w (%s)", ErrAlreadyReleased, strings.Join(dateStrings(dup), ", "))
		}
		rows, spotIDs, err := s.removeReleasedDays(ctx, tx, es, passID, v, days, note, nil)
		if err != nil {
			return err
		}
		out = rows
		ids := releaseIDs(rows)
		if preview {
			return errPreview
		}
		dates := dateStrings(days)
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: actor, Action: "season_pass.days_released", EntityType: "booking", EntityID: passID,
			Diff: map[string]any{"release_ids": ids, "dates": dates, "slots": slotsOf(days), "by": "customer"},
		}); err != nil {
			return fmt.Errorf("pass: %w", err)
		}
		if _, err := s.d.Outbox.Write(ctx, tx, event.SeasonDayReleased, es.ID(), 1, event.AllocationChange{
			Reason: "day_released", BookingID: passID, ReleaseIDs: ids, SpotIDs: spotIDs, Dates: dates,
		}); err != nil {
			return fmt.Errorf("pass: %w", err)
		}
		return nil
	})
	if errors.Is(err, errPreview) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
