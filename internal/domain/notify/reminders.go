package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// Automatic reminders (spec 19, ADR 0022): the day-before reminder and the
// balance reminder. The job notify.reminders runs every few minutes; for
// each establishment it computes "today" and the send instant in the
// establishment timezone (calendar.In, time.Date in the location: DST
// handled by the standard library) and, once the instant has passed and
// until the end of that local day, queues one message per eligible
// booking. A reminder whose day passed while the server was down is not
// sent late.
//
// Exactly once (INV-95): the message is queued with DedupeKey
// "reminder:<kind>:<booking id>", unique per establishment in message_log
// (INSERT ... ON CONFLICT DO NOTHING), so concurrent runs, several
// instances and later runs of the same day never queue a second one.
//
// Eligibility (INV-96), checked when queuing and again when sending
// (compose): booking confirmed (not cancelled, expired, rejected, held,
// checked in or completed), not created in test mode, created before the
// send instant (a booking made after it just got its confirmation), a
// customer who can receive email (not anonymized, not the online
// placeholder, with an address), the toggle of the kind on in the email
// settings; for the balance reminder a positive balance. The day-before
// reminder concerns bookings starting tomorrow; the balance reminder those
// whose balance due date (policy balance_due) is BalanceLeadDays from today.

// JobReminders is the name of the reminder job.
const JobReminders = "notify.reminders"

// ReminderInterval is how often the reminder job runs.
const ReminderInterval = 5 * time.Minute

// reminderBatch bounds the candidates read per establishment and kind.
const reminderBatch = 2000

// maxBalanceHorizon bounds how far ahead a balance due date can push the
// start date of a candidate (days_before:N with a large N).
const maxBalanceHorizon = 366

// ReminderDedupeKey is the message_log dedupe key of a reminder.
func ReminderDedupeKey(kind, bookingID string) string { return "reminder:" + kind + ":" + bookingID }

// ReminderJobs returns the reminder job.
func (s *Service) ReminderJobs() []jobs.Job {
	return []jobs.Job{{
		Name: JobReminders, Interval: ReminderInterval, RunOnStart: true, Timeout: 5 * time.Minute,
		Run: func(ctx context.Context) error {
			_, err := s.RunReminders(ctx)
			return err
		},
	}}
}

// ReminderWindow tells whether a reminder sent daily at hhmm is due at
// now in loc: due from the send instant until the end of the local day.
// today is the local date of now; at is the send instant (UTC).
func ReminderWindow(now time.Time, loc *time.Location, hhmm string) (today calendar.Date, at time.Time, due bool) {
	h, m, ok := online.ParseClock(hhmm)
	if !ok {
		return calendar.Date{}, time.Time{}, false
	}
	today = calendar.In(now, loc)
	// time.Date normalises a wall time skipped by DST (e.g. 02:30 on the
	// spring-forward day) to the next valid instant.
	at = time.Date(today.Year(), today.Month(), today.Day(), h, m, 0, 0, loc).UTC()
	return today, at, !now.Before(at)
}

// RunReminders queues the due reminders of every establishment and
// returns how many messages it queued.
func (s *Service) RunReminders(ctx context.Context) (int, error) {
	tzs, err := store.ListEstablishmentTimezones(ctx, s.d.DB)
	if err != nil {
		return 0, fmt.Errorf("notify: reminders: %w", err)
	}
	total := 0
	for estID, tz := range tzs {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			continue
		}
		es, err := store.SystemEstablishmentScope(ctx, s.d.DB, estID)
		if err != nil {
			continue
		}
		n, err := s.remindEstablishment(ctx, es, loc)
		total += n
		if err != nil {
			s.d.Log.ErrorContext(ctx, "reminders failed", slog.String("establishment_id", estID), slog.String("error", err.Error()))
		}
	}
	if total > 0 {
		s.kick()
	}
	return total, nil
}

// remindEstablishment queues the due reminders of one establishment.
func (s *Service) remindEstablishment(ctx context.Context, es store.EstablishmentScope, loc *time.Location) (int, error) {
	cfg, err := s.emailConfig(ctx, s.d.DB, es)
	if err != nil {
		return 0, err
	}
	sched := cfg.Schedule()
	now := s.now()
	n := 0
	if cfg.ToCustomer.DayBeforeReminder {
		if today, at, due := ReminderWindow(now, loc, sched.DayBeforeTime); due {
			tomorrow := today.AddDays(1)
			cands, err := store.ListReminderCandidates(ctx, s.d.DB, es, store.ReminderCandidateFilter{
				StartsFrom: tomorrow, StartsTo: tomorrow, CreatedBefore: at, Limit: reminderBatch,
			})
			if err != nil {
				return n, err
			}
			for i := range cands {
				ok, err := s.QueueReminder(ctx, es, TemplateDayBeforeReminder, &cands[i])
				if err != nil {
					return n, err
				}
				if ok {
					n++
				}
			}
		}
	}
	if cfg.ToCustomer.BalanceReminder {
		if today, at, due := ReminderWindow(now, loc, sched.BalanceTime); due {
			dueDay := today.AddDays(sched.BalanceLeadDays)
			cands, err := store.ListReminderCandidates(ctx, s.d.DB, es, store.ReminderCandidateFilter{
				StartsFrom: dueDay, StartsTo: dueDay.AddDays(maxBalanceHorizon), CreatedBefore: at,
				WithBalance: true, Limit: reminderBatch,
			})
			if err != nil {
				return n, err
			}
			for i := range cands {
				d, _, ok := BalanceDueDate(&cands[i])
				if !ok || !d.Equal(dueDay) {
					continue
				}
				ok, err := s.QueueReminder(ctx, es, TemplateBalanceReminder, &cands[i])
				if err != nil {
					return n, err
				}
				if ok {
					n++
				}
			}
		}
	}
	return n, nil
}

// QueueReminder queues the reminder kind for booking b when b is eligible
// and returns whether a new message was queued (false when skipped or
// already queued: the dedupe key makes it once only, INV-95).
func (s *Service) QueueReminder(ctx context.Context, es store.EstablishmentScope, kind string, b *store.Booking) (bool, error) {
	if !ReminderEligible(kind, b) {
		return false, nil
	}
	queued := false
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cu, err := store.GetCustomer(ctx, tx, es, b.CustomerID)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("notify: reminder: %w", err)
		}
		if !canReceive(cu) {
			return nil
		}
		_, created, err := s.enqueue(ctx, tx, es, Enqueue{
			Template: kind, BookingID: b.ID, CustomerID: cu.ID, Recipient: *cu.Email,
			Locale: bookingLocale(b, cu), DedupeKey: ReminderDedupeKey(kind, b.ID),
			Payload: map[string]any{"starts_on": b.StartsOn.String()},
		})
		if errors.Is(err, errInvalidRecipient) {
			return nil
		}
		if err != nil {
			return err
		}
		queued = created
		return nil
	})
	return queued, err
}

// ReminderEligible applies the booking rules of INV-96 that do not need
// the customer: confirmed, not a test booking, a balance for the balance
// reminder.
func ReminderEligible(kind string, b *store.Booking) bool {
	if b == nil || b.Status != statusConfirmed || b.Test {
		return false
	}
	switch kind {
	case TemplateDayBeforeReminder:
		return true
	case TemplateBalanceReminder:
		return b.BalanceDue() > 0
	}
	return false
}
