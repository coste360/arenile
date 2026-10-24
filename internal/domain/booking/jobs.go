package booking

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// Job names of the booking service.
const (
	JobExpireHolds = "bookings.expire_holds"
	JobComplete    = "bookings.complete"
)

// Batch sizes of one job run.
const (
	expireBatch   = 200
	completeBatch = 500
)

// errSkip rolls back a job transaction that found nothing to do (the
// booking changed between the scan and the lock).
var errSkip = errors.New("booking: skip")

// Jobs returns the background jobs of the booking service: expiry of the
// booking holds every 30 s (held, pending_payment, awaiting_approval past
// hold_expires_at -> expired, resources released) and the automatic
// completion of past bookings every hour. Registered by the app.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{
		{Name: JobExpireHolds, Interval: 30 * time.Second, RunOnStart: true, Timeout: 2 * time.Minute, Run: func(ctx context.Context) error {
			_, err := s.ExpireHolds(ctx)
			return err
		}},
		{Name: JobComplete, Interval: time.Hour, RunOnStart: true, Timeout: 5 * time.Minute, Run: func(ctx context.Context) error {
			_, err := s.CompletePast(ctx)
			return err
		}},
	}
}

// ExpireHolds expires the booking holds whose time is over and returns how
// many bookings it expired. Each booking is one transaction (status,
// release, audit, booking.expired). A booking awaiting approval that was
// paid (at the desk, rule R2) is left to the manager: expiring it would
// keep the customer's money on an expired booking (only online money has
// an automatic refund, INV-51).
func (s *Service) ExpireHolds(ctx context.Context) (int, error) {
	now := s.now()
	refs, err := store.ListExpiredBookingHolds(ctx, s.d.DB, now, expireBatch)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ref := range refs {
		es, err := store.SystemEstablishmentScope(ctx, s.d.DB, ref.EstablishmentID)
		if err != nil {
			continue
		}
		extended := false
		err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			_, err := s.mutateInTx(ctx, tx, audit.SystemActor, es, ref.ID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
				if b.HoldExpiresAt == nil || b.HoldExpiresAt.After(now) {
					return nil, errSkip
				}
				if Status(b.Status) == StatusAwaitingApproval && b.PaidAmount > 0 {
					// Money collected at the desk (rule R2) is never left on
					// an expired booking without a refund: the hold is renewed
					// and the booking waits for the manager, who approves or
					// rejects (and refunds).
					extended = true
					return s.renewApprovalHoldLocked(ctx, tx, es, b)
				}
				if err := Transition(Status(b.Status), StatusExpired); err != nil {
					return nil, errSkip
				}
				return s.expireLocked(ctx, tx, es, b, nil)
			})
			return err
		})
		switch {
		case errors.Is(err, errSkip), extended && err == nil:
		case err != nil:
			s.d.Log.ErrorContext(ctx, "booking hold expiry failed", slog.String("booking_id", ref.ID), slog.String("error", err.Error()))
		default:
			n++
		}
	}
	return n, nil
}

// renewApprovalHoldLocked extends the approval hold of a paid booking
// awaiting approval by another ApprovalTTL (audit booking.hold_extended,
// reason awaiting_approval_paid): its resources stay taken until the
// manager decides.
func (s *Service) renewApprovalHoldLocked(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
	until := s.now().Add(s.d.ApprovalTTL)
	if err := s.d.Inventory.ExtendHold(ctx, tx, es, b.ID, until); err != nil {
		return nil, err
	}
	prev := time.Time{}
	if b.HoldExpiresAt != nil {
		prev = *b.HoldExpiresAt
	}
	b.HoldExpiresAt = &until
	return &outcome{action: actionHoldExtended, event: eventModified, diff: map[string]any{
		"hold_expires_at": until.Format(time.RFC3339), "previous_hold_expires_at": prev.Format(time.RFC3339),
		"reason": "awaiting_approval_paid", "paid_amount": int64(b.PaidAmount),
	}}, nil
}

// expireLocked moves the locked booking b to expired: resources released,
// booking.expired with diff, and, when something was paid, the refund in
// the same transaction: what was paid with customer credit goes back as
// credit at once (INV-71), what was paid online is scheduled by the
// ExpiredRefunder (INV-51: an expired booking does not keep the customer's
// money).
func (s *Service) expireLocked(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, diff map[string]any) (*outcome, error) {
	alloc, err := s.releaseAll(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	b.Status = string(StatusExpired)
	b.HoldExpiresAt = nil
	o := &outcome{action: eventExpired, diff: diff, alloc: alloc}
	if b.PaidAmount > 0 && s.d.ExpiredRefunds != nil {
		if o.diff == nil {
			o.diff = map[string]any{}
		}
		o.diff["paid_amount"] = int64(b.PaidAmount)
		o.diff["refund_scheduled"] = true
		o.after = func(ctx context.Context, tx bun.IDB, b *store.Booking) error {
			return s.d.ExpiredRefunds.RefundExpiredBooking(ctx, tx, es, b.ID)
		}
	}
	if err := s.returnCreditOnEnd(ctx, tx, es, b, o); err != nil {
		return nil, err
	}
	return o, nil
}

// ExpireHold expires a held or pending_payment booking whose hold already
// elapsed, in tx (the transition of ExpireHolds, on demand): the payment
// gateway calls it when a provider payment arrives after hold_expires_at
// and before the sweeper ran. ErrInvalidTransition when the booking is in
// another status; a hold still running is a conflict. Implements
// gateway.HoldExpirer.
func (s *Service) ExpireHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error {
	_, err := s.mutateInTx(ctx, tx, audit.SystemActor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		st := Status(b.Status)
		if st != StatusHeld && st != StatusPendingPayment {
			return nil, &TransitionError{From: st, To: StatusExpired}
		}
		if b.HoldExpiresAt == nil || b.HoldExpiresAt.After(s.now()) {
			return nil, fmt.Errorf("%w: il blocco della prenotazione non è ancora scaduto", configerr.ErrConflict)
		}
		return s.expireLocked(ctx, tx, es, b, map[string]any{"reason": "payment_after_hold"})
	})
	return err
}

// CompletePast moves confirmed and checked-in bookings whose last day is
// before today (establishment timezone) to completed. Allocations stay: they
// are the history of past occupancy.
func (s *Service) CompletePast(ctx context.Context) (int, error) {
	tzs, err := store.ListEstablishmentTimezones(ctx, s.d.DB)
	if err != nil {
		return 0, err
	}
	n := 0
	for estID, tz := range tzs {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			continue
		}
		es, err := store.SystemEstablishmentScope(ctx, s.d.DB, estID)
		if err != nil {
			continue
		}
		today := calendar.In(s.now(), loc)
		ids, err := store.ListBookingsEndedBefore(ctx, s.d.DB, es, today, []string{string(StatusConfirmed), string(StatusCheckedIn)}, completeBatch)
		if err != nil {
			return n, fmt.Errorf("booking: complete: %w", err)
		}
		for _, id := range ids {
			err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
				_, err := s.mutateInTx(ctx, tx, audit.SystemActor, es, id, nil, func(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, b *store.Booking) (*outcome, error) {
					if err := Transition(Status(b.Status), StatusCompleted); err != nil || !b.EndsOn.Before(today) {
						return nil, errSkip
					}
					b.Status = string(StatusCompleted)
					return &outcome{action: eventCompleted}, nil
				})
				return err
			})
			switch {
			case errors.Is(err, errSkip):
			case err != nil:
				s.d.Log.ErrorContext(ctx, "booking completion failed", slog.String("booking_id", id), slog.String("error", err.Error()))
			default:
				n++
			}
		}
	}
	return n, nil
}
