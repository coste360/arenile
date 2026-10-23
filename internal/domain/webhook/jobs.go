package webhook

import (
	"context"
	"log/slog"
	"time"

	"github.com/coste360/arenile/internal/jobs"
)

// JobRetention is the name of the delivery log retention job.
const JobRetention = "webhooks.retention"

// Jobs returns the background jobs of the webhook management: the delivery
// log retention (PurgeHistory), hourly. Deliveries themselves are sent by
// the core job "webhooks.deliver" (event.Deliverer).
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name: JobRetention, Interval: time.Hour, RunOnStart: true, Timeout: 5 * time.Minute,
		Run: func(ctx context.Context) error {
			n, err := s.PurgeHistory(ctx)
			if err == nil && n > 0 {
				s.d.Log.InfoContext(ctx, "webhook deliveries purged", slog.Int64("deleted", n))
			}
			return err
		},
	}}
}
