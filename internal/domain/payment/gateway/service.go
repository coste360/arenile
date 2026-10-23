package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Defaults of the reconciliation (spec 13.3).
const (
	DefaultReconcileInterval = 15 * time.Minute
	DefaultPendingAfter      = 10 * time.Minute
	// MaxWebhookBody bounds the raw webhook body read before verification.
	MaxWebhookBody = 256 << 10
	// reconcileBatch bounds the intents looked at by one run.
	reconcileBatch = 200
	// refundRetryAfter is the age after which a refund never sent to the
	// provider (crash, provider down) is retried by the reconciliation.
	refundRetryAfter = time.Minute
	// reportLimit bounds the rows of the reconciliation report and
	// reportLiveChecks the provider lookups it makes.
	reportLimit      = 500
	reportLiveChecks = 25
	// unknownRefRetryWindow: a recent event for a reference not (yet)
	// known is answered 503 so the provider retries (the transaction that
	// stores the reference may not have committed).
	unknownRefRetryWindow = time.Hour
	maxRawEventIDs        = 50
)

// Metadata keys shared with payment drivers (the core Stripe driver uses
// the same literals: stripe.Hint* / stripe.Meta*). Hints in
// ext.CreatePaymentRequest.Metadata are read by the driver and not
// forwarded; Meta* keys of ext.PaymentEvent.Metadata are set by the driver.
const (
	MetaLocale         = "locale"     // hint: customer locale ("it", "en")
	MetaExpiresAt      = "expires_at" // hint: RFC 3339 end of the booking hold
	MetaIntentID       = "intent_id"  // forwarded: payment_intents id
	MetaPurpose        = "purpose"    // forwarded: full, deposit, balance
	MetaOutcome        = "outcome"
	MetaPaymentIntent  = "payment_intent"
	MetaAmountCaptured = "amount_captured"
)

// Outcomes (ext.PaymentEvent.Metadata[MetaOutcome]) refining the event type.
const (
	OutcomeExpired       = "expired"
	OutcomeAttemptFailed = "attempt_failed"
	OutcomeRefundFailed  = "refund_failed"
	OutcomeRefundTotal   = "refund_total"
	OutcomeRefundSingle  = "refund_single"
)

// EventRefundScheduled is the outbox event (payload event.PaymentChange,
// version 1, status pending, direction refund) written when the gateway
// schedules an automatic refund: late payment on a booking whose resources
// are gone, or an online payment above the booking total. Notify sends the
// customer the "rimborso in corso" message from it.
const EventRefundScheduled = "payment.refund_scheduled"

// Reconciliation issues (api.ReconciliationIssue).
const (
	IssueNone                = "none"
	IssuePendingTooLong      = "pending_too_long"
	IssueStatusMismatch      = "status_mismatch"
	IssueMissingLedgerRow    = "missing_ledger_row"
	IssueAmountMismatch      = "amount_mismatch"
	IssueProviderUnreachable = "provider_unreachable"
)

// placeholderPrefix marks the external_id of a refund intent not yet sent
// to the provider (external_id is NOT NULL and unique per provider).
const placeholderPrefix = "pending:"

// Errors of the gateway (mapped by internal/http/problem).
var (
	// ErrProviderUnavailable: the provider could not be reached or answered
	// with a server error (503 payment_provider_unavailable).
	ErrProviderUnavailable = errors.New("gateway: payment provider unavailable")
	// ErrSignatureInvalid: webhook signature missing or wrong (400
	// webhook_signature_invalid). Wraps ext.ErrInvalidSignature.
	ErrSignatureInvalid = fmt.Errorf("gateway: %w", ext.ErrInvalidSignature)
	// ErrProviderNotConfigured: no credentials for the provider of the
	// active configuration (409 for payments and refunds).
	ErrProviderNotConfigured = fmt.Errorf("%w: nessun provider di pagamento impostato", configerr.ErrConflict)
	// ErrWebhookTarget: the establishment or the provider of a webhook URL
	// does not exist or was never configured (404, no details).
	ErrWebhookTarget = fmt.Errorf("%w: webhook target", configerr.ErrNotFound)
	// ErrRefundNotSupported: the provider of the payment cannot refund.
	ErrRefundNotSupported = fmt.Errorf("%w: il provider non supporta i rimborsi", configerr.ErrConflict)
	// ErrNotOnlinePayment: the refunded payment was not made online.
	ErrNotOnlinePayment = configerr.Invalid("payment_id", "Il pagamento scelto non è un pagamento online rimborsabile tramite il provider.")
	// ErrIdempotencyMismatch: the key was already used for another refund.
	ErrIdempotencyMismatch = fmt.Errorf("%w: chiave di idempotenza già usata per un altro rimborso", configerr.ErrConflict)
	// ErrProviderRefused: the provider refused the request (4xx); the
	// short provider code is appended.
	ErrProviderRefused = fmt.Errorf("%w: il provider di pagamento ha rifiutato la richiesta", configerr.ErrConflict)
	// errRetryLater rolls back a webhook whose reference is not known yet;
	// HandleWebhook answers 503 so the provider retries.
	errRetryLater = fmt.Errorf("%w: reference not known yet", ErrProviderUnavailable)
	// errDuplicate rolls back the transaction of an already processed event.
	errDuplicate = errors.New("gateway: duplicate event")
)

// ---- ports (implemented elsewhere, asserted in internal/app/m4.go) -------

// ProviderSettings returns decrypted provider configurations. Implemented
// by *online.Service.
type ProviderSettings interface {
	// ActiveProvider is the provider of the public page of es ("manual"
	// when nothing online is active).
	ActiveProvider(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*online.ProviderConfig, error)
	// ProviderConfig is the configuration of provider for es, active or not;
	// store.ErrNotFound when never configured.
	ProviderConfig(ctx context.Context, db bun.IDB, es store.EstablishmentScope, provider string) (*online.ProviderConfig, error)
}

// Ledger writes the ledger rows of provider outcomes in the caller's tx.
// Implemented by *payment.Service (provider.go).
type Ledger interface {
	RecordProviderPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in payment.ProviderPayment) (*payment.Result, error)
	RecordProviderRefund(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in payment.ProviderRefund) (*store.Payment, error)
}

// Providers resolves payment drivers by code. Implemented by *app.Registry.
type Providers interface {
	PaymentProvider(code string) (ext.PaymentProvider, bool)
}

// LateConfirmer is the OPTIONAL booking-side path of a late payment (ADR
// 0014): a provider payment that succeeds after the booking hold expired.
// It must re-allocate the resources of the expired booking through the
// inventory (DB constraints decide) and confirm it, with audit and outbox,
// inside tx; any error (resources taken, transition refused) means "not
// possible" and the gateway refunds the payment instead. The gateway calls
// it inside a savepoint, so a failure leaves tx usable. Implemented by the
// booking service when available (internal/app wires it); without it every
// late payment is refunded.
type LateConfirmer interface {
	ConfirmLatePayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, pm *store.Payment) error
}

// PaymentStatusChecker is an OPTIONAL capability of a payment driver, used
// by reconciliation to fetch the current outcome of a provider payment or
// refund (see doc.go). It returns the normalised event the provider would
// have sent (ProviderEventID may be empty; the gateway then uses a
// synthetic id), or nil when the payment is still pending.
type PaymentStatusChecker interface {
	PaymentStatus(ctx context.Context, providerRef string, creds ext.Credentials) (*ext.PaymentEvent, error)
}

// CredentialsVerifier is an OPTIONAL capability used by "test payment
// settings": it checks the credentials with the provider and returns a
// short, non-secret account label.
type CredentialsVerifier interface {
	VerifyCredentials(ctx context.Context, creds ext.Credentials) (accountLabel string, err error)
}

// PaymentCanceller is an OPTIONAL capability: it closes a provider payment
// replaced by a new attempt (Stripe: expire the Checkout Session), so the
// customer cannot pay twice. An error (also for a payment no longer open,
// e.g. just paid) leaves the intent untouched: the webhook or the
// reconciliation reports its real outcome.
type PaymentCanceller interface {
	CancelPayment(ctx context.Context, providerRef string, creds ext.Credentials) error
}

// Deps are the dependencies of the gateway. Every field is required except
// Log, LateConfirmer, PendingAfter and ReconcileInterval.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Providers Providers
	Settings  ProviderSettings
	Ledger    Ledger
	// LateConfirmer confirms late payments when resources are still free
	// (optional, see LateConfirmer).
	LateConfirmer LateConfirmer
	// BaseURL is config.EffectiveBaseURL() (default return and cancel URLs).
	BaseURL string
	// PendingAfter is the age after which a pending intent is reconciled.
	PendingAfter time.Duration
	// ReconcileInterval is the period of the reconciliation job.
	ReconcileInterval time.Duration
}

// Service implements the gateway use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.PendingAfter <= 0 {
		d.PendingAfter = DefaultPendingAfter
	}
	if d.ReconcileInterval <= 0 {
		d.ReconcileInterval = DefaultReconcileInterval
	}
	return &Service{d: d}
}

// SetLateConfirmer installs the late payment path after construction (the
// booking service is built after the payment services in internal/app).
func (s *Service) SetLateConfirmer(c LateConfirmer) { s.d.LateConfirmer = c }

// Purpose of a provider payment.
type Purpose string

// Purposes.
const (
	PurposeFull    Purpose = "full"    // whole total from the public page
	PurposeDeposit Purpose = "deposit" // deposit_due from the public page
	PurposeBalance Purpose = "balance" // balance from the manage area
)

// StartRequest starts a provider payment of a booking.
type StartRequest struct {
	BookingID   string
	BookingCode string
	Purpose     Purpose
	Amount      money.Cents
	Currency    string
	// Description is the line shown on the provider page (no personal data).
	Description   string
	CustomerEmail string
	Locale        string
	// ReturnURL / CancelURL are absolute URLs of the public page (the
	// return page only polls the status, it never confirms).
	ReturnURL string
	CancelURL string
}

// StartResult tells the client how to complete the payment.
type StartResult struct {
	Provider       string
	Mode           ext.PaymentMode
	RedirectURL    string
	ClientSecret   string
	PublishableKey string
	IntentID       string
	Amount         money.Cents
	ExpiresAt      *time.Time
}

// WebhookOutcome is the result of HandleWebhook.
type WebhookOutcome struct {
	// Duplicate: the event was already processed (no effect).
	Duplicate bool
	// Ignored: authentic event of a type (or for a reference) Arenile does
	// not handle.
	Ignored bool
}

// RefundRequest refunds (part of) an online payment through its provider.
type RefundRequest struct {
	// BookingID, when set, must be the booking of the payment.
	BookingID string
	// PaymentID is the ledger row (direction in) of the provider payment.
	PaymentID      string
	Amount         money.Cents
	Reason         string
	IdempotencyKey string
	// Actor is the audit actor (zero value = system; the checkout passes
	// the customer, RefundBooking the staff principal).
	Actor audit.Actor
}

// ReportQuery filters the reconciliation report.
type ReportQuery struct {
	From, To   *calendar.Date
	IssuesOnly bool
}

// ReportItem is one line of the reconciliation report (api.ReconciliationItem).
type ReportItem struct {
	Intent         store.PaymentIntent
	BookingCode    string
	ProviderStatus string
	// Issue is one of the Issue* constants.
	Issue string
}

// Report is the reconciliation report (api.ReconciliationReport).
type Report struct {
	GeneratedAt time.Time
	From, To    *calendar.Date
	Checked     int
	Issues      int
	Items       []ReportItem
}

// ReconcileStats is the outcome of a reconciliation run.
type ReconcileStats struct {
	Checked, Updated, Failed int
	StartedAt, FinishedAt    time.Time
}

// ---- helpers ------------------------------------------------------------------

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, payment.ErrEstablishmentNotFound
	}
	return es, err
}

// provider returns the configuration and the driver of provider for es.
func (s *Service) provider(ctx context.Context, db bun.IDB, es store.EstablishmentScope, code string) (*online.ProviderConfig, ext.PaymentProvider, error) {
	cfg, err := s.d.Settings.ProviderConfig(ctx, db, es, code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrProviderNotConfigured
		}
		return nil, nil, fmt.Errorf("gateway: provider settings: %w", err)
	}
	pp, ok := s.d.Providers.PaymentProvider(code)
	if !ok {
		return nil, nil, ErrProviderNotConfigured
	}
	return cfg, pp, nil
}

// providerError classifies a driver error: temporary failures become
// ErrProviderUnavailable, refusals ErrProviderRefused with the provider
// code. The driver message is never propagated (it may echo request data).
func providerError(op string, err error) error {
	if err == nil {
		return nil
	}
	if isTemporary(err) {
		return fmt.Errorf("gateway: %s: %w", op, ErrProviderUnavailable)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("gateway: %s: %w", op, err)
	}
	return fmt.Errorf("%w (%s)", ErrProviderRefused, providerCode(err))
}

// isTemporary reports a retryable driver error (Temporary() true, or a
// timeout).
func isTemporary(err error) bool {
	var t interface{ Temporary() bool }
	if errors.As(err, &t) && t.Temporary() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrProviderUnavailable)
}

func isIdempotencyConflict(err error) bool {
	var ic interface{ IdempotencyConflict() bool }
	return errors.As(err, &ic) && ic.IdempotencyConflict()
}

// providerCode is the short machine code of a driver refusal.
func providerCode(err error) string {
	var pc interface{ ProviderCode() string }
	if errors.As(err, &pc) && pc.ProviderCode() != "" {
		return pc.ProviderCode()
	}
	return "error"
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func isOpen(status string) bool {
	return status == store.IntentCreated || status == store.IntentPending
}

func isPlaceholder(externalID string) bool { return strings.HasPrefix(externalID, placeholderPrefix) }

// appendEvent records an applied provider event id on the intent.
func appendEvent(pi *store.PaymentIntent, id string) {
	for _, x := range pi.RawEventIDs {
		if x == id {
			return
		}
	}
	pi.RawEventIDs = append(pi.RawEventIDs, id)
	if n := len(pi.RawEventIDs); n > maxRawEventIDs {
		pi.RawEventIDs = pi.RawEventIDs[n-maxRawEventIDs:]
	}
}

// change is the outbox payload of an intent (no personal data).
func change(pi *store.PaymentIntent, paymentID, direction, status string) event.PaymentChange {
	if paymentID == "" {
		paymentID = pi.ID
	}
	return event.PaymentChange{
		PaymentID: paymentID, BookingID: deref(pi.BookingID), Provider: pi.Provider, Method: store.PaymentMethodCard,
		Direction: direction, Amount: int64(pi.Amount), Status: status,
	}
}

// record writes an audit entry of intent pi and, when eventType is set, the
// outbox event (same transaction).
func (s *Service) record(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, actor audit.Actor, action string, pi *store.PaymentIntent, diff map[string]any, eventType string, ch event.PaymentChange) error {
	if diff == nil {
		diff = map[string]any{}
	}
	diff["provider"], diff["kind"], diff["status"], diff["amount"] = pi.Provider, pi.Kind, pi.Status, int64(pi.Amount)
	if pi.BookingID != nil {
		diff["booking_id"] = *pi.BookingID
	}
	if actor.Type == "" {
		actor = audit.SystemActor
	}
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "payment_intent", EntityID: pi.ID, Diff: diff,
	}); err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	if eventType == "" {
		return nil
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, ch); err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	return nil
}
