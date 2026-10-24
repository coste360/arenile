package printing

import (
	"context"
	"errors"
	"time"
)

// Domain errors, mapped to problem types by the HTTP layer.
var (
	// ErrUnavailable: ESC/POS printing needs the desktop edition (409 printing_unavailable).
	ErrUnavailable = errors.New("printing: escpos printing is available only in the desktop edition")
	// ErrAddressNotAllowed: the printer host is not a private/LAN address (422 printer_address_not_allowed).
	ErrAddressNotAllowed = errors.New("printing: printer address must be a private network address")
	// ErrRouteMissing: no routing rule and no explicit printer for the document (409 print_route_missing).
	ErrRouteMissing = errors.New("printing: no printer routed for this document")
	// ErrPrinterInactive: the target printer is deactivated (409 conflict).
	ErrPrinterInactive = errors.New("printing: printer is inactive")
	// ErrNotFound: printer, job, register or document not found in the establishment (404).
	ErrNotFound = errors.New("printing: not found")
	// ErrInvalidTransition: retry of a job that is not failed (409 invalid_transition).
	ErrInvalidTransition = errors.New("printing: invalid print job transition")
	// ErrIdempotencyKeyReused: the Idempotency-Key of an existing job sent
	// for another document or printer (422 idempotency_key_reused).
	ErrIdempotencyKeyReused = errors.New("printing: idempotency key reused for another request")
)

// Target identifies the printer a Driver talks to.
type Target struct {
	Host string
	Port int
}

// Status is the outcome of a reachability check.
type Status struct {
	Reachable bool
	// PaperLow and CoverOpen are set when the printer answers DLE EOT.
	PaperLow  bool
	CoverOpen bool
	Latency   time.Duration
}

// Driver delivers a rendered ESC/POS payload to a printer. Implementations
// must honour ctx deadlines, never log the payload and return errors whose
// text contains no personal data.
type Driver interface {
	// Name is the printers.driver value served by this driver.
	Name() string
	// Send writes the whole payload (all copies already included) or fails.
	Send(ctx context.Context, t Target, payload []byte) error
	// Check probes the printer without printing.
	Check(ctx context.Context, t Target) (Status, error)
}
