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

// reminderBatch is the page size of the candidates read per establishment and kind.
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
			s.d.Log.WarnContext(ctx, "reminders skipped: invalid timezone", slog.String("establishment_id", estID), slog.String("error", err.Error()))
			continue
		}
		es, err := store.SystemEstablishmentScope(ctx, s.d.DB, estID)
		if err != nil {
			s.d.Log.WarnContext(ctx, "reminders skipped", slog.String("establishment_id", estID), slog.String("error", err.Error()))
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
	queue := func(kind string, b *store.Booking) error {
		ok, err := s.QueueReminder(ctx, es, kind, b)
		if ok {
			n++
		}
		return err
	}
	if cfg.ToCustomer.DayBeforeReminder {
		if today, at, due := ReminderWindow(now, loc, sched.DayBeforeTime); due {
			tomorrow := today.AddDays(1)
			err := s.eachCandidate(ctx, es, TemplateDayBeforeReminder, store.ReminderCandidateFilter{StartsFrom: tomorrow, StartsTo: tomorrow, CreatedBefore: at},
				func(b *store.Booking) error { return queue(TemplateDayBeforeReminder, b) })
			if err != nil {
				return n, err
			}
		}
	}
	if cfg.ToCustomer.BalanceReminder {
		if today, at, due := ReminderWindow(now, loc, sched.BalanceTime); due {
			dueDay := today.AddDays(sched.BalanceLeadDays)
			f := store.ReminderCandidateFilter{StartsFrom: dueDay, StartsTo: dueDay.AddDays(maxBalanceHorizon), CreatedBefore: at, WithBalance: true}
			err := s.eachCandidate(ctx, es, TemplateBalanceReminder, f, func(b *store.Booking) error {
				if d, _, ok := BalanceDueDate(b); !ok || !d.Equal(dueDay) {
					return nil
				}
				return queue(TemplateBalanceReminder, b)
			})
			if err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// eachCandidate calls fn for every reminder candidate of f not reminded yet
// for kind, page by page (keyset on start date and id, so rows queued by fn
// leaving the result set never shift the next page).
func (s *Service) eachCandidate(ctx context.Context, es store.EstablishmentScope, kind string, f store.ReminderCandidateFilter, fn func(*store.Booking) error) error {
	f.Limit, f.ExcludeDedupePrefix = s.reminderBatch(), ReminderDedupeKey(kind, "")
	for {
		page, err := store.ListReminderCandidates(ctx, s.d.DB, es, f)
		if err != nil {
			return err
		}
		for i := range page {
			if err := fn(&page[i]); err != nil {
				return err
			}
		}
		if len(page) < f.Limit {
			return nil
		}
		last := page[len(page)-1]
		f.AfterStartsOn, f.AfterID = last.StartsOn, last.ID
	}
}

func (s *Service) reminderBatch() int {
	if s.d.ReminderBatch > 0 {
		return s.d.ReminderBatch
	}
	return reminderBatch
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
			Payload: reminderPayload(kind, b),
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

// reminderPayload records the dates the reminder was queued for; compose
// skips the message when they changed or passed before delivery.
func reminderPayload(kind string, b *store.Booking) map[string]any {
	p := map[string]any{"starts_on": b.StartsOn.String()}
	if kind == TemplateBalanceReminder {
		if due, _, ok := BalanceDueDate(b); ok {
			p["balance_due_on"] = due.String()
		}
	}
	return p
}

// ReminderStillDue reports whether a queued reminder still applies at
// delivery: the booking dates it was queued for are unchanged and not past
// (today is the establishment local date of the delivery).
func ReminderStillDue(kind string, b *store.Booking, payload map[string]any, today calendar.Date) bool {
	if s, _ := payload["starts_on"].(string); s != b.StartsOn.String() || !today.Before(b.StartsOn) {
		return false
	}
	if kind != TemplateBalanceReminder {
		return true
	}
	due, _, ok := BalanceDueDate(b)
	queued, _ := payload["balance_due_on"].(string)
	return ok && queued == due.String() && !today.After(due)
}
