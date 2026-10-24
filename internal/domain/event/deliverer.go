package event

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/store"
)

// TestEventType is the type of the synthetic event sent by the panel "test"
// action (internal/domain/webhook.Service.SendTest). Its outbox row is
// written already dispatched, so only the tested endpoint receives it; its
// delivery is attempted once, synchronously, and never counts towards the
// automatic disable of the endpoint.
const TestEventType = "webhook.test"

// Delivery defaults.
const (
	DefaultMaxAge       = 24 * time.Hour
	DefaultDisableAfter = 20
	DefaultTimeout      = 10 * time.Second
	DefaultMaxPayload   = 256 << 10
	responseExcerptMax  = 1024
)

// ErrDeliveryNotPending is returned by AttemptNow when the delivery is
// already finished or leased by another worker.
var ErrDeliveryNotPending = errors.New("event: webhook delivery is not pending")

// DeliveryOptions configure webhook delivery.
type DeliveryOptions struct {
	// MaxAge is how long a delivery is retried, from its creation (or its
	// last redelivery), before it becomes failed: the dead-letter state,
	// kept in the delivery log and redeliverable from the panel (default 24h).
	MaxAge time.Duration
	// DisableAfter consecutive failed attempts disables the endpoint (default 20).
	DisableAfter int
	// Timeout of each HTTP attempt (default 10s).
	Timeout time.Duration
	// UserAgent sent with deliveries.
	UserAgent string
	// MaxPayload bounds the envelope size in bytes (default 256 KiB). A
	// larger envelope is failed without being sent.
	MaxPayload int
	// Egress is the SSRF policy of the default HTTP client (ignored when
	// NewDeliverer receives a client).
	Egress EgressPolicy
	// Resolver overrides the DNS resolver of the default client (tests).
	Resolver Resolver
}

// Deliverer sends due webhook deliveries: signed HTTP POSTs of the event
// envelope, retried with exponential Backoff, automatic disable of an
// endpoint after DisableAfter consecutive failures (audit
// "webhook.disabled"). Several Deliverers (instances) may run on the same
// database: each attempt is claimed with a lease first.
type Deliverer struct {
	db     bun.IDB
	clock  clock.Clock
	keys   *crypto.KeyRing
	client *http.Client
	audit  *audit.Writer
	log    *slog.Logger
	opts   DeliveryOptions
	// onDisabled is called in the transaction that auto-disables an
	// endpoint (owner notification, ADR 0019 F); nil = none.
	onDisabled EndpointDisabledFunc
}

// EndpointDisabledFunc is told, inside the transaction that disables it,
// that ep was disabled after failures consecutive failed deliveries. An
// error rolls the outcome back (the attempt is then retried).
type EndpointDisabledFunc func(ctx context.Context, tx bun.IDB, ep *store.WebhookEndpoint, failures int, at time.Time) error

// OnEndpointDisabled registers fn (wired by internal/app: the in-panel
// notification and the email to the owners).
func (d *Deliverer) OnEndpointDisabled(fn EndpointDisabledFunc) { d.onDisabled = fn }

// NewDeliverer builds a Deliverer. client may be nil: the default client is
// NewEgressClient(opts.Egress, opts.Resolver, opts.Timeout), which enforces
// the SSRF policy. A non-nil client is used as is (tests) and is then
// responsible for its own egress policy and redirect handling.
func NewDeliverer(db bun.IDB, c clock.Clock, keys *crypto.KeyRing, client *http.Client, aw *audit.Writer, log *slog.Logger, opts DeliveryOptions) *Deliverer {
	if opts.MaxAge <= 0 {
		opts.MaxAge = DefaultMaxAge
	}
	if opts.DisableAfter <= 0 {
		opts.DisableAfter = DefaultDisableAfter
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.MaxPayload <= 0 {
		opts.MaxPayload = DefaultMaxPayload
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "Arenile-Webhooks/1"
	}
	if client == nil {
		client = NewEgressClient(opts.Egress, opts.Resolver, opts.Timeout)
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Deliverer{db: db, clock: c, keys: keys, client: client, audit: aw, log: log, opts: opts}
}

// CreateEndpointInput describes a new webhook endpoint.
type CreateEndpointInput struct {
	URL    string
	Events []string
}

// CreateEndpoint registers an endpoint for the establishment of es and
// returns the plaintext signing secret (shown once; stored encrypted). It is
// the M0 primitive used by tests and system code; the panel uses
// internal/domain/webhook.Service.Create, which validates and audits.
func (d *Deliverer) CreateEndpoint(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in CreateEndpointInput) (*store.WebhookEndpoint, string, error) {
	if d.keys == nil {
		return nil, "", errors.New("event: secret key not configured")
	}
	secret, err := NewWebhookSecret()
	if err != nil {
		return nil, "", err
	}
	id := store.NewID()
	// The endpoint id is the AAD: a ciphertext copied onto another row does
	// not decrypt there.
	enc, err := d.keys.Encrypt([]byte(secret), []byte(id))
	if err != nil {
		return nil, "", fmt.Errorf("event: encrypt webhook secret: %w", err)
	}
	now := d.clock.Now()
	ep := &store.WebhookEndpoint{ID: id, URL: in.URL, Secret: enc, Events: in.Events, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := store.InsertWebhookEndpoint(ctx, db, es, ep); err != nil {
		return nil, "", err
	}
	return ep, secret, nil
}

// DeliverDue performs all due attempts (up to limit) and returns how many
// attempts were made.
func (d *Deliverer) DeliverDue(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	due, err := store.ListDueWebhookDeliveries(ctx, d.db, d.clock.Now(), limit)
	if err != nil {
		return 0, err
	}
	attempts := 0
	for i := range due {
		if err := ctx.Err(); err != nil {
			return attempts, err
		}
		// Claim the row with a lease before sending, so that another
		// instance polling the same database does not deliver it too. If this
		// process dies mid-attempt, the delivery becomes due again when the
		// lease expires.
		now := d.clock.Now()
		lease := now.Add(d.leaseDuration())
		claimed, err := store.ClaimWebhookDelivery(ctx, d.db, due[i].ID, now, lease)
		if err != nil {
			return attempts, err
		}
		if !claimed {
			continue
		}
		due[i].NextAttemptAt = &lease
		attempts++
		if err := d.attempt(ctx, &due[i]); err != nil {
			d.log.ErrorContext(ctx, "webhook attempt", slog.String("delivery_id", due[i].ID), slog.String("error", err.Error()))
		}
	}
	return attempts, nil
}

// AttemptNow performs one attempt of a pending delivery synchronously (the
// panel test action). The delivery is claimed with the same lease as
// DeliverDue, so a background worker never sends it at the same time; it
// returns ErrDeliveryNotPending when it is finished or leased. The outcome is
// stored on the delivery row (the caller reloads it); the returned error is
// only for infrastructure failures, not for a receiver answering non-2xx.
func (d *Deliverer) AttemptNow(ctx context.Context, deliveryID string) error {
	now := d.clock.Now()
	claimed, err := store.ClaimWebhookDeliveryNow(ctx, d.db, deliveryID, now, now.Add(d.leaseDuration()))
	if err != nil {
		return err
	}
	if !claimed {
		return ErrDeliveryNotPending
	}
	del, err := store.GetWebhookDelivery(ctx, d.db, deliveryID)
	if err != nil {
		return err
	}
	return d.attempt(ctx, del)
}

// leaseDuration bounds how long a claimed delivery stays invisible to other
// instances: the HTTP timeout plus a margin for the database writes.
func (d *Deliverer) leaseDuration() time.Duration { return 2*d.opts.Timeout + 30*time.Second }

func (d *Deliverer) attempt(ctx context.Context, del *store.WebhookDelivery) error {
	ep, err := store.GetWebhookEndpointByID(ctx, d.db, del.EndpointID)
	if err != nil {
		return err
	}
	row, err := store.GetOutboxEvent(ctx, d.db, del.EventID)
	if err != nil {
		return err
	}
	test := row.Type == TestEventType
	now := d.clock.Now()
	// A paused or disabled endpoint receives nothing, except the test event
	// (the owner may test an endpoint before activating it).
	if !ep.Active && !test {
		return d.finishWithoutSending(ctx, del, "endpoint disabled", now)
	}
	body, err := envelopeJSON(FromRow(*row))
	if err != nil {
		return err
	}
	if len(body) > d.opts.MaxPayload {
		return d.finishWithoutSending(ctx, del, fmt.Sprintf("payload too large (%d bytes, max %d)", len(body), d.opts.MaxPayload), now)
	}
	secrets, err := d.signingSecrets(ctx, ep, now)
	if err != nil {
		return err
	}

	status, excerpt, sendErr := d.send(ctx, ep.URL, secrets, body, row, now)
	elapsed := d.clock.Now().Sub(now)
	del.Attempt++
	del.UpdatedAt = d.clock.Now()
	del.StatusCode, del.ResponseExcerpt = nil, nil
	if status != 0 {
		del.StatusCode = &status
	}
	if excerpt != "" {
		del.ResponseExcerpt = &excerpt
	}
	success := sendErr == nil && status >= 200 && status < 300
	if success {
		t := d.clock.Now()
		del.DeliveredAt, del.NextAttemptAt, del.LastError = &t, nil, nil
	} else {
		msg := fmt.Sprintf("HTTP %d", status)
		if sendErr != nil {
			msg = sendErr.Error()
		}
		del.LastError = &msg
		next := d.clock.Now().Add(Backoff(del.Attempt))
		if test || next.Sub(del.CreatedAt) > d.opts.MaxAge {
			// Dead letter: no more automatic retries (a test is never retried).
			t := d.clock.Now()
			del.FailedAt, del.NextAttemptAt = &t, nil
		} else {
			del.NextAttemptAt = &next
		}
	}
	return store.RunInTx(ctx, d.db, func(ctx context.Context, tx bun.Tx) error {
		if err := store.UpdateWebhookDelivery(ctx, tx, del); err != nil {
			return err
		}
		if err := store.SetWebhookEndpointLastDelivery(ctx, tx, ep.ID, del.UpdatedAt); err != nil {
			return err
		}
		if err := store.InsertWebhookDeliveryAttempt(ctx, tx, attemptRow(ep, del, now, elapsed)); err != nil {
			return err
		}
		if test {
			return nil
		}
		failures, err := store.RecordWebhookEndpointResult(ctx, tx, ep.ID, success, del.UpdatedAt)
		if err != nil {
			return err
		}
		if success || failures < d.opts.DisableAfter {
			return nil
		}
		reason := fmt.Sprintf("disabled after %d consecutive failed deliveries", failures)
		if err := store.DisableWebhookEndpoint(ctx, tx, ep.ID, reason, del.UpdatedAt); err != nil {
			return err
		}
		d.log.WarnContext(ctx, "webhook endpoint disabled", slog.String("endpoint_id", ep.ID), slog.String("establishment_id", ep.EstablishmentID))
		if d.onDisabled != nil {
			if err := d.onDisabled(ctx, tx, ep, failures, del.UpdatedAt); err != nil {
				return err
			}
		}
		if d.audit == nil {
			return nil
		}
		return d.audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: ep.EstablishmentID, Actor: audit.SystemActor, Action: "webhook.disabled",
			EntityType: "webhook_endpoint", EntityID: ep.ID, Diff: map[string]any{"reason": reason},
		})
	})
}

// attemptRow is the history row of the attempt just made on del (the
// counters of del already include it).
func attemptRow(ep *store.WebhookEndpoint, del *store.WebhookDelivery, at time.Time, elapsed time.Duration) *store.WebhookDeliveryAttempt {
	a := &store.WebhookDeliveryAttempt{
		ID: store.NewID(), EstablishmentID: ep.EstablishmentID, EndpointID: ep.ID, DeliveryID: del.ID,
		Attempt: del.Attempt, StatusCode: del.StatusCode, Error: del.LastError, ResponseExcerpt: del.ResponseExcerpt,
		DurationMS: max(elapsed.Milliseconds(), 0), AttemptedAt: at,
	}
	if del.DeliveredAt != nil {
		a.Error = nil
	}
	return a
}

// finishWithoutSending fails a delivery that cannot be sent at all (endpoint
// inactive, payload too large): no HTTP request, no effect on the failure
// counter of the endpoint.
func (d *Deliverer) finishWithoutSending(ctx context.Context, del *store.WebhookDelivery, reason string, now time.Time) error {
	del.LastError, del.FailedAt, del.NextAttemptAt, del.UpdatedAt = &reason, &now, nil, now
	return store.UpdateWebhookDelivery(ctx, d.db, del)
}

// signingSecrets returns the plaintext secrets that sign a delivery: the
// current one and, during the grace period of a rotation, the previous one.
// A previous secret that does not decrypt is skipped (logged), it never
// blocks the delivery.
func (d *Deliverer) signingSecrets(ctx context.Context, ep *store.WebhookEndpoint, now time.Time) ([]string, error) {
	if d.keys == nil {
		return nil, errors.New("event: secret key not configured")
	}
	cur, err := d.keys.Decrypt(ep.Secret, []byte(ep.ID))
	if err != nil {
		return nil, fmt.Errorf("event: decrypt endpoint secret: %w", err)
	}
	out := []string{string(cur)}
	if ep.PreviousSecret != nil && ep.PreviousSecretExpiresAt != nil && now.Before(*ep.PreviousSecretExpiresAt) {
		prev, err := d.keys.Decrypt(*ep.PreviousSecret, []byte(PreviousSecretAAD(ep.ID)))
		if err != nil {
			d.log.WarnContext(ctx, "webhook previous secret does not decrypt", slog.String("endpoint_id", ep.ID))
		} else {
			out = append(out, string(prev))
		}
	}
	return out, nil
}

// PreviousSecretAAD is the additional data binding the previous secret
// ciphertext to its endpoint row and column.
func PreviousSecretAAD(endpointID string) string { return endpointID + ":previous" }

func (d *Deliverer) send(ctx context.Context, endpointURL string, secrets []string, body []byte, row *store.OutboxEvent, now time.Time) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(body))
	if err != nil {
		return 0, "", errors.New("invalid endpoint url")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", d.opts.UserAgent)
	req.Header.Set(SignatureHeader, SignAll(secrets, body, now))
	req.Header.Set("Arenile-Event-Id", IDPrefix+row.ID)
	req.Header.Set("Arenile-Event-Type", row.Type)
	resp, err := d.client.Do(req)
	if err != nil {
		// The *url.Error repeats the URL (which may carry a receiver token in
		// its query): keep only the cause in the delivery log.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, responseExcerptMax))
	// Drain a little more so the connection can be reused, never unbounded.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	excerpt := string(b)
	for !utf8.ValidString(excerpt) && len(excerpt) > 0 {
		excerpt = excerpt[:len(excerpt)-1]
	}
	return resp.StatusCode, excerpt, nil
}

// EnvelopeJSON returns the envelope bytes of an outbox row: exactly the body
// every delivery attempt of that event sends (and signs).
func EnvelopeJSON(row store.OutboxEvent) ([]byte, error) { return envelopeJSON(FromRow(row)) }
