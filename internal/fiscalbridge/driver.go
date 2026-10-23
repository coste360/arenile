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

// establishmentKey is the context key of WithEstablishment.
type establishmentKey struct{}

// WithEstablishment returns a context that carries the establishment of a
// Void or Status call: ext.FiscalDriver has no establishment parameter on
// those operations, and the bridge queue is per establishment. Callers in
// the fiscal domain set it before calling the driver; the app wires
// EstablishmentFromContext into the registered Driver.
func WithEstablishment(ctx context.Context, establishmentID string) context.Context {
	return context.WithValue(ctx, establishmentKey{}, establishmentID)
}

// EstablishmentFromContext returns the establishment set by
// WithEstablishment, or "".
func EstablishmentFromContext(ctx context.Context) string {
	id, _ := ctx.Value(establishmentKey{}).(string)
	return id
}

// ErrNoEstablishment: Void or Status called without WithEstablishment.
var ErrNoEstablishment = errors.New("fiscalbridge: the establishment is not in the context (fiscalbridge.WithEstablishment)")

// Driver is the "bridge" ext.FiscalDriver (strategy registered in the
// app.Registry). It never blocks on the device: see DeferredError.
type Driver struct {
	Queue Queue
	// EstablishmentFromContext resolves the establishment for Void/Status
	// (ext.FiscalDriver has no establishment parameter there). The app wires
	// fiscalbridge.EstablishmentFromContext; nil means "never set".
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
		return ErrNoEstablishment
	}
	if ref == "" {
		return errors.New("fiscalbridge: void without document reference")
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
	if est == "" {
		return ext.DeviceStatus{}, ErrNoEstablishment
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
