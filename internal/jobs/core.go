package jobs

import (
	"context"
	"log/slog"
	"time"
)

// Dispatcher moves events out of the outbox.
type Dispatcher interface {
	DispatchPending(ctx context.Context) (int, error)
}

// WebhookDeliverer performs due webhook attempts.
type WebhookDeliverer interface {
	DeliverDue(ctx context.Context, limit int) (int, error)
}

// Cleaner deletes expired rows and returns how many.
type Cleaner func(ctx context.Context) (int64, error)

// CoreDeps are the services used by the core jobs.
type CoreDeps struct {
	Outbox          Dispatcher
	Webhooks        WebhookDeliverer
	SessionsCleanup Cleaner
	// IdempotencyCleanup removes expired Idempotency-Key records.
	IdempotencyCleanup Cleaner
	Log                *slog.Logger
}

// Core job names.
const (
	JobOutboxDispatch     = "outbox.dispatch"
	JobWebhookDelivery    = "webhooks.deliver"
	JobSessionCleanup     = "sessions.cleanup"
	JobIdempotencyCleanup = "idempotency.cleanup"
)

// CoreJobs returns the jobs every installation runs.
func CoreJobs(d CoreDeps) []Job {
	log := d.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	var out []Job
	if d.Outbox != nil {
		out = append(out, Job{
			Name: JobOutboxDispatch, Interval: time.Second, RunOnStart: true, Timeout: time.Minute,
			Run: func(ctx context.Context) error {
				// Drain in batches until empty.
				for {
					n, err := d.Outbox.DispatchPending(ctx)
					if err != nil || n == 0 {
						return err
					}
				}
			},
		})
	}
	if d.Webhooks != nil {
		out = append(out, Job{
			Name: JobWebhookDelivery, Interval: 5 * time.Second, Timeout: 2 * time.Minute,
			Run: func(ctx context.Context) error {
				_, err := d.Webhooks.DeliverDue(ctx, 100)
				return err
			},
		})
	}
	if d.SessionsCleanup != nil {
		out = append(out, cleanupJob(JobSessionCleanup, time.Hour, d.SessionsCleanup, log))
	}
	if d.IdempotencyCleanup != nil {
		out = append(out, cleanupJob(JobIdempotencyCleanup, time.Hour, d.IdempotencyCleanup, log))
	}
	return out
}

func cleanupJob(name string, every time.Duration, fn Cleaner, log *slog.Logger) Job {
	return Job{
		Name: name, Interval: every, RunOnStart: true,
		Run: func(ctx context.Context) error {
			n, err := fn(ctx)
			if err == nil && n > 0 {
				log.InfoContext(ctx, "cleanup", slog.String("job", name), slog.Int64("deleted", n))
			}
			return err
		},
	}
}
