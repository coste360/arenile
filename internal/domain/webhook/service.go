package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
)

// TestEventType is the type of the event sent by SendTest; it is never
// written by the domain and reaches only the tested endpoint.
const TestEventType = "webhook.test"

// EventTypes lists the subscribable event types (spec 18.1), in the order
// shown by the panel. Keep in sync with the API enum WebhookEventType.
var EventTypes = []string{
	event.BookingCreated, event.BookingHeld, event.BookingConfirmed, event.BookingModified,
	event.BookingCancelled, event.BookingCheckedIn, event.BookingNoShow, event.BookingCompleted,
	event.BookingApprovalRequested, event.AllocationReleased, event.SeasonDayReleased,
	event.SeasonRenewalOffered, event.PaymentSucceeded, event.PaymentFailed, event.PaymentRefunded,
	event.CustomerCreated, event.CustomerAnonymized, event.CashSessionClosed,
	event.ConfigPricingChanged, event.ConfigMapChanged,
}

// DefaultGrace is the default grace period of a rotated secret.
const DefaultGrace = 24 * time.Hour

// MaxGrace bounds the grace period of a rotated secret.
const MaxGrace = 168 * time.Hour

// EndpointStatus is the derived state of an endpoint (API enum
// WebhookEndpointStatus).
type EndpointStatus string

// Endpoint states.
const (
	EndpointActive   EndpointStatus = "active"
	EndpointPaused   EndpointStatus = "paused"
	EndpointDisabled EndpointStatus = "disabled"
)

// DeliveryStatus is the derived state of a delivery (API enum
// WebhookDeliveryStatus).
type DeliveryStatus string

// Delivery states.
const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)

// Endpoint is the management view of an endpoint (never a secret).
type Endpoint struct {
	ID                      string
	URL                     string
	Description             string
	Events                  []string
	Active                  bool
	Status                  EndpointStatus
	ConsecutiveFailures     int
	DisabledAt              *time.Time
	DisabledReason          string
	PreviousSecretExpiresAt *time.Time
	LastDeliveryAt          *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// Delivery is one row of the delivery log.
type Delivery struct {
	ID            string
	EndpointID    string
	EventID       string // "evt_<uuid>"
	EventType     string
	Status        DeliveryStatus
	Attempts      int
	StatusCode    *int
	LastError     string
	NextAttemptAt *time.Time
	DeliveredAt   *time.Time
	FailedAt      *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DeliveryDetail adds the sent envelope and the response excerpt.
type DeliveryDetail struct {
	Delivery
	RequestBody     json.RawMessage
	ResponseExcerpt string
}

// CreateInput describes a new endpoint.
type CreateInput struct {
	URL         string
	Description string
	Events      []string
	Active      bool
	IP          string
}

// UpdateInput changes an endpoint; nil fields are unchanged.
type UpdateInput struct {
	URL         *string
	Description *string
	Events      []string // nil = unchanged
	Active      *bool
	IP          string
}

// DeliveryFilter selects the delivery log page.
type DeliveryFilter struct {
	Status DeliveryStatus // "" = all
	Cursor string
	Limit  int
}

// Deliverer is the part of event.Deliverer used by this package (declared
// by the consumer; *event.Deliverer implements it once AttemptNow lands).
type Deliverer interface {
	AttemptNow(ctx context.Context, deliveryID string) error
}

// Deps are the dependencies of the service. Every field is required except
// Log.
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Audit *audit.Writer
	// Keys encrypts and decrypts endpoint secrets (ARENILE_SECRET_KEY).
	Keys *crypto.KeyRing
	// Deliverer performs the synchronous test attempt.
	Deliverer Deliverer
	Log       *slog.Logger
}

// Service implements the webhook endpoint management use cases.
//
// Handler mapping (internal/http/admin/webhookendpoints.go):
//
//	ListWebhookEndpoints  -> List           -> api.WebhookEndpointList
//	CreateWebhookEndpoint -> Create         -> 201 api.WebhookEndpointCreated (no-store)
//	GetWebhookEndpoint    -> Get            -> api.WebhookEndpoint
//	UpdateWebhookEndpoint -> Update         -> api.WebhookEndpoint
//	DeleteWebhookEndpoint -> Delete         -> 204
//	RotateWebhookSecret   -> RotateSecret   -> api.WebhookSecretRotated (no-store)
//	SendWebhookTest       -> SendTest       -> api.WebhookDelivery
//	ListWebhookDeliveries -> ListDeliveries -> api.WebhookDeliveryList
//	GetWebhookDelivery    -> GetDelivery    -> api.WebhookDeliveryDetail
//	RedeliverWebhook      -> Redeliver      -> 202 api.WebhookDelivery
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// List returns the endpoints of the establishment, oldest first.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string) ([]Endpoint, error) {
	return nil, configerr.ErrNotImplemented
}

// Get returns one endpoint.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Endpoint, error) {
	return nil, configerr.ErrNotImplemented
}

// Create registers an endpoint and returns it with the plaintext secret
// (shown once).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*Endpoint, string, error) {
	return nil, "", configerr.ErrNotImplemented
}

// Update changes url, description, events or the active flag.
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, in UpdateInput) (*Endpoint, error) {
	return nil, configerr.ErrNotImplemented
}

// Delete removes an endpoint and its deliveries.
func (s *Service) Delete(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) error {
	return configerr.ErrNotImplemented
}

// RotateSecret replaces the secret, keeping the previous one valid for
// grace (0..MaxGrace), and returns the new plaintext secret (shown once).
func (s *Service) RotateSecret(ctx context.Context, p *auth.Principal, establishmentID, id string, grace time.Duration, ip string) (*Endpoint, string, error) {
	return nil, "", configerr.ErrNotImplemented
}

// SendTest delivers a TestEventType event to this endpoint synchronously.
func (s *Service) SendTest(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) (*Delivery, error) {
	return nil, configerr.ErrNotImplemented
}

// ListDeliveries returns a page of the delivery log, newest first, and the
// next cursor ("" when last).
func (s *Service) ListDeliveries(ctx context.Context, p *auth.Principal, establishmentID, endpointID string, f DeliveryFilter) ([]Delivery, string, error) {
	return nil, "", configerr.ErrNotImplemented
}

// GetDelivery returns one delivery with the envelope and response excerpt.
func (s *Service) GetDelivery(ctx context.Context, p *auth.Principal, establishmentID, endpointID, deliveryID string) (*DeliveryDetail, error) {
	return nil, configerr.ErrNotImplemented
}

// Redeliver queues a failed or delivered delivery again.
func (s *Service) Redeliver(ctx context.Context, p *auth.Principal, establishmentID, endpointID, deliveryID, ip string) (*Delivery, error) {
	return nil, configerr.ErrNotImplemented
}
