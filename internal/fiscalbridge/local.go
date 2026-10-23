package fiscalbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/pkg/ext"
)

// Local drivers of the agent (desktop side): the FiscalDriver that talks to
// the printer / registratore telematico attached to the cash desk PC.
// Strategy + registry: the launcher builds the driver chosen in the remote
// settings by code; real drivers (one per protocol family, spec 14.2)
// register a factory here or are passed to the agent directly.

// NoopCode is the code of NoopDriver.
const NoopCode = "noop"

// LocalDriverFactory builds a local driver from its JSON configuration
// (serial port, IP of the register...).
type LocalDriverFactory func(cfg json.RawMessage, c clock.Clock) (ext.FiscalDriver, error)

// LocalDrivers is a registry of local driver factories (safe for
// concurrent use).
type LocalDrivers struct {
	mu sync.RWMutex
	m  map[string]LocalDriverFactory
}

// ErrUnknownLocalDriver: no factory for the code.
var ErrUnknownLocalDriver = errors.New("fiscalbridge: unknown local fiscal driver")

// NewLocalDrivers returns a registry with the built-in drivers (noop).
func NewLocalDrivers() *LocalDrivers {
	r := &LocalDrivers{m: map[string]LocalDriverFactory{}}
	r.Register(NoopCode, func(_ json.RawMessage, c clock.Clock) (ext.FiscalDriver, error) {
		return NoopDriver{Clock: c}, nil
	})
	return r
}

// Register adds or replaces the factory of code.
func (r *LocalDrivers) Register(code string, f LocalDriverFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[code] = f
}

// New builds the driver code with cfg.
func (r *LocalDrivers) New(code string, cfg json.RawMessage, c clock.Clock) (ext.FiscalDriver, error) {
	r.mu.RLock()
	f, ok := r.m[code]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownLocalDriver, code)
	}
	if c == nil {
		c = clock.System{}
	}
	return f(cfg, c)
}

// Codes lists the registered codes, sorted.
func (r *LocalDrivers) Codes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.m))
	for k := range r.m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// NoopDriver acknowledges every document without a device: the outcome is
// tracked in Arenile and the operator issues the receipt on their own
// register (the same meaning as the "none" driver of the server). Its Ref
// is derived from the idempotency key, so a duplicate delivery yields the
// same outcome. It is the default until a real driver is configured.
type NoopDriver struct{ Clock clock.Clock }

var _ ext.FiscalDriver = NoopDriver{}

// Code implements ext.FiscalDriver.
func (NoopDriver) Code() string { return NoopCode }

// Issue implements ext.FiscalDriver.
func (n NoopDriver) Issue(_ context.Context, req ext.FiscalDocumentRequest) (*ext.FiscalDocumentResult, error) {
	return &ext.FiscalDocumentResult{Ref: "noop_" + req.IdempotencyKey, IssuedAt: n.now()}, nil
}

// Void implements ext.FiscalDriver.
func (NoopDriver) Void(context.Context, string) error { return nil }

// Status implements ext.FiscalDriver.
func (n NoopDriver) Status(context.Context) (ext.DeviceStatus, error) {
	return ext.DeviceStatus{Online: true, Message: "nessun dispositivo fiscale collegato", CheckedAt: n.now()}, nil
}

func (n NoopDriver) now() time.Time {
	if n.Clock == nil {
		return time.Now().UTC()
	}
	return n.Clock.Now().UTC()
}

// RetryableError marks a local driver error as temporary (device off,
// paper out, cable unplugged): the instance queues the job again with
// backoff instead of failing the document.
type RetryableError struct{ Err error }

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

// Retryable implements the retry capability checked by the agent.
func (*RetryableError) Retryable() bool { return true }

// Retryable wraps err as a RetryableError (nil stays nil).
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &RetryableError{Err: err}
}

// IsRetryable reports whether a local driver error is temporary: it
// implements Retryable() bool or Temporary() bool returning true, or it is
// a timeout of the driver call.
func IsRetryable(err error) bool {
	var r interface{ Retryable() bool }
	if errors.As(err, &r) {
		return r.Retryable()
	}
	var t interface{ Temporary() bool }
	if errors.As(err, &t) {
		return t.Temporary()
	}
	return errors.Is(err, context.DeadlineExceeded)
}
