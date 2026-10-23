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
// release, audit, booking.expired).
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
		err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			_, err := s.mutateInTx(ctx, tx, audit.SystemActor, es, ref.ID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
				if b.HoldExpiresAt == nil || b.HoldExpiresAt.After(now) {
					return nil, errSkip
				}
				if err := Transition(Status(b.Status), StatusExpired); err != nil {
					return nil, errSkip
				}
				alloc, err := s.releaseAll(ctx, tx, es, b)
				if err != nil {
					return nil, err
				}
				b.Status = string(StatusExpired)
				b.HoldExpiresAt = nil
				return &outcome{action: eventExpired, alloc: alloc}, nil
			})
			return err
		})
		switch {
		case errors.Is(err, errSkip):
		case err != nil:
			s.d.Log.ErrorContext(ctx, "booking hold expiry failed", slog.String("booking_id", ref.ID), slog.String("error", err.Error()))
		default:
			n++
		}
	}
	return n, nil
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
