package ext

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ErrNotSupported is returned by optional operations a driver does not implement.
var ErrNotSupported = errors.New("ext: operation not supported by this driver")

// ErrInvalidSignature is returned by ParseWebhook when the signature is missing or wrong.
var ErrInvalidSignature = errors.New("ext: invalid webhook signature")

// ErrIgnoredEvent is returned by ParseWebhook for authentic provider events
// that Arenile does not handle; the webhook endpoint answers 200.
var ErrIgnoredEvent = errors.New("ext: ignored webhook event")

// PaymentCapabilities describes what a payment provider supports.
type PaymentCapabilities struct {
	// Online is true when customers can pay online (false for "manual").
	Online bool
	// Refunds is true when Refund can return money to the customer.
	Refunds bool
	// PartialRefunds is true when Refund accepts an amount lower than the payment.
	PartialRefunds bool
	// Wallets is true when Apple Pay / Google Pay are available.
	Wallets bool
	// EmbeddedMode is true when the provider supports in-page payment forms
	// in addition to redirect.
	EmbeddedMode bool
}

// PaymentMode tells the client how to complete a payment.
type PaymentMode string

const (
	// PaymentModeRedirect requires a top-level navigation to RedirectURL.
	PaymentModeRedirect PaymentMode = "redirect"
	// PaymentModeEmbedded requires an in-page form initialised with ClientSecret.
	PaymentModeEmbedded PaymentMode = "embedded"
	// PaymentModeNone means no online step is needed (e.g. pay at the desk).
	PaymentModeNone PaymentMode = "none"
)

// PaymentProvider is a payment gateway driver (spec 13.1). Credentials are
// per establishment and come from the context of the call (the core passes
// them through CreatePaymentRequest.Credentials, already decrypted).
type PaymentProvider interface {
	// Code is the stable identifier (e.g. "manual", "stripe").
	Code() string
	// Capabilities reports the supported features.
	Capabilities() PaymentCapabilities
	// CreatePayment creates a payment intent and returns data for the client.
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResult, error)
	// ParseWebhook verifies and interprets a provider webhook. It must return
	// ErrInvalidSignature for unauthenticated requests and ErrIgnoredEvent for
	// irrelevant event types.
	ParseWebhook(ctx context.Context, r *http.Request, creds Credentials) (*PaymentEvent, error)
	// Refund returns money to the customer, or ErrNotSupported.
	Refund(ctx context.Context, req RefundRequest) (*RefundResult, error)
}

// Credentials are the decrypted, provider-specific settings of one
// establishment (e.g. "secret_key", "webhook_secret").
type Credentials map[string]string

// CreatePaymentRequest asks a provider to start a payment. Amount is in
// integer minor units (cents).
type CreatePaymentRequest struct {
	EstablishmentID string
	BookingID       string
	BookingCode     string
	Amount          int64
	Currency        string
	Description     string
	CustomerEmail   string
	ReturnURL       string
	CancelURL       string
	// PreferredMode is the mode chosen by the establishment; providers may
	// fall back to redirect.
	PreferredMode PaymentMode
	// IdempotencyKey must be forwarded to the provider (booking id + attempt).
	IdempotencyKey string
	Metadata       map[string]string
	Credentials    Credentials
}

// CreatePaymentResult tells the client how to proceed.
type CreatePaymentResult struct {
	ProviderRef  string
	Mode         PaymentMode
	RedirectURL  string
	ClientSecret string
	// ExpiresAt is when the provider session expires, if known.
	ExpiresAt *time.Time
}

// PaymentEventType is the normalised outcome reported by a webhook.
type PaymentEventType string

const (
	PaymentSucceeded PaymentEventType = "payment.succeeded"
	PaymentFailed    PaymentEventType = "payment.failed"
	PaymentRefunded  PaymentEventType = "payment.refunded"
	PaymentPending   PaymentEventType = "payment.pending"
)

// PaymentEvent is a verified provider notification.
type PaymentEvent struct {
	// ProviderEventID deduplicates notifications: the same id is never
	// processed twice.
	ProviderEventID string
	Type            PaymentEventType
	ProviderRef     string
	EstablishmentID string
	BookingID       string
	// Amount in cents; for refunds, the refunded amount.
	Amount     int64
	Currency   string
	OccurredAt time.Time
	// FailureReason is a short, non-personal description for failures.
	FailureReason string
	Metadata      map[string]string
}

// RefundRequest asks a provider to refund (part of) a payment.
type RefundRequest struct {
	EstablishmentID string
	PaymentID       string
	ProviderRef     string
	Amount          int64
	Currency        string
	Reason          string
	IdempotencyKey  string
	Credentials     Credentials
}

// RefundStatus is the provider-side state of a refund.
type RefundStatus string

const (
	RefundPending   RefundStatus = "pending"
	RefundSucceeded RefundStatus = "succeeded"
	RefundFailed    RefundStatus = "failed"
)

// RefundResult is the outcome of Refund.
type RefundResult struct {
	ProviderRef string
	Status      RefundStatus
}
