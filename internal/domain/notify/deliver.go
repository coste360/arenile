package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// Delivery (imperative shell): the job claims due rows with a conditional
// update (one worker per message, also across instances), renders them
// with the data read now, sends through the Transport and records the
// outcome. Failures are retried with exponential backoff (1, 2, 4 ... min,
// capped at 6 h); after MaxAttempts, or at once for permanent errors, the
// row stays "failed" without next_attempt_at (dead letter) and a
// message.failed event tells the manager (spec 19).

// JobDeliver is the name of the delivery job.
const JobDeliver = "notify.deliver"

// Jobs returns the background jobs of notify.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:       JobDeliver,
		Interval:   30 * time.Second,
		RunOnStart: true,
		Timeout:    5 * time.Minute,
		Wake:       s.wake,
		Run: func(ctx context.Context) error {
			// Drain in batches; stop on an empty batch.
			for {
				n, err := s.Deliver(ctx)
				if err != nil || n == 0 {
					return err
				}
			}
		},
	}}
}

// Backoff is the wait before attempt n+1 after n failed attempts.
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		return backoffBase
	}
	d := backoffBase
	for i := 1; i < attempts; i++ {
		d *= 2
		if d >= backoffCap {
			return backoffCap
		}
	}
	return d
}

// Deliver processes one batch of due messages and returns how many it
// attempted (sent, failed or cancelled).
func (s *Service) Deliver(ctx context.Context) (int, error) {
	now := s.now()
	due, err := store.ListDueMessages(ctx, s.d.DB, now, now.Add(-staleSending), s.d.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("notify: deliver: %w", err)
	}
	n := 0
	for i := range due {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		m := &due[i]
		ok, err := store.ClaimMessage(ctx, s.d.DB, m.ID, s.now(), s.now().Add(-staleSending))
		if err != nil {
			return n, fmt.Errorf("notify: deliver: %w", err)
		}
		if !ok {
			continue // another worker owns it
		}
		m.Status = store.MessageSending
		if err := s.deliverOne(ctx, m); err != nil {
			s.d.Log.ErrorContext(ctx, "notify: delivery bookkeeping failed",
				slog.String("message_id", m.ID), slog.String("error", err.Error()))
			continue
		}
		n++
	}
	return n, nil
}

// deliverOne performs one attempt on a claimed message.
func (s *Service) deliverOne(ctx context.Context, m *store.Message) error {
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, m.EstablishmentID)
	if err != nil {
		return err
	}
	to, err := s.recipient(m)
	if err != nil {
		return s.finish(ctx, es, m, nil, "", errSkip)
	}
	email, onFailure, err := s.compose(ctx, es, m, to)
	if err != nil {
		return s.finish(ctx, es, m, nil, "", err)
	}
	providerID, sendErr := s.d.Transport.Send(ctx, email)
	if sendErr != nil && onFailure != nil {
		onFailure(ctx)
	}
	return s.finish(ctx, es, m, &email, providerID, sendErr)
}

// finish records the outcome of an attempt, with the outbox event of a
// dead letter in the same transaction.
func (s *Service) finish(ctx context.Context, es store.EstablishmentScope, m *store.Message, e *Email, providerID string, sendErr error) error {
	now := s.now()
	m.UpdatedAt = now
	deadLetter := false
	switch {
	case sendErr == nil:
		m.Attempts++
		m.Status, m.SentAt, m.NextAttemptAt, m.LastError = store.MessageSent, &now, nil, nil
		if providerID != "" {
			m.ProviderMessageID = &providerID
		}
	case errors.Is(sendErr, errSkip):
		reason := "not applicable: " + strings.TrimPrefix(sendErr.Error(), errSkip.Error()+": ")
		m.Status, m.NextAttemptAt, m.LastError = store.MessageCancelled, nil, &reason
	default:
		m.Attempts++
		msg := redactError(sendErr)
		m.LastError = &msg
		m.Status = store.MessageFailed
		if errors.Is(sendErr, ErrPermanent) || m.Attempts >= s.d.MaxAttempts {
			m.NextAttemptAt = nil
			deadLetter = true
		} else {
			next := now.Add(Backoff(m.Attempts))
			m.NextAttemptAt = &next
		}
	}
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ok, err := store.FinishMessageDelivery(ctx, tx, m)
		if err != nil || !ok {
			return err
		}
		if !deadLetter {
			return nil
		}
		change := event.MessageChange{MessageID: m.ID, Template: m.Template, Attempts: m.Attempts}
		if m.BookingID != nil {
			change.BookingID = *m.BookingID
		}
		if _, err := s.d.Outbox.Write(ctx, tx, event.MessageFailed, es.ID(), 1, change); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: audit.SystemActor, Action: event.MessageFailed,
			EntityType: "message", EntityID: m.ID, Diff: map[string]any{"template": m.Template, "attempts": m.Attempts},
		})
	})
	if err != nil {
		return fmt.Errorf("notify: finish delivery: %w", err)
	}
	attrs := []any{slog.String("message_id", m.ID), slog.String("template", m.Template), slog.String("status", m.Status), slog.Int("attempts", m.Attempts)}
	if e != nil {
		attrs = append(attrs, slog.String("to", MaskEmail(e.To)))
	}
	switch {
	case deadLetter:
		s.d.Log.WarnContext(ctx, "notify: message dead-lettered", append(attrs, slog.String("error", *m.LastError))...)
	case m.Status == store.MessageFailed:
		s.d.Log.InfoContext(ctx, "notify: delivery failed, will retry", append(attrs, slog.String("error", *m.LastError))...)
	default:
		s.d.Log.DebugContext(ctx, "notify: delivery", attrs...)
	}
	return nil
}

var reEmailInText = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// redactError masks every address in an error message (SMTP replies often
// echo the recipient) and bounds its length; it is stored and shown in the
// message log.
func redactError(err error) string {
	s := reEmailInText.ReplaceAllStringFunc(err.Error(), MaskEmail)
	if r := []rune(s); len(r) > 500 {
		s = string(r[:500])
	}
	return s
}
