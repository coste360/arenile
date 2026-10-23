package stripe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	stripego "github.com/stripe/stripe-go/v87"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/pkg/ext"
)

// Code is the provider code of the driver.
const Code = "stripe"

// APIVersion is the Stripe API version pinned by the SDK in every request
// (stripe-go v87). Webhook endpoints should be created with the same
// version; events of other versions are still parsed (only the fields
// Arenile reads are used).
const APIVersion = stripego.APIVersion

// DefaultBaseURL is the Stripe API endpoint.
const DefaultBaseURL = "https://api.stripe.com"

// Credential keys (ext.Credentials) stored encrypted per establishment.
const (
	CredSecretKey      = "secret_key"
	CredWebhookSecret  = "webhook_secret"
	CredPublishableKey = "publishable_key"
)

// SignatureTolerance is the maximum distance between the webhook signature
// timestamp and the injected clock (replay protection).
const SignatureTolerance = 5 * time.Minute

// MaxWebhookBody bounds the raw body read by ParseWebhook (the HTTP handler
// applies the same bound before).
const MaxWebhookBody = 256 << 10

// Checkout Session lifetime bounds imposed by Stripe (expires_at must be
// between 30 minutes and 24 hours from creation).
const (
	minSessionLifetime = 31 * time.Minute // one minute of margin over Stripe's 30
	maxSessionLifetime = 24 * time.Hour
)

// Hint keys of ext.CreatePaymentRequest.Metadata read by the driver and NOT
// forwarded to Stripe (gateway.MetaLocale, gateway.MetaExpiresAt).
const (
	HintLocale    = "locale"
	HintExpiresAt = "expires_at"
)

// Keys of ext.PaymentEvent.Metadata set by the driver (gateway.Meta*).
const (
	MetaOutcome        = "outcome"
	MetaPaymentIntent  = "payment_intent"
	MetaAmountCaptured = "amount_captured"
	MetaStripeType     = "stripe_type"
)

// Outcome values (ext.PaymentEvent.Metadata[MetaOutcome], gateway.Outcome*).
const (
	OutcomeExpired       = "expired"        // checkout session expired unpaid
	OutcomeAttemptFailed = "attempt_failed" // one attempt failed, the session is still payable
	OutcomeRefundFailed  = "refund_failed"  // a refund failed or was cancelled
	OutcomeRefundTotal   = "refund_total"   // Amount is the cumulative refunded amount of the payment
	OutcomeRefundSingle  = "refund_single"  // Amount is the amount of the refund ProviderRef
)

// temporaryError marks failures worth retrying: network errors, 5xx, 429.
// The gateway recognises them through the Temporary method, without
// importing this package.
type temporaryError struct{ msg string }

func (e *temporaryError) Error() string   { return e.msg }
func (e *temporaryError) Temporary() bool { return true }

// idempotencyError marks a reused idempotency key with different
// parameters (gateway: retry with a fresh key).
type idempotencyError struct{}

func (idempotencyError) Error() string {
	return "stripe: idempotency key reused with different parameters"
}
func (idempotencyError) IdempotencyConflict() bool { return true }

// Errors of the driver.
var (
	// ErrUnavailable: network error, 5xx or 429 from Stripe.
	ErrUnavailable error = &temporaryError{msg: "stripe: service unavailable"}
	// ErrMissingCredentials: the secret key (or webhook secret) is missing.
	ErrMissingCredentials = errors.New("stripe: missing credentials")
	// ErrAuthentication: Stripe refused the secret key (401).
	ErrAuthentication = errors.New("stripe: secret key refused")
	// ErrIdempotencyConflict: Stripe refused a reused idempotency key.
	ErrIdempotencyConflict error = idempotencyError{}
)

// APIError is a 4xx answer of Stripe. Only the machine-readable parts are
// kept: the message may echo request data and is never logged.
type APIError struct {
	Status int
	Type   string
	Code   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("stripe: request refused (status %d, type %s, code %s)", e.Status, e.Type, e.Code)
}

// ProviderCode is the short, non-personal reason saved on the intent.
func (e *APIError) ProviderCode() string {
	if e.Code != "" {
		return e.Code
	}
	return e.Type
}

// Options configure the driver.
type Options struct {
	// HTTPClient defaults to a client with a 20 s timeout.
	HTTPClient *http.Client
	// BaseURL defaults to DefaultBaseURL (tests use an httptest.Server).
	BaseURL string
	// Clock verifies webhook timestamps and computes session expiries
	// (default clock.System).
	Clock clock.Clock
	// MaxNetworkRetries is the SDK retry budget for network errors and
	// 409/5xx answers (always with the same idempotency key). Nil = 1.
	MaxNetworkRetries *int64
}

// Driver is the Stripe payment provider. It is stateless and safe for
// concurrent use: the per-establishment secret key arrives with every call
// and a short-lived SDK client is built around shared HTTP backends.
type Driver struct {
	o        Options
	backends *stripego.Backends
}

var _ ext.PaymentProvider = (*Driver)(nil)

// New returns a Driver.
func New(o Options) *Driver {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	if o.BaseURL == "" {
		o.BaseURL = DefaultBaseURL
	}
	if o.Clock == nil {
		o.Clock = clock.System{}
	}
	retries := int64(1)
	if o.MaxNetworkRetries != nil {
		retries = *o.MaxNetworkRetries
	}
	base := strings.TrimRight(o.BaseURL, "/")
	cfg := &stripego.BackendConfig{
		HTTPClient:        o.HTTPClient,
		URL:               stripego.String(base),
		MaxNetworkRetries: stripego.Int64(retries),
		EnableTelemetry:   stripego.Bool(false),
		// The SDK logger could print request details: silenced, errors are
		// returned and logged by the gateway without secrets.
		LeveledLogger: &stripego.LeveledLogger{Level: stripego.LevelNull},
	}
	return &Driver{o: o, backends: stripego.NewBackendsWithConfig(cfg)}
}

// Code implements ext.PaymentProvider.
func (*Driver) Code() string { return Code }

// Capabilities implements ext.PaymentProvider.
func (*Driver) Capabilities() ext.PaymentCapabilities {
	return ext.PaymentCapabilities{Online: true, Refunds: true, PartialRefunds: true, Wallets: true, EmbeddedMode: true}
}

// client returns an SDK client bound to the secret key of creds.
func (d *Driver) client(creds ext.Credentials) (*stripego.Client, error) {
	key := strings.TrimSpace(creds[CredSecretKey])
	if key == "" {
		return nil, ErrMissingCredentials
	}
	return stripego.NewClient(key, stripego.WithBackends(d.backends)), nil
}

// mapErr turns SDK and transport errors into the driver errors. It never
// includes the Stripe message (it may echo request data).
func mapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	var se *stripego.Error
	if errors.As(err, &se) {
		switch {
		case se.Type == stripego.ErrorTypeIdempotency:
			return fmt.Errorf("stripe: %s: %w", op, ErrIdempotencyConflict)
		case se.HTTPStatusCode == http.StatusUnauthorized:
			return fmt.Errorf("stripe: %s: %w", op, ErrAuthentication)
		case se.HTTPStatusCode >= 500, se.HTTPStatusCode == http.StatusTooManyRequests,
			se.Type == stripego.ErrorTypeAPI, se.Type == stripego.ErrorTypeRateLimit:
			return fmt.Errorf("stripe: %s (status %d): %w", op, se.HTTPStatusCode, ErrUnavailable)
		default:
			return fmt.Errorf("stripe: %s: %w", op, &APIError{Status: se.HTTPStatusCode, Type: string(se.Type), Code: string(se.Code)})
		}
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("stripe: %s: %w", op, err)
	}
	// Anything else comes from the transport (timeout, connection refused,
	// EOF, TLS): unavailability, retried later with the same idempotency key.
	return fmt.Errorf("stripe: %s: %w", op, ErrUnavailable)
}

// stripeLocale maps an Arenile locale to a Checkout locale.
func stripeLocale(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	switch l {
	case "it", "en", "de", "fr", "es", "nl", "pt", "pl":
		return l
	default:
		return "auto"
	}
}
