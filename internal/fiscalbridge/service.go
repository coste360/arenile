package fiscalbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// ErrNotImplemented: kept for the scaffolding contract (501).
var ErrNotImplemented = configerr.ErrNotImplemented

// Poll limits.
const (
	DefaultMaxJobs = 10
	MaxJobsLimit   = 50
	// defaultRecheck bounds how long a waiting poll trusts the in-process
	// notifier: it looks at the queue again at least this often (retry
	// backoffs maturing, jobs enqueued by another instance).
	defaultRecheck = 5 * time.Second
)

var driverCodeRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

// Deps are the dependencies of Service.
type Deps struct {
	DB    *store.DB
	Clock clock.Clock
	Audit *audit.Writer
	Log   *slog.Logger
	// Recheck overrides how often a waiting poll re-reads the queue
	// (default 5 s; tests use a few milliseconds).
	Recheck time.Duration
}

// Service is the instance side of the fiscal bridge. It implements Queue
// (for Driver) and device.BridgeRevoker. Waiting long-polls are woken by an
// in-process notifier on Enqueue (single instance; multi-instance
// deployments fall back to the Recheck interval).
type Service struct {
	d      Deps
	notify *notifier
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Recheck <= 0 {
		d.Recheck = defaultRecheck
	}
	return &Service{d: d, notify: newNotifier()}
}

// deviceScope checks that p is a device principal allowed to operate the
// cash desk of its establishment and returns that establishment.
func (s *Service) deviceScope(ctx context.Context, p *auth.Principal) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	if p.DeviceID == "" || p.DeviceEstablishmentID == "" {
		return store.EstablishmentScope{}, ErrNotDevice
	}
	return p.Authorize(ctx, s.d.DB, p.DeviceEstablishmentID, auth.PermCashOperate)
}

// Register makes the device of p the active bridge of its establishment
// (deactivating any other) or refreshes its driver and device status.
// Requires a device principal with auth.PermCashOperate. Audit
// "fiscal_bridge.registered" only when the active device changes, since
// the agent registers again periodically to report the device status.
func (s *Service) Register(ctx context.Context, p *auth.Principal, reg Registration) (Status, error) {
	es, err := s.deviceScope(ctx, p)
	if err != nil {
		return Status{}, err
	}
	if !driverCodeRE.MatchString(reg.Driver) {
		return Status{}, errInvalid("driver", "codice driver non valido")
	}
	var ds store.JSON
	if reg.DeviceStatus != nil {
		st := *reg.DeviceStatus
		if rs := []rune(st.Message); len(rs) > MaxErrorLen {
			st.Message = string(rs[:MaxErrorLen])
		}
		if ds, err = store.MarshalJSONValue(st); err != nil {
			return Status{}, err
		}
	}
	register := func() error {
		return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			prev, _, err := store.GetActiveFiscalBridge(ctx, tx, es)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if _, err := store.ActivateFiscalBridge(ctx, tx, es, p.DeviceID, reg.Driver, ds, s.now()); err != nil {
				return err
			}
			if prev != nil && prev.DeviceID == p.DeviceID {
				return nil
			}
			diff := map[string]any{"device_id": p.DeviceID, "driver": reg.Driver}
			if prev != nil {
				diff["previous_device_id"] = prev.DeviceID
			}
			return s.d.Audit.Record(ctx, tx, audit.Entry{
				EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "fiscal_bridge.registered",
				EntityType: "device", EntityID: p.DeviceID, Diff: diff,
			})
		})
	}
	err = register()
	if errors.Is(err, store.ErrConflict) {
		// Another device activated concurrently: last writer wins.
		err = register()
	}
	if err != nil {
		return Status{}, fmt.Errorf("fiscalbridge: register: %w", err)
	}
	s.notify.broadcast(es.ID()) // a waiting poll of the previous bridge returns now
	return s.status(ctx, es)
}

// Unregister deactivates the registration of the device of p (idempotent)
// and returns its leased jobs to the queue. Audit "fiscal_bridge.unregistered".
func (s *Service) Unregister(ctx context.Context, p *auth.Principal) error {
	es, err := s.deviceScope(ctx, p)
	if err != nil {
		return err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		was, err := store.DeactivateFiscalBridgeOfDevice(ctx, tx, es, p.DeviceID, s.now())
		if err != nil || !was {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "fiscal_bridge.unregistered",
			EntityType: "device", EntityID: p.DeviceID,
		})
	})
	if err != nil {
		return fmt.Errorf("fiscalbridge: unregister: %w", err)
	}
	s.notify.broadcast(es.ID())
	return nil
}

// Poll leases up to maxJobs due jobs of the device establishment to the
// device of p, waiting up to wait (<= MaxWait) when none is due. It also
// updates last_seen_at. A device that is not the active bridge gets
// ErrNotActiveBridge. A cancelled ctx (client gone) returns no jobs: jobs
// leased just before are delivered again when their lease expires.
func (s *Service) Poll(ctx context.Context, p *auth.Principal, wait time.Duration, maxJobs int) ([]Job, error) {
	es, err := s.deviceScope(ctx, p)
	if err != nil {
		return nil, err
	}
	wait = min(max(wait, 0), MaxWait)
	if maxJobs <= 0 {
		maxJobs = DefaultMaxJobs
	}
	maxJobs = min(maxJobs, MaxJobsLimit)
	if err := s.touch(ctx, es, p.DeviceID); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait) // wall clock: a real wait, not domain time
	for {
		// Subscribe before reading, so an Enqueue between the read and the
		// wait is not missed.
		wake := s.notify.subscribe(es.ID())
		now := s.now()
		rows, err := store.LeaseFiscalBridgeJobs(ctx, s.d.DB, es, p.DeviceID, now, now.Add(LeaseDuration), maxJobs, MaxAttempts)
		if err != nil {
			if ctx.Err() != nil {
				return []Job{}, nil
			}
			return nil, fmt.Errorf("fiscalbridge: poll: %w", err)
		}
		if len(rows) > 0 {
			return jobsFromRows(rows)
		}
		left := time.Until(deadline)
		if left <= 0 {
			return []Job{}, nil
		}
		t := time.NewTimer(min(left, s.d.Recheck))
		select {
		case <-ctx.Done():
			t.Stop()
			return []Job{}, nil
		case <-wake:
			t.Stop()
		case <-t.C:
		}
		// Still the active bridge? (another device may have taken over)
		if err := s.touch(ctx, es, p.DeviceID); err != nil {
			return nil, err
		}
	}
}

func (s *Service) touch(ctx context.Context, es store.EstablishmentScope, deviceID string) error {
	ok, err := store.TouchFiscalBridge(ctx, s.d.DB, es, deviceID, s.now())
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("fiscalbridge: %w", err)
	}
	if !ok {
		return ErrNotActiveBridge
	}
	return nil
}

func jobsFromRows(rows []store.FiscalBridgeJob) ([]Job, error) {
	out := make([]Job, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		j := Job{ID: r.ID, Kind: JobKind(r.Kind), Attempt: r.Attempts}
		if r.LeaseExpiresAt != nil {
			j.LeaseExpiresAt = r.LeaseExpiresAt.UTC()
		}
		switch j.Kind {
		case KindIssue:
			var d Document
			if err := r.Payload.Unmarshal(&d); err != nil {
				return nil, fmt.Errorf("fiscalbridge: job %s payload: %w", r.ID, err)
			}
			j.Document = &d
		case KindVoid:
			var v voidPayload
			if err := r.Payload.Unmarshal(&v); err != nil {
				return nil, fmt.Errorf("fiscalbridge: job %s payload: %w", r.ID, err)
			}
			j.Ref = v.Ref
		}
		out = append(out, j)
	}
	return out, nil
}

type voidPayload struct {
	Ref string `json:"ref"`
}

// Submit stores the result of jobID leased to the device of p. It is
// idempotent: a duplicate of the stored outcome (agent retry after a lost
// response, reconnect) succeeds without a second transition;
// ErrLeaseLost when the job was completed differently or belongs to
// another device. Audit "fiscal_bridge_job.<status>" on the transition.
func (s *Service) Submit(ctx context.Context, p *auth.Principal, jobID string, res Result) error {
	es, err := s.deviceScope(ctx, p)
	if err != nil {
		return err
	}
	if res, err = res.Validate(); err != nil {
		return err
	}
	job, err := store.GetFiscalBridgeJob(ctx, s.d.DB, es, jobID)
	if err != nil {
		return fmt.Errorf("fiscalbridge: submit: %w", err)
	}
	tr := Decide(job.Attempts, res)
	resJSON, err := store.MarshalJSONValue(res)
	if err != nil {
		return err
	}
	now := s.now()
	var applied bool
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ok, err := store.CompleteFiscalBridgeJob(ctx, tx, es, jobID, p.DeviceID, store.FiscalBridgeJobOutcome{
			Status: tr.Status, Result: resJSON, NextAttemptAt: now.Add(tr.RetryIn),
		}, now)
		if err != nil || !ok {
			return err
		}
		applied = true
		diff := map[string]any{"kind": job.Kind, "attempt": job.Attempts, "outcome": res.Outcome}
		if res.Number != "" {
			diff["number"] = res.Number
		}
		if res.Error != "" {
			diff["error"] = res.Error
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "fiscal_bridge_job." + tr.Status,
			EntityType: "fiscal_bridge_job", EntityID: jobID, Diff: diff,
		})
	})
	if err != nil {
		return fmt.Errorf("fiscalbridge: submit: %w", err)
	}
	if applied {
		return nil
	}
	// No transition: duplicate or stale. Compare with what is stored.
	cur, err := store.GetFiscalBridgeJob(ctx, s.d.DB, es, jobID)
	if err != nil {
		return fmt.Errorf("fiscalbridge: submit: %w", err)
	}
	var stored Result
	if len(cur.Result) > 0 {
		if err := cur.Result.Unmarshal(&stored); err == nil && res.SameAs(stored) {
			return nil
		}
	}
	return ErrLeaseLost
}

// Status returns the bridge state of establishmentID for the panel
// (auth.PermReportsRead or auth.PermCashOperate).
func (s *Service) Status(ctx context.Context, p *auth.Principal, establishmentID string) (Status, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermReportsRead)
	if errors.Is(err, auth.ErrForbidden) {
		es, err = p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	}
	if err != nil {
		return Status{}, err
	}
	return s.status(ctx, es)
}

func (s *Service) status(ctx context.Context, es store.EstablishmentScope) (Status, error) {
	var st Status
	pending, failed, err := store.CountFiscalBridgeJobs(ctx, s.d.DB, es)
	if err != nil {
		return st, fmt.Errorf("fiscalbridge: status: %w", err)
	}
	st.PendingJobs, st.FailedJobs = pending, failed
	b, name, err := store.GetActiveFiscalBridge(ctx, s.d.DB, es)
	if errors.Is(err, store.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("fiscalbridge: status: %w", err)
	}
	st.Registered, st.DeviceID, st.DeviceName, st.Driver = true, b.DeviceID, name, b.Driver
	if b.LastSeenAt != nil {
		t := b.LastSeenAt.UTC()
		st.LastSeenAt = &t
	}
	st.Online = IsOnline(st.LastSeenAt, s.now())
	if ds, ok := decodeDeviceStatus(b.DeviceStatus); ok {
		st.DeviceStatus = &ds
	}
	return st, nil
}

// IsOnline reports whether a bridge last seen at lastSeen is online at now (pure).
func IsOnline(lastSeen *time.Time, now time.Time) bool {
	return lastSeen != nil && now.Sub(*lastSeen) <= OnlineWindow
}

func decodeDeviceStatus(j store.JSON) (DeviceStatus, bool) {
	var raw map[string]json.RawMessage
	if err := j.Unmarshal(&raw); err != nil || len(raw) == 0 {
		return DeviceStatus{}, false
	}
	var ds DeviceStatus
	if err := j.Unmarshal(&ds); err != nil {
		return DeviceStatus{}, false
	}
	return ds, true
}

// Enqueue implements Queue: it inserts the job once per (establishment,
// kind, idempotency key), wakes the waiting poll and reports the stored
// result once the device has submitted a final outcome.
func (s *Service) Enqueue(ctx context.Context, establishmentID string, kind JobKind, idempotencyKey string, payload any) (string, *Result, bool, error) {
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, establishmentID)
	if err != nil {
		return "", nil, false, fmt.Errorf("fiscalbridge: enqueue: %w", err)
	}
	pl, err := store.MarshalJSONValue(payload)
	if err != nil {
		return "", nil, false, err
	}
	now := s.now()
	job := &store.FiscalBridgeJob{
		ID: store.NewID(), Kind: string(kind), IdempotencyKey: idempotencyKey, Status: store.FiscalBridgeJobQueued,
		Payload: pl, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if doc, ok := payload.(Document); ok && doc.PaymentID != "" {
		if id, err := store.FiscalDocumentIDForPayment(ctx, s.d.DB, es, doc.PaymentID); err == nil && id != "" {
			job.FiscalDocumentID = &id
		}
	}
	got, created, err := store.EnqueueFiscalBridgeJob(ctx, s.d.DB, es, job)
	if err != nil {
		return "", nil, false, fmt.Errorf("fiscalbridge: enqueue: %w", err)
	}
	if created {
		s.notify.broadcast(es.ID())
		return got.ID, nil, false, nil
	}
	switch got.Status {
	case store.FiscalBridgeJobSucceeded, store.FiscalBridgeJobFailed:
		var r Result
		if err := got.Result.Unmarshal(&r); err != nil {
			return "", nil, false, fmt.Errorf("fiscalbridge: job %s result: %w", got.ID, err)
		}
		if got.Status == store.FiscalBridgeJobFailed {
			r.Outcome = OutcomeFailed // also when exhausted without a device answer
		}
		return got.ID, &r, true, nil
	}
	return got.ID, nil, false, nil
}

// BridgeStatus implements Queue: the last status reported by the active
// bridge, offline when it has not been seen within OnlineWindow.
func (s *Service) BridgeStatus(ctx context.Context, establishmentID string) (DeviceStatus, error) {
	if establishmentID == "" {
		return DeviceStatus{}, ErrNoBridge
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, establishmentID)
	if err != nil {
		return DeviceStatus{}, err
	}
	b, _, err := store.GetActiveFiscalBridge(ctx, s.d.DB, es)
	if errors.Is(err, store.ErrNotFound) {
		return DeviceStatus{}, ErrNoBridge
	}
	if err != nil {
		return DeviceStatus{}, err
	}
	ds, ok := decodeDeviceStatus(b.DeviceStatus)
	if !ok {
		ds = DeviceStatus{Online: true}
	}
	if !IsOnline(b.LastSeenAt, s.now()) {
		ds.Online, ds.Message = false, "ponte fiscale non raggiungibile"
	}
	return ds, nil
}

// DeactivateDevice implements device.BridgeRevoker (runs in the device
// revocation transaction, which writes its own audit entry).
func (s *Service) DeactivateDevice(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, deviceID string) error {
	if _, err := store.DeactivateFiscalBridgeOfDevice(ctx, tx, es, deviceID, s.now()); err != nil {
		return fmt.Errorf("fiscalbridge: deactivate device: %w", err)
	}
	return nil
}

// RequeueExpiredLeases returns jobs whose lease expired to the queue, and
// fails those delivered MaxAttempts times without an answer (a job that
// crashes the agent must not loop forever).
func (s *Service) RequeueExpiredLeases(ctx context.Context) error {
	exhausted, err := store.MarshalJSONValue(Result{
		Outcome: OutcomeFailed, Error: fmt.Sprintf("nessun esito dal ponte fiscale dopo %d tentativi", MaxAttempts),
	})
	if err != nil {
		return err
	}
	requeued, failed, err := store.RequeueExpiredFiscalBridgeLeases(ctx, s.d.DB, s.now(), MaxAttempts, exhausted)
	if err != nil {
		return fmt.Errorf("fiscalbridge: %w", err)
	}
	if failed > 0 {
		s.d.Log.WarnContext(ctx, "fiscal bridge jobs failed without an answer", slog.Int64("count", failed))
	}
	if requeued > 0 {
		s.notify.broadcastAll()
	}
	return nil
}

// Jobs are the background jobs of the bridge.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: "fiscalbridge.requeue_expired_leases", Interval: 30 * time.Second, Run: s.RequeueExpiredLeases}}
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }
