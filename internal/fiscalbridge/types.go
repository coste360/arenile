package fiscalbridge

import (
	"encoding/json"
	"time"

	"github.com/coste360/arenile/pkg/ext"
)

// Code is the fiscal driver code of the bridge (settings.fiscal.driver).
const Code = "bridge"

// Queue tuning.
const (
	LeaseDuration = 60 * time.Second
	MaxAttempts   = 5
	// OnlineWindow: a bridge seen within this window is online.
	OnlineWindow = 60 * time.Second
	// MaxWait is the longest long-poll (below the server WriteTimeout).
	MaxWait = 25 * time.Second
)

// Line mirrors ext.FiscalLine (gross cents, VAT in basis points).
type Line struct {
	Description string `json:"description"`
	Quantity    int64  `json:"quantity"`
	UnitGross   int64  `json:"unit_gross"`
	Gross       int64  `json:"gross"`
	VATRateBP   int64  `json:"vat_rate_bp"`
}

// Payment mirrors ext.FiscalPayment.
type Payment struct {
	Method string `json:"method"`
	Amount int64  `json:"amount"`
}

// Document is the payload of an issue job (api.FiscalBridgeDocument).
type Document struct {
	EstablishmentID string    `json:"establishment_id"`
	BookingID       string    `json:"booking_id,omitempty"`
	PaymentID       string    `json:"payment_id,omitempty"`
	Currency        string    `json:"currency"`
	Lines           []Line    `json:"lines"`
	Payments        []Payment `json:"payments"`
	Total           int64     `json:"total"`
	IssuedAt        time.Time `json:"issued_at"`
	IdempotencyKey  string    `json:"idempotency_key"`
}

// DocumentFromRequest maps a driver request to the wire document (pure).
func DocumentFromRequest(r ext.FiscalDocumentRequest) Document {
	d := Document{
		EstablishmentID: r.EstablishmentID, BookingID: r.BookingID, PaymentID: r.PaymentID,
		Currency: r.Currency, Total: r.Total, IssuedAt: r.IssuedAt.UTC(), IdempotencyKey: r.IdempotencyKey,
		Lines: make([]Line, len(r.Lines)), Payments: make([]Payment, len(r.Payments)),
	}
	for i, l := range r.Lines {
		d.Lines[i] = Line{Description: l.Description, Quantity: l.Quantity, UnitGross: l.UnitGross, Gross: l.Gross, VATRateBP: l.VATRateBP}
	}
	for i, p := range r.Payments {
		d.Payments[i] = Payment{Method: p.Method, Amount: p.Amount}
	}
	return d
}

// Request maps the wire document back to a driver request (agent side).
func (d Document) Request() ext.FiscalDocumentRequest {
	r := ext.FiscalDocumentRequest{
		EstablishmentID: d.EstablishmentID, BookingID: d.BookingID, PaymentID: d.PaymentID,
		Currency: d.Currency, Total: d.Total, IssuedAt: d.IssuedAt, IdempotencyKey: d.IdempotencyKey,
		Lines: make([]ext.FiscalLine, len(d.Lines)), Payments: make([]ext.FiscalPayment, len(d.Payments)),
	}
	for i, l := range d.Lines {
		r.Lines[i] = ext.FiscalLine{Description: l.Description, Quantity: l.Quantity, UnitGross: l.UnitGross, Gross: l.Gross, VATRateBP: l.VATRateBP}
	}
	for i, p := range d.Payments {
		r.Payments[i] = ext.FiscalPayment{Method: p.Method, Amount: p.Amount}
	}
	return r
}

// JobKind is the operation of a job (store.FiscalBridgeJob*).
type JobKind string

// Job kinds.
const (
	KindIssue JobKind = "issue"
	KindVoid  JobKind = "void"
)

// Job is a leased job as delivered to the device (api.FiscalBridgeJob).
type Job struct {
	ID             string
	Kind           JobKind
	Attempt        int
	LeaseExpiresAt time.Time
	Document       *Document // KindIssue
	Ref            string    // KindVoid
}

// Outcome of a job.
type Outcome string

// Outcomes (api.FiscalBridgeJobResultOutcome).
const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
)

// Result is what the device submits (api.FiscalBridgeJobResult).
type Result struct {
	Outcome   Outcome         `json:"outcome"`
	Ref       string          `json:"ref,omitempty"`
	Number    string          `json:"number,omitempty"`
	IssuedAt  *time.Time      `json:"issued_at,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	Error     string          `json:"error,omitempty"`
	Retryable bool            `json:"retryable,omitempty"`
}

// DeviceStatus is the state of the local fiscal device reported by the
// bridge (api.FiscalDeviceStatus).
type DeviceStatus struct {
	Online    bool      `json:"online"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

// Registration is the request of Service.Register.
type Registration struct {
	Driver       string
	DeviceStatus *DeviceStatus
}

// Status is the bridge state of an establishment (api.FiscalBridgeStatus).
type Status struct {
	Registered   bool
	DeviceID     string
	DeviceName   string
	Driver       string
	Online       bool
	LastSeenAt   *time.Time
	DeviceStatus *DeviceStatus
	PendingJobs  int
	FailedJobs   int
}

// Retry tuning of retryable failures reported by the device.
const (
	retryBase = 15 * time.Second
	retryMax  = 10 * time.Minute
	// MaxRawBytes bounds Result.Raw (api.FiscalBridgeJobResult.raw).
	MaxRawBytes = 16 << 10
	// MaxErrorLen bounds Result.Error (characters kept).
	MaxErrorLen = 500
)

// Backoff is the delay before a job that failed with a retryable error at
// delivery attempt is offered again: 15 s doubling up to 10 minutes (pure,
// deterministic so that tests and operators can predict it).
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := retryBase
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= retryMax {
			return retryMax
		}
	}
	return d
}

// Transition is the job state decided for a submitted result.
type Transition struct {
	Status  string // store.FiscalBridgeJob{Succeeded,Failed,Queued}
	RetryIn time.Duration
}

// Decide maps a result submitted for a job delivered attempts times to
// the next job state (functional core of Service.Submit): success is
// final; a retryable failure goes back to the queue with Backoff until
// MaxAttempts deliveries, then it is final like any other failure.
func Decide(attempts int, r Result) Transition {
	switch {
	case r.Outcome == OutcomeSucceeded:
		return Transition{Status: statusSucceeded}
	case r.Retryable && attempts < MaxAttempts:
		return Transition{Status: statusQueued, RetryIn: Backoff(attempts)}
	default:
		return Transition{Status: statusFailed}
	}
}

// Store statuses mirrored here to keep Decide free of the store import.
const (
	statusQueued    = "queued"
	statusSucceeded = "succeeded"
	statusFailed    = "failed"
)

// Validate checks a submitted result (outcome, sizes) and trims the error
// text. It returns the normalised result.
func (r Result) Validate() (Result, error) {
	switch r.Outcome {
	case OutcomeSucceeded, OutcomeFailed:
	default:
		return r, errInvalid("outcome", "deve essere succeeded o failed")
	}
	if len(r.Raw) > MaxRawBytes {
		return r, errInvalid("raw", "supera 16 KiB")
	}
	if len(r.Raw) > 0 && !json.Valid(r.Raw) {
		return r, errInvalid("raw", "JSON non valido")
	}
	if rs := []rune(r.Error); len(rs) > MaxErrorLen {
		r.Error = string(rs[:MaxErrorLen])
	}
	if r.Outcome == OutcomeSucceeded {
		r.Retryable = false
	}
	if r.IssuedAt != nil {
		t := r.IssuedAt.UTC()
		r.IssuedAt = &t
	}
	return r, nil
}

// SameAs reports whether r is a duplicate submission of stored (the fields
// that identify an outcome; Raw and timestamps may be re-serialised).
func (r Result) SameAs(stored Result) bool {
	return r.Outcome == stored.Outcome && r.Ref == stored.Ref && r.Number == stored.Number &&
		r.Error == stored.Error && r.Retryable == stored.Retryable
}
