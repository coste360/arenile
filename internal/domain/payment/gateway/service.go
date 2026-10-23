package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
	"github.com/coste360/arenile/internal/jobs"
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
)

// Errors of the gateway (mapped by internal/http/problem).
var (
	// ErrProviderUnavailable: the provider could not be reached or answered
	// with a server error (503 payment_provider_unavailable).
	ErrProviderUnavailable = errors.New("gateway: payment provider unavailable")
	// ErrSignatureInvalid: webhook signature missing or wrong (400
	// webhook_signature_invalid). Wraps ext.ErrInvalidSignature.
	ErrSignatureInvalid = fmt.Errorf("gateway: %w", ext.ErrInvalidSignature)
	// ErrProviderNotConfigured: no credentials for the provider of the URL
	// or of the active configuration (404 for webhooks, 409 for refunds).
	ErrProviderNotConfigured = fmt.Errorf("%w: nessun provider di pagamento impostato", configerr.ErrConflict)
	// ErrRefundNotSupported: the provider of the payment cannot refund.
	ErrRefundNotSupported = fmt.Errorf("%w: il provider non supporta i rimborsi", configerr.ErrConflict)
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

// PaymentStatusChecker is an OPTIONAL capability of a payment driver, used
// by reconciliation to fetch the current outcome of a provider payment
// (see doc.go). It returns the normalised event the provider would have
// sent (ProviderEventID may be empty; the gateway then uses a synthetic
// id), or nil when the payment is still pending.
type PaymentStatusChecker interface {
	PaymentStatus(ctx context.Context, providerRef string, creds ext.Credentials) (*ext.PaymentEvent, error)
}

// CredentialsVerifier is an OPTIONAL capability used by "test payment
// settings": it checks the credentials with the provider and returns a
// short, non-secret account label.
type CredentialsVerifier interface {
	VerifyCredentials(ctx context.Context, creds ext.Credentials) (accountLabel string, err error)
}

// Deps are the dependencies of the gateway. Every field is required except
// Log, PendingAfter and ReconcileInterval.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Providers Providers
	Settings  ProviderSettings
	Ledger    Ledger
	// BaseURL is config.EffectiveBaseURL() (return and cancel URLs).
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
	// Ignored: authentic event of a type Arenile does not handle.
	Ignored bool
}

// RefundRequest refunds (part of) an online payment through its provider.
type RefundRequest struct {
	BookingID string
	// PaymentID is the ledger row (direction in) of the provider payment.
	PaymentID      string
	Amount         money.Cents
	Reason         string
	IdempotencyKey string
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
	// Issue is one of none, pending_too_long, status_mismatch,
	// missing_ledger_row, amount_mismatch, provider_unreachable.
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

// Start creates a provider payment for a booking of es (see doc.go for the
// transaction shape). tx is the caller's transaction for the local rows.
func (s *Service) Start(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req StartRequest) (*StartResult, error) {
	return nil, configerr.ErrNotImplemented
}

// LatestForBooking returns the most recent payment intent of a booking
// (store.ErrNotFound when none): status polling of the public page.
func (s *Service) LatestForBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*store.PaymentIntent, error) {
	return nil, configerr.ErrNotImplemented
}

// HandleWebhook verifies and applies one provider notification for the
// establishment of the URL. r.Body is read raw (MaxWebhookBody).
func (s *Service) HandleWebhook(ctx context.Context, provider, establishmentID string, r *http.Request) (*WebhookOutcome, error) {
	return nil, configerr.ErrNotImplemented
}

// RefundViaProvider executes a refund with the provider of the payment and
// records a refund intent (pending until the provider confirms; the ledger
// row is written by the webhook or reconciliation). Used by the admin
// refund with via_provider and by customer cancellations.
func (s *Service) RefundViaProvider(ctx context.Context, es store.EstablishmentScope, req RefundRequest) (*store.PaymentIntent, error) {
	return nil, configerr.ErrNotImplemented
}

// RefundBooking is RefundViaProvider with the principal check
// (auth.PermRefundsIssue), for internal/http/admin/payments.go.
func (s *Service) RefundBooking(ctx context.Context, p *auth.Principal, establishmentID string, req RefundRequest) (*store.PaymentIntent, error) {
	return nil, configerr.ErrNotImplemented
}

// Report builds the reconciliation report (auth.PermBookingsRead).
func (s *Service) Report(ctx context.Context, p *auth.Principal, establishmentID string, q ReportQuery) (*Report, error) {
	return nil, configerr.ErrNotImplemented
}

// ReconcileEstablishment runs the reconciliation now for one establishment
// (auth.PermPaymentsConfigure).
func (s *Service) ReconcileEstablishment(ctx context.Context, p *auth.Principal, establishmentID string) (*ReconcileStats, error) {
	return nil, configerr.ErrNotImplemented
}

// Reconcile is the job body: every establishment, intents pending for more
// than PendingAfter.
func (s *Service) Reconcile(ctx context.Context) (*ReconcileStats, error) {
	return nil, configerr.ErrNotImplemented
}

// Jobs returns the background jobs of the gateway (reconciliation).
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:     "payments.reconcile",
		Interval: s.d.ReconcileInterval,
		Run: func(ctx context.Context) error {
			_, err := s.Reconcile(ctx)
			if errors.Is(err, configerr.ErrNotImplemented) {
				return nil
			}
			return err
		},
	}}
}
