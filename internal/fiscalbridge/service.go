package fiscalbridge

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// ErrNotImplemented: scaffolding, the operation is not built yet (501).
var ErrNotImplemented = configerr.ErrNotImplemented

// Deps are the dependencies of Service.
type Deps struct {
	DB    *store.DB
	Clock clock.Clock
	Audit *audit.Writer
	Log   *slog.Logger
}

// Service is the instance side of the fiscal bridge. It implements Queue
// (for Driver) and device.BridgeRevoker. Waiting long-polls are woken by an
// in-process notifier on Enqueue (single instance; multi-instance
// deployments fall back to the poll interval).
type Service struct{ d Deps }

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// Register makes the device of p the active bridge of its establishment
// (deactivating any other) or refreshes its driver and device status.
// Requires a device principal with auth.PermCashOperate.
func (s *Service) Register(ctx context.Context, p *auth.Principal, reg Registration) (Status, error) {
	return Status{}, ErrNotImplemented
}

// Unregister deactivates the registration of the device of p (idempotent).
func (s *Service) Unregister(ctx context.Context, p *auth.Principal) error {
	return ErrNotImplemented
}

// Poll leases up to maxJobs due jobs of the device establishment to the
// device of p, waiting up to wait (<= MaxWait) when none is due. It also
// updates last_seen_at. A device that is not the active bridge gets
// configerr.ErrConflict.
func (s *Service) Poll(ctx context.Context, p *auth.Principal, wait time.Duration, maxJobs int) ([]Job, error) {
	return nil, ErrNotImplemented
}

// Submit stores the result of jobID leased to the device of p (idempotent
// for the same outcome; configerr.ErrConflict when the lease was lost).
func (s *Service) Submit(ctx context.Context, p *auth.Principal, jobID string, res Result) error {
	return ErrNotImplemented
}

// Status returns the bridge state of establishmentID for the panel.
func (s *Service) Status(ctx context.Context, p *auth.Principal, establishmentID string) (Status, error) {
	return Status{}, ErrNotImplemented
}

// Enqueue implements Queue.
func (s *Service) Enqueue(ctx context.Context, establishmentID string, kind JobKind, idempotencyKey string, payload any) (string, *Result, bool, error) {
	return "", nil, false, ErrNotImplemented
}

// BridgeStatus implements Queue.
func (s *Service) BridgeStatus(ctx context.Context, establishmentID string) (DeviceStatus, error) {
	return DeviceStatus{}, ErrNoBridge
}

// DeactivateDevice implements device.BridgeRevoker (runs in the device
// revocation transaction).
func (s *Service) DeactivateDevice(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, deviceID string) error {
	return nil
}

// RequeueExpiredLeases returns jobs whose lease expired to the queue.
func (s *Service) RequeueExpiredLeases(ctx context.Context) error {
	return nil
}

// Jobs are the background jobs of the bridge.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: "fiscalbridge.requeue_expired_leases", Interval: 30 * time.Second, Run: s.RequeueExpiredLeases}}
}
