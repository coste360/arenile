package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/pushrelay"
	"github.com/coste360/arenile/internal/store"
)

// Delivery tuning (ADR 0027).
const (
	// DeliverInterval is the period of the push.deliver job (it also runs
	// right after the sink queues something).
	DeliverInterval = 15 * time.Second
	// MaxAttempts bounds the relay attempts of a delivery.
	MaxAttempts = 5
	// MaxAge expires a delivery not sent within an hour: a late "new
	// booking" is noise.
	MaxAge = time.Hour
	// Retention deletes closed deliveries after a week.
	Retention = 7 * 24 * time.Hour
	// lease bounds one attempt (relay timeout is 10 s).
	lease = 2 * time.Minute
	// batchSize is the most deliveries one run handles.
	batchSize = 100
)

// backoff is the wait before attempt n+1 after n failed attempts.
var backoff = []time.Duration{30 * time.Second, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute}

// Job names.
const (
	JobDeliver   = "push.deliver"
	JobRetention = "push.retention"
)

// Jobs are the background jobs of the push service.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{
		{Name: JobDeliver, Interval: DeliverInterval, Wake: s.wake, Run: func(ctx context.Context) error {
			_, err := s.DeliverDue(ctx)
			return err
		}},
		{Name: JobRetention, Interval: time.Hour, Run: s.Cleanup},
	}
}

// DeliverDue sends the due deliveries and returns how many it handled.
func (s *Service) DeliverDue(ctx context.Context) (int, error) {
	if s.d.DB == nil {
		return 0, nil
	}
	due, err := store.DuePushDeliveries(ctx, s.d.DB, s.now(), batchSize)
	if err != nil {
		return 0, err
	}
	n := 0
	var firstErr error
	for _, d := range due {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		// One failing delivery must not hold back the others: log, keep
		// going, report the first error at the end. Its lease expires and
		// it is retried later.
		handled, err := s.deliver(ctx, d)
		if err != nil {
			s.d.Log.WarnContext(ctx, "push: delivery failed", slog.String("delivery_id", d.ID), slog.String("error", err.Error()))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if handled {
			n++
		}
	}
	return n, firstErr
}

// deliver claims one delivery and sends it. It returns false when another
// worker has it.
func (s *Service) deliver(ctx context.Context, due store.DuePushDelivery) (bool, error) {
	now := s.now()
	until := now.Add(lease)
	d, err := store.ClaimPushDelivery(ctx, s.d.DB, due.ID, now, until)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	closeUnsent := func(status, reason string) (bool, error) {
		_, err := store.FinishPushDeliveryUnsent(ctx, s.d.DB, d.ID, until, s.now(), nil, status, reason, false)
		return true, err
	}
	if now.Sub(d.CreatedAt) > MaxAge {
		return closeUnsent(store.PushDeliveryExpired, "expired")
	}
	if !s.RelayConfigured() {
		return closeUnsent(store.PushDeliveryExpired, "relay not configured")
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, due.EstablishmentID)
	if errors.Is(err, store.ErrNotFound) {
		return closeUnsent(store.PushDeliveryExpired, "establishment gone")
	}
	if err != nil {
		return true, err
	}
	// Re-check the consent and the recipient at send time: push turned off,
	// device revoked or expired, user without the role.
	st, err := s.settings(ctx, s.d.DB, es)
	if err != nil {
		return true, err
	}
	if !st.topicOn(d.MessageKey) {
		return closeUnsent(store.PushDeliveryExpired, "push turned off")
	}
	r, err := store.GetPushRecipient(ctx, s.d.DB, es, d.RegistrationID)
	if errors.Is(err, store.ErrNotFound) {
		return closeUnsent(store.PushDeliveryExpired, "registration gone")
	}
	if err != nil {
		return true, err
	}
	ok, err := s.eligible(ctx, es, r, d.MessageKey, now)
	if err != nil {
		return true, err
	}
	if !ok {
		return closeUnsent(store.PushDeliveryExpired, "recipient not eligible")
	}
	token, err := s.d.Keys.Decrypt(r.TokenCiphertext, tokenAAD(r.DeviceID))
	if err != nil {
		s.d.Log.WarnContext(ctx, "push: token unreadable", slog.String("registration_id", r.ID))
		return closeUnsent(store.PushDeliveryFailed, "token unreadable")
	}
	res := s.d.Relay.Send(ctx, pushrelay.Notification{
		ID: d.ID, DevicePushID: string(token), Platform: r.Platform, Environment: r.Environment,
		Message: pushrelay.Message{Key: d.MessageKey, Locale: r.Locale},
	})
	done := s.now()
	switch {
	case res.Cancelled:
		// Shutdown during the request: back to the queue, attempt not
		// counted (the relay deduplicates on the delivery id if it got it).
		_, err = store.FinishPushDeliveryUnsent(context.WithoutCancel(ctx), s.d.DB, d.ID, until, done, &done, store.PushDeliveryQueued, res.Error, false)
	case res.Outcome == pushrelay.Delivered:
		_, err = store.FinishPushDeliverySent(ctx, s.d.DB, d.ID, until, done)
	case res.Outcome == pushrelay.Gone:
		// The token is no longer valid: forget the registration (its
		// deliveries cascade).
		s.d.Log.InfoContext(ctx, "push: token gone, registration deleted", slog.String("registration_id", r.ID))
		err = store.DeletePushRegistration(ctx, s.d.DB, es, r.ID)
	case res.Outcome == pushrelay.Retry:
		attempts := d.Attempts + 1
		var next *time.Time
		if attempts < MaxAttempts {
			wait := backoff[min(attempts-1, len(backoff)-1)]
			wait = max(wait, res.RetryAfter)
			at := done.Add(wait)
			next = &at
		}
		s.d.Log.InfoContext(ctx, "push: relay attempt failed", slog.String("delivery_id", d.ID), slog.Int("attempt", attempts), slog.String("error", res.Error))
		_, err = store.FinishPushDeliveryUnsent(ctx, s.d.DB, d.ID, until, done, next, store.PushDeliveryFailed, res.Error, true)
	default:
		s.d.Log.WarnContext(ctx, "push: relay rejected the notification", slog.String("delivery_id", d.ID), slog.Int("status", res.Status))
		_, err = store.FinishPushDeliveryUnsent(ctx, s.d.DB, d.ID, until, done, nil, store.PushDeliveryFailed, res.Error, true)
	}
	if err != nil {
		return true, fmt.Errorf("push: deliver: %w", err)
	}
	return true, nil
}

// Cleanup deletes the deliveries closed for longer than Retention.
func (s *Service) Cleanup(ctx context.Context) error {
	if s.d.DB == nil {
		return nil
	}
	n, err := store.DeleteClosedPushDeliveries(ctx, s.d.DB, s.now().Add(-Retention))
	if err != nil {
		return fmt.Errorf("push: retention: %w", err)
	}
	if n > 0 {
		s.d.Log.InfoContext(ctx, "push: closed deliveries deleted", slog.Int64("count", n))
	}
	return nil
}
