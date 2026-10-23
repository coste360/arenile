package stripe

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/pkg/ext"
)

// Code is the provider code of the driver.
const Code = "stripe"

// APIVersion is the Stripe API version pinned in every request.
const APIVersion = "2024-06-20"

// DefaultBaseURL is the Stripe API endpoint.
const DefaultBaseURL = "https://api.stripe.com"

// Credential keys (ext.Credentials) stored encrypted per establishment.
const (
	CredSecretKey      = "secret_key"
	CredWebhookSecret  = "webhook_secret"
	CredPublishableKey = "publishable_key"
)

// SignatureTolerance is the maximum age of a webhook signature timestamp.
const SignatureTolerance = 5 * time.Minute

// Errors of the driver.
var (
	// ErrUnavailable: network error, 5xx or 429 from Stripe.
	ErrUnavailable = errors.New("stripe: service unavailable")
	// ErrMissingCredentials: the secret key (or webhook secret) is missing.
	ErrMissingCredentials = errors.New("stripe: missing credentials")
	// ErrNotImplemented marks the scaffolding (M4 contract).
	ErrNotImplemented = errors.New("stripe: not implemented")
)

// Options configure the driver.
type Options struct {
	// HTTPClient defaults to a client with a 20 s timeout.
	HTTPClient *http.Client
	// BaseURL defaults to DefaultBaseURL (tests use an httptest.Server).
	BaseURL string
	// Clock verifies webhook timestamps (default clock.System).
	Clock clock.Clock
}

// Driver is the Stripe payment provider. Safe for concurrent use.
type Driver struct {
	o Options
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
	return &Driver{o: o}
}

// Code implements ext.PaymentProvider.
func (*Driver) Code() string { return Code }

// Capabilities implements ext.PaymentProvider.
func (*Driver) Capabilities() ext.PaymentCapabilities {
	return ext.PaymentCapabilities{Online: true, Refunds: true, PartialRefunds: true, Wallets: true, EmbeddedMode: true}
}

// CreatePayment implements ext.PaymentProvider (Checkout Session, see doc.go).
func (d *Driver) CreatePayment(ctx context.Context, req ext.CreatePaymentRequest) (*ext.CreatePaymentResult, error) {
	return nil, ErrNotImplemented
}

// ParseWebhook implements ext.PaymentProvider (signature verification, see doc.go).
func (d *Driver) ParseWebhook(ctx context.Context, r *http.Request, creds ext.Credentials) (*ext.PaymentEvent, error) {
	return nil, ErrNotImplemented
}

// Refund implements ext.PaymentProvider.
func (d *Driver) Refund(ctx context.Context, req ext.RefundRequest) (*ext.RefundResult, error) {
	return nil, ErrNotImplemented
}

// PaymentStatus implements gateway.PaymentStatusChecker (reconciliation).
func (d *Driver) PaymentStatus(ctx context.Context, providerRef string, creds ext.Credentials) (*ext.PaymentEvent, error) {
	return nil, ErrNotImplemented
}

// VerifyCredentials implements gateway.CredentialsVerifier.
func (d *Driver) VerifyCredentials(ctx context.Context, creds ext.Credentials) (string, error) {
	return "", ErrNotImplemented
}
