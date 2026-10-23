package fiscalbridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coste360/arenile/pkg/ext"
)

// DeferredError tells the fiscal module that the document was accepted by
// the bridge queue but its outcome is not known yet: the document stays
// pending and is retried by the next fiscal.issue_pending run. It
// implements the `interface{ Deferred() bool }` checked by
// internal/domain/fiscal.
type DeferredError struct {
	JobID string
}

func (e *DeferredError) Error() string {
	return "fiscalbridge: awaiting bridge outcome for job " + e.JobID
}

// Deferred implements the fiscal deferral capability.
func (*DeferredError) Deferred() bool { return true }

// ErrNoBridge: no active bridge for the establishment (Status only; jobs
// still queue and wait for a bridge).
var ErrNoBridge = errors.New("fiscalbridge: no active bridge")

// Queue is the part of Service used by Driver (consumer-declared port).
type Queue interface {
	// Enqueue inserts the job if (establishment, kind, idempotency key) is
	// new and returns its id and, when the device has already submitted it,
	// the final result (done=true).
	Enqueue(ctx context.Context, establishmentID string, kind JobKind, idempotencyKey string, payload any) (jobID string, res *Result, done bool, err error)
	// DeviceStatus returns the last status reported by the active bridge.
	BridgeStatus(ctx context.Context, establishmentID string) (DeviceStatus, error)
}

// Driver is the "bridge" ext.FiscalDriver (strategy registered in the
// app.Registry). It never blocks on the device: see DeferredError.
type Driver struct {
	Queue Queue
	// EstablishmentFromContext, when set, resolves the establishment for
	// Void/Status (ext.FiscalDriver has no establishment parameter there).
	EstablishmentFromContext func(ctx context.Context) string
}

var _ ext.FiscalDriver = (*Driver)(nil)

// Code implements ext.FiscalDriver.
func (*Driver) Code() string { return Code }

// Issue implements ext.FiscalDriver.
func (d *Driver) Issue(ctx context.Context, req ext.FiscalDocumentRequest) (*ext.FiscalDocumentResult, error) {
	if req.IdempotencyKey == "" || req.EstablishmentID == "" {
		return nil, errors.New("fiscalbridge: request without establishment or idempotency key")
	}
	id, res, done, err := d.Queue.Enqueue(ctx, req.EstablishmentID, KindIssue, req.IdempotencyKey, DocumentFromRequest(req))
	if err != nil {
		return nil, fmt.Errorf("fiscalbridge: enqueue: %w", err)
	}
	if !done {
		return nil, &DeferredError{JobID: id}
	}
	return resultToExt(res)
}

// Void implements ext.FiscalDriver.
func (d *Driver) Void(ctx context.Context, ref string) error {
	est := ""
	if d.EstablishmentFromContext != nil {
		est = d.EstablishmentFromContext(ctx)
	}
	if est == "" {
		return errors.New("fiscalbridge: void needs the establishment in the context")
	}
	id, res, done, err := d.Queue.Enqueue(ctx, est, KindVoid, "void:"+ref, map[string]string{"ref": ref})
	if err != nil {
		return fmt.Errorf("fiscalbridge: enqueue void: %w", err)
	}
	if !done {
		return &DeferredError{JobID: id}
	}
	_, err = resultToExt(res)
	return err
}

// Status implements ext.FiscalDriver.
func (d *Driver) Status(ctx context.Context) (ext.DeviceStatus, error) {
	est := ""
	if d.EstablishmentFromContext != nil {
		est = d.EstablishmentFromContext(ctx)
	}
	st, err := d.Queue.BridgeStatus(ctx, est)
	if err != nil {
		return ext.DeviceStatus{}, err
	}
	return ext.DeviceStatus{Online: st.Online, Message: st.Message, CheckedAt: st.CheckedAt}, nil
}

func resultToExt(r *Result) (*ext.FiscalDocumentResult, error) {
	if r == nil {
		return nil, errors.New("fiscalbridge: missing result")
	}
	if r.Outcome != OutcomeSucceeded {
		msg := r.Error
		if msg == "" {
			msg = "failed on the bridge device"
		}
		return nil, fmt.Errorf("fiscalbridge: %s", msg)
	}
	issued := time.Time{}
	if r.IssuedAt != nil {
		issued = r.IssuedAt.UTC()
	}
	return &ext.FiscalDocumentResult{Ref: r.Ref, Number: r.Number, IssuedAt: issued, Raw: r.Raw}, nil
}
