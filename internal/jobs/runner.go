// Package jobs runs periodic background jobs inside the arenile process
// (outbox dispatch, webhook retries, cleanups). Jobs never overlap with
// themselves, recover from panics and stop on context cancellation.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Job is a periodic task.
type Job struct {
	Name     string
	Interval time.Duration
	// RunOnStart runs the job immediately when the runner starts.
	RunOnStart bool
	// Timeout bounds one run (default: Interval, at least 30s).
	Timeout time.Duration
	// Wake is optional: every receive runs the job right away instead of
	// waiting for the next tick (a producer signals new work, e.g. a queued
	// email). Signals that arrive while the job runs are coalesced by the
	// producer's buffered channel.
	Wake <-chan struct{}
	Run  func(ctx context.Context) error
}

// ErrUnknownJob is returned by RunNow for unregistered jobs.
var ErrUnknownJob = errors.New("jobs: unknown job")

// Runner schedules registered jobs.
type Runner struct {
	log  *slog.Logger
	mu   sync.Mutex
	jobs []Job
	wg   sync.WaitGroup
	busy map[string]*sync.Mutex
}

// NewRunner returns an empty runner.
func NewRunner(log *slog.Logger) *Runner {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Runner{log: log, busy: map[string]*sync.Mutex{}}
}

// Register adds a job. It must be called before Start.
func (r *Runner) Register(j Job) error {
	if j.Name == "" || j.Interval <= 0 || j.Run == nil {
		return fmt.Errorf("jobs: invalid job %q", j.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.busy[j.Name]; dup {
		return fmt.Errorf("jobs: duplicate job %q", j.Name)
	}
	r.jobs = append(r.jobs, j)
	r.busy[j.Name] = &sync.Mutex{}
	return nil
}

// Jobs returns the registered job names.
func (r *Runner) Jobs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.jobs))
	for i, j := range r.jobs {
		out[i] = j.Name
	}
	return out
}

// Start launches one goroutine per job; they stop when ctx is cancelled.
// Call Wait to block until they have returned.
func (r *Runner) Start(ctx context.Context) {
	r.mu.Lock()
	jobs := append([]Job(nil), r.jobs...)
	r.mu.Unlock()
	for _, j := range jobs {
		r.wg.Add(1)
		go func(j Job) {
			defer r.wg.Done()
			r.loop(ctx, j)
		}(j)
	}
}

// Wait blocks until every job goroutine has stopped.
func (r *Runner) Wait() { r.wg.Wait() }

// RunNow runs a job synchronously (tests, CLI). It skips if the job is
// already running.
func (r *Runner) RunNow(ctx context.Context, name string) error {
	r.mu.Lock()
	var job *Job
	for i := range r.jobs {
		if r.jobs[i].Name == name {
			job = &r.jobs[i]
		}
	}
	r.mu.Unlock()
	if job == nil {
		return fmt.Errorf("%w: %s", ErrUnknownJob, name)
	}
	return r.runOnce(ctx, *job)
}

func (r *Runner) loop(ctx context.Context, j Job) {
	if j.RunOnStart {
		_ = r.runOnce(ctx, j)
	}
	t := time.NewTicker(j.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = r.runOnce(ctx, j)
		case <-j.Wake: // nil channel: never ready
			_ = r.runOnce(ctx, j)
		}
	}
}

func (r *Runner) runOnce(ctx context.Context, j Job) (err error) {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	r.mu.Lock()
	lock := r.busy[j.Name]
	r.mu.Unlock()
	if !lock.TryLock() {
		r.log.DebugContext(ctx, "job still running, skipping", slog.String("job", j.Name))
		return nil
	}
	defer lock.Unlock()

	timeout := j.Timeout
	if timeout <= 0 {
		timeout = max(j.Interval, 30*time.Second)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("jobs: %s panicked: %v", j.Name, rec)
			r.log.ErrorContext(ctx, "job panicked", slog.String("job", j.Name), slog.Any("panic", rec), slog.String("stack", string(debug.Stack())))
		}
	}()
	err = j.Run(runCtx)
	switch {
	case err == nil:
		r.log.DebugContext(ctx, "job done", slog.String("job", j.Name), slog.Duration("duration", time.Since(start)))
	case errors.Is(err, context.Canceled) && ctx.Err() != nil:
		// shutting down
	default:
		r.log.ErrorContext(ctx, "job failed", slog.String("job", j.Name), slog.String("error", err.Error()))
	}
	return err
}
