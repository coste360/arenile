package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// TestEventType is the type of the event sent by SendTest; it is never
// written by the domain and reaches only the tested endpoint.
const TestEventType = event.TestEventType

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

// DefaultRetention is how long finished deliveries stay in the delivery log.
const DefaultRetention = 30 * 24 * time.Hour

// Errors of the webhook package (mapped through the configerr kinds).
var (
	// ErrEndpointInactive: redelivery to a paused or disabled endpoint (409).
	ErrEndpointInactive = fmt.Errorf("%w: endpoint non attivo, riattivalo prima di reinviare", configerr.ErrConflict)
	// ErrDeliveryPending: redelivery of a delivery still in progress (409).
	ErrDeliveryPending = fmt.Errorf("%w: consegna ancora in corso", configerr.ErrConflict)
	// ErrNoSecretKey: ARENILE_SECRET_KEY missing, secrets cannot be stored.
	ErrNoSecretKey = errors.New("webhook: secret key not configured")
)

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
// by the consumer).
type Deliverer interface {
	AttemptNow(ctx context.Context, deliveryID string) error
}

var _ Deliverer = (*event.Deliverer)(nil)

// Deps are the dependencies of the service. Every field is required except
// Log, Egress (zero value: the strict policy) and Retention.
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Audit *audit.Writer
	// Keys encrypts and decrypts endpoint secrets (ARENILE_SECRET_KEY).
	Keys *crypto.KeyRing
	// Deliverer performs the synchronous test attempt.
	Deliverer Deliverer
	// Egress is the SSRF policy also applied when validating IP literals in
	// endpoint URLs (the Deliverer enforces it again at connect time).
	Egress event.EgressPolicy
	// Retention of finished deliveries (default DefaultRetention).
	Retention time.Duration
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
	if d.Retention <= 0 {
		d.Retention = DefaultRetention
	}
	return &Service{d: d}
}

// authorize refuses API key principals (a key never manages webhooks) and
// requires auth.PermUsersManage on the establishment.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string) (store.EstablishmentScope, error) {
	if p != nil && p.Kind == auth.PrincipalAPIKey {
		return store.EstablishmentScope{}, auth.ErrForbidden
	}
	return p.Authorize(ctx, s.d.DB, establishmentID, auth.PermUsersManage)
}

// List returns the endpoints of the establishment, oldest first.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string) ([]Endpoint, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListWebhookEndpoints(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	out := make([]Endpoint, len(rows))
	for i := range rows {
		out[i] = s.endpointView(&rows[i])
	}
	return out, nil
}

// Get returns one endpoint.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Endpoint, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	ep, err := store.GetWebhookEndpoint(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, err
	}
	v := s.endpointView(ep)
	return &v, nil
}

// Create registers an endpoint and returns it with the plaintext secret
// (shown once).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*Endpoint, string, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, "", err
	}
	if s.d.Keys == nil {
		return nil, "", ErrNoSecretKey
	}
	ve := &configerr.ValidationError{}
	u := ValidateURL(ve, in.URL, s.d.Egress)
	events := ValidateEvents(ve, in.Events)
	desc := validateDescription(ve, in.Description)
	if err := ve.OrNil(); err != nil {
		return nil, "", err
	}
	secret, err := event.NewWebhookSecret()
	if err != nil {
		return nil, "", err
	}
	id := store.NewID()
	enc, err := s.d.Keys.Encrypt([]byte(secret), []byte(id))
	if err != nil {
		return nil, "", fmt.Errorf("webhook: encrypt secret: %w", err)
	}
	now := s.d.Clock.Now()
	ep := &store.WebhookEndpoint{
		ID: id, URL: u, Secret: enc, Events: events, Active: in.Active, Description: desc,
		CreatedAt: now, UpdatedAt: now,
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertWebhookEndpoint(ctx, tx, es, ep); err != nil {
			return err
		}
		return s.record(ctx, tx, p, es, in.IP, "webhook.created", id, map[string]any{
			"url": u, "events": events, "active": in.Active, "description": desc,
		})
	})
	if err != nil {
		return nil, "", fmt.Errorf("webhook: create: %w", err)
	}
	v := s.endpointView(ep)
	return &v, secret, nil
}

// Update changes url, description, events or the active flag. Activating
// an endpoint (paused or disabled) resets its failure counter and the
// automatic disable.
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, in UpdateInput) (*Endpoint, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	var out Endpoint
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ep, err := store.GetWebhookEndpoint(ctx, tx, es, id)
		if err != nil {
			return err
		}
		ve := &configerr.ValidationError{}
		diff := map[string]any{}
		var cols []string
		if in.URL != nil {
			if u := ValidateURL(ve, *in.URL, s.d.Egress); u != "" && u != ep.URL {
				diff["url"] = change(ep.URL, u)
				ep.URL = u
				cols = append(cols, "url")
			}
		}
		if in.Description != nil {
			if d := validateDescription(ve, *in.Description); d != ep.Description {
				diff["description"] = change(ep.Description, d)
				ep.Description = d
				cols = append(cols, "description")
			}
		}
		if in.Events != nil {
			if evs := ValidateEvents(ve, in.Events); len(evs) > 0 && !equalStrings(evs, ep.Events) {
				diff["events"] = change(ep.Events, evs)
				ep.Events = evs
				cols = append(cols, "events")
			}
		}
		if in.Active != nil && (*in.Active != ep.Active || (*in.Active && ep.DisabledAt != nil)) {
			diff["active"] = change(ep.Active, *in.Active)
			ep.Active = *in.Active
			cols = append(cols, "active")
			if ep.Active {
				ep.ConsecutiveFailures, ep.DisabledAt, ep.DisabledReason = 0, nil, nil
				cols = append(cols, "consecutive_failures", "disabled_at", "disabled_reason")
			}
		}
		if err := ve.OrNil(); err != nil {
			return err
		}
		if len(cols) > 0 {
			ep.UpdatedAt = s.d.Clock.Now()
			if err := store.UpdateWebhookEndpoint(ctx, tx, es, ep, cols...); err != nil {
				return err
			}
			if err := s.record(ctx, tx, p, es, in.IP, "webhook.updated", id, diff); err != nil {
				return err
			}
		}
		out = s.endpointView(ep)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes an endpoint and its deliveries.
func (s *Service) Delete(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ep, err := store.GetWebhookEndpoint(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if err := store.DeleteWebhookEndpoint(ctx, tx, es, id); err != nil {
			return err
		}
		return s.record(ctx, tx, p, es, ip, "webhook.deleted", id, map[string]any{"url": ep.URL, "events": ep.Events})
	})
}

// RotateSecret replaces the secret, keeping the previous one valid for
// grace (0..MaxGrace; 0 drops it immediately), and returns the new plaintext
// secret (shown once). During the grace period every delivery carries two
// v1 signatures (event.SignAll), so the receiver can switch secret without
// losing deliveries. Rotating again during a grace period replaces the
// previous secret with the current one (only one previous secret exists).
func (s *Service) RotateSecret(ctx context.Context, p *auth.Principal, establishmentID, id string, grace time.Duration, ip string) (*Endpoint, string, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, "", err
	}
	if grace < 0 || grace > MaxGrace {
		return nil, "", configerr.Invalid("grace_hours", fmt.Sprintf("Deve essere tra 0 e %d ore.", int(MaxGrace.Hours())))
	}
	if s.d.Keys == nil {
		return nil, "", ErrNoSecretKey
	}
	secret, err := event.NewWebhookSecret()
	if err != nil {
		return nil, "", err
	}
	var out Endpoint
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ep, err := store.GetWebhookEndpoint(ctx, tx, es, id)
		if err != nil {
			return err
		}
		now := s.d.Clock.Now()
		ep.PreviousSecret, ep.PreviousSecretExpiresAt = nil, nil
		if grace > 0 {
			cur, err := s.d.Keys.Decrypt(ep.Secret, []byte(ep.ID))
			if err != nil {
				return fmt.Errorf("webhook: decrypt current secret: %w", err)
			}
			prev, err := s.d.Keys.Encrypt(cur, []byte(event.PreviousSecretAAD(ep.ID)))
			if err != nil {
				return fmt.Errorf("webhook: encrypt previous secret: %w", err)
			}
			until := now.Add(grace)
			ep.PreviousSecret, ep.PreviousSecretExpiresAt = &prev, &until
		}
		enc, err := s.d.Keys.Encrypt([]byte(secret), []byte(ep.ID))
		if err != nil {
			return fmt.Errorf("webhook: encrypt secret: %w", err)
		}
		ep.Secret, ep.UpdatedAt = enc, now
		if err := store.UpdateWebhookEndpoint(ctx, tx, es, ep, "secret", "previous_secret", "previous_secret_expires_at"); err != nil {
			return err
		}
		out = s.endpointView(ep)
		return s.record(ctx, tx, p, es, ip, "webhook.secret_rotated", id, map[string]any{
			"grace_hours": int(grace.Hours()), "previous_secret_expires_at": ep.PreviousSecretExpiresAt,
		})
	})
	if err != nil {
		return nil, "", err
	}
	return &out, secret, nil
}

// SendTest delivers a TestEventType event to this endpoint synchronously:
// an outbox row written already dispatched (no other endpoint or sink
// receives it), one delivery row, one attempt (no retries, no effect on the
// automatic disable). It works on paused endpoints too, so the owner can
// test before activating. The returned delivery carries the outcome.
func (s *Service) SendTest(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) (*Delivery, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if s.d.Deliverer == nil {
		return nil, configerr.ErrNotImplemented
	}
	now := s.d.Clock.Now()
	payload, err := json.Marshal(map[string]string{"message": "Evento di prova da Arenile", "endpoint_id": id})
	if err != nil {
		return nil, err
	}
	estID := es.ID()
	row := &store.OutboxEvent{
		ID: store.NewID(), EstablishmentID: &estID, Type: TestEventType, Payload: store.JSON(payload),
		Version: 1, OccurredAt: now, CreatedAt: now, DispatchedAt: &now,
	}
	del := &store.WebhookDelivery{ID: store.NewID(), EndpointID: id, EventID: row.ID, CreatedAt: now, UpdatedAt: now}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if _, err := store.GetWebhookEndpoint(ctx, tx, es, id); err != nil {
			return err
		}
		if err := store.InsertOutboxEvent(ctx, tx, row); err != nil {
			return err
		}
		if err := store.InsertWebhookDelivery(ctx, tx, del); err != nil {
			return err
		}
		return s.record(ctx, tx, p, es, ip, "webhook.tested", id, map[string]any{"delivery_id": del.ID})
	})
	if err != nil {
		return nil, err
	}
	if err := s.d.Deliverer.AttemptNow(ctx, del.ID); err != nil {
		return nil, fmt.Errorf("webhook: test attempt: %w", err)
	}
	return s.loadDelivery(ctx, id, del.ID)
}

// ListDeliveries returns a page of the delivery log, newest first, and the
// next cursor ("" when last).
func (s *Service) ListDeliveries(ctx context.Context, p *auth.Principal, establishmentID, endpointID string, f DeliveryFilter) ([]Delivery, string, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, "", err
	}
	if _, err := store.GetWebhookEndpoint(ctx, s.d.DB, es, endpointID); err != nil {
		return nil, "", err
	}
	switch f.Status {
	case "", DeliveryPending, DeliveryDelivered, DeliveryFailed:
	default:
		return nil, "", configerr.Invalid("status", "Stato sconosciuto.")
	}
	page, err := store.ListEndpointWebhookDeliveries(ctx, s.d.DB, endpointID, store.WebhookDeliveryState(f.Status), f.Cursor, f.Limit)
	if err != nil {
		return nil, "", err
	}
	ids := make([]string, len(page.Items))
	for i := range page.Items {
		ids[i] = page.Items[i].EventID
	}
	types, err := store.OutboxEventTypes(ctx, s.d.DB, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]Delivery, len(page.Items))
	for i := range page.Items {
		out[i] = deliveryView(&page.Items[i], types[page.Items[i].EventID])
	}
	return out, page.NextCursor, nil
}

// GetDelivery returns one delivery with the envelope and response excerpt.
func (s *Service) GetDelivery(ctx context.Context, p *auth.Principal, establishmentID, endpointID, deliveryID string) (*DeliveryDetail, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if _, err := store.GetWebhookEndpoint(ctx, s.d.DB, es, endpointID); err != nil {
		return nil, err
	}
	d, err := store.GetEndpointWebhookDelivery(ctx, s.d.DB, endpointID, deliveryID)
	if err != nil {
		return nil, err
	}
	row, err := store.GetOutboxEvent(ctx, s.d.DB, d.EventID)
	if err != nil {
		return nil, err
	}
	body, err := event.EnvelopeJSON(*row)
	if err != nil {
		return nil, err
	}
	out := &DeliveryDetail{Delivery: deliveryView(d, row.Type), RequestBody: body}
	if d.ResponseExcerpt != nil {
		out.ResponseExcerpt = *d.ResponseExcerpt
	}
	return out, nil
}

// Redeliver queues a failed or delivered delivery again (due now, a fresh
// retry window); the background Deliverer sends it.
func (s *Service) Redeliver(ctx context.Context, p *auth.Principal, establishmentID, endpointID, deliveryID, ip string) (*Delivery, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ep, err := store.GetWebhookEndpoint(ctx, tx, es, endpointID)
		if err != nil {
			return err
		}
		if !ep.Active {
			return ErrEndpointInactive
		}
		if _, err := store.GetEndpointWebhookDelivery(ctx, tx, endpointID, deliveryID); err != nil {
			return err
		}
		ok, err := store.RequeueWebhookDelivery(ctx, tx, endpointID, deliveryID, s.d.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return ErrDeliveryPending
		}
		return s.record(ctx, tx, p, es, ip, "webhook.redelivered", endpointID, map[string]any{"delivery_id": deliveryID})
	})
	if err != nil {
		return nil, err
	}
	return s.loadDelivery(ctx, endpointID, deliveryID)
}

// PurgeHistory applies the delivery log retention: finished deliveries
// older than Deps.Retention are deleted, never-attempted test deliveries
// older than an hour are failed, and unreferenced test events are dropped.
// It returns the number of deleted deliveries.
func (s *Service) PurgeHistory(ctx context.Context) (int64, error) {
	now := s.d.Clock.Now()
	if _, err := store.FailAbandonedWebhookDeliveries(ctx, s.d.DB, now.Add(-time.Hour), now, "test interrupted"); err != nil {
		return 0, err
	}
	n, err := store.PurgeWebhookDeliveries(ctx, s.d.DB, now.Add(-s.d.Retention))
	if err != nil {
		return 0, err
	}
	if _, err := store.DeleteDispatchedOutboxEventsOfType(ctx, s.d.DB, TestEventType, now.Add(-s.d.Retention)); err != nil {
		return n, err
	}
	return n, nil
}

// ---- helpers --------------------------------------------------------------------

func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, ip, action, id string, diff map[string]any) error {
	return s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: action,
		EntityType: "webhook_endpoint", EntityID: id, Diff: diff,
	})
}

func (s *Service) loadDelivery(ctx context.Context, endpointID, deliveryID string) (*Delivery, error) {
	d, err := store.GetEndpointWebhookDelivery(ctx, s.d.DB, endpointID, deliveryID)
	if err != nil {
		return nil, err
	}
	types, err := store.OutboxEventTypes(ctx, s.d.DB, []string{d.EventID})
	if err != nil {
		return nil, err
	}
	v := deliveryView(d, types[d.EventID])
	return &v, nil
}

func (s *Service) endpointView(ep *store.WebhookEndpoint) Endpoint {
	v := Endpoint{
		ID: ep.ID, URL: ep.URL, Description: ep.Description, Events: append([]string{}, ep.Events...),
		Active: ep.Active, Status: EndpointStatusOf(ep), ConsecutiveFailures: ep.ConsecutiveFailures,
		DisabledAt: ep.DisabledAt, LastDeliveryAt: ep.LastDeliveryAt, CreatedAt: ep.CreatedAt, UpdatedAt: ep.UpdatedAt,
	}
	if ep.DisabledReason != nil {
		v.DisabledReason = *ep.DisabledReason
	}
	if ep.PreviousSecretExpiresAt != nil && s.d.Clock.Now().Before(*ep.PreviousSecretExpiresAt) {
		v.PreviousSecretExpiresAt = ep.PreviousSecretExpiresAt
	}
	return v
}

// EndpointStatusOf derives the endpoint state (pure).
func EndpointStatusOf(ep *store.WebhookEndpoint) EndpointStatus {
	switch {
	case ep.Active:
		return EndpointActive
	case ep.DisabledAt != nil:
		return EndpointDisabled
	default:
		return EndpointPaused
	}
}

// DeliveryStatusOf derives the delivery state (pure).
func DeliveryStatusOf(d *store.WebhookDelivery) DeliveryStatus {
	switch {
	case d.DeliveredAt != nil:
		return DeliveryDelivered
	case d.FailedAt != nil:
		return DeliveryFailed
	default:
		return DeliveryPending
	}
}

func deliveryView(d *store.WebhookDelivery, eventType string) Delivery {
	v := Delivery{
		ID: d.ID, EndpointID: d.EndpointID, EventID: event.IDPrefix + d.EventID, EventType: eventType,
		Status: DeliveryStatusOf(d), Attempts: d.Attempt, StatusCode: d.StatusCode,
		DeliveredAt: d.DeliveredAt, FailedAt: d.FailedAt, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if v.Status == DeliveryPending {
		v.NextAttemptAt = d.NextAttemptAt
	}
	if d.LastError != nil {
		v.LastError = *d.LastError
	}
	return v
}

func change(from, to any) map[string]any { return map[string]any{"from": from, "to": to} }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
