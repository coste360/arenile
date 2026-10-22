package event

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Dispatcher moves events out of the outbox: it calls the in-process sinks,
// publishes to the real-time bus, schedules webhook deliveries and marks the
// event dispatched. Delivery to sinks is at-least-once.
type Dispatcher struct {
	db        bun.IDB
	clock     clock.Clock
	sinks     []ext.EventSink
	bus       ext.EventBus
	log       *slog.Logger
	batchSize int
}

// NewDispatcher builds a dispatcher. bus may be nil.
func NewDispatcher(db bun.IDB, c clock.Clock, sinks []ext.EventSink, bus ext.EventBus, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Dispatcher{db: db, clock: c, sinks: sinks, bus: bus, log: log, batchSize: 100}
}

// DispatchPending processes up to one batch of pending events and returns
// how many were dispatched.
func (d *Dispatcher) DispatchPending(ctx context.Context) (int, error) {
	rows, err := store.ListPendingOutboxEvents(ctx, d.db, d.batchSize)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		ev := FromRow(row)
		for _, s := range d.sinks {
			if err := d.callSink(ctx, s, ev); err != nil {
				d.log.WarnContext(ctx, "event sink failed", slog.String("sink", s.Name()), slog.String("event_id", ev.ID),
					slog.String("event_type", ev.Type), slog.String("error", err.Error()))
			}
		}
		if d.bus != nil {
			if err := d.bus.Publish(ctx, ev); err != nil {
				d.log.WarnContext(ctx, "event bus publish failed", slog.String("event_id", ev.ID), slog.String("error", err.Error()))
			}
		}
		now := d.clock.Now()
		err := store.RunInTx(ctx, d.db, func(ctx context.Context, tx bun.Tx) error {
			fresh, err := store.MarkOutboxEventDispatched(ctx, tx, row.ID, now)
			if err != nil || !fresh {
				return err
			}
			if ev.EstablishmentID == "" {
				return nil
			}
			endpoints, err := store.ListActiveWebhookEndpoints(ctx, tx, ev.EstablishmentID)
			if err != nil {
				return err
			}
			for i := range endpoints {
				if !endpoints[i].Matches(ev.Type) {
					continue
				}
				next := now
				del := &store.WebhookDelivery{
					ID: store.NewID(), EndpointID: endpoints[i].ID, EventID: row.ID,
					NextAttemptAt: &next, CreatedAt: now, UpdatedAt: now,
				}
				if err := store.InsertWebhookDeliveryIfAbsent(ctx, tx, del); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return n, fmt.Errorf("event: dispatch %s: %w", ev.ID, err)
		}
		n++
	}
	return n, nil
}

func (d *Dispatcher) callSink(ctx context.Context, s ext.EventSink, ev ext.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return s.Handle(ctx, ev)
}

// ---- webhook delivery ------------------------------------------------------------

// DeliveryOptions configure webhook delivery.
type DeliveryOptions struct {
	// MaxAge is how long a delivery is retried before giving up (default 24h).
	MaxAge time.Duration
	// DisableAfter consecutive failed attempts disables the endpoint (default 20).
	DisableAfter int
	// Timeout of each HTTP attempt (default 10s).
	Timeout time.Duration
	// UserAgent sent with deliveries.
	UserAgent string
}

// Deliverer sends due webhook deliveries.
type Deliverer struct {
	db     bun.IDB
	clock  clock.Clock
	keys   *crypto.KeyRing
	client *http.Client
	audit  *audit.Writer
	log    *slog.Logger
	opts   DeliveryOptions
}

// NewDeliverer builds a Deliverer. client may be nil (a client with the
// configured timeout is used).
func NewDeliverer(db bun.IDB, c clock.Clock, keys *crypto.KeyRing, client *http.Client, aw *audit.Writer, log *slog.Logger, opts DeliveryOptions) *Deliverer {
	if opts.MaxAge <= 0 {
		opts.MaxAge = 24 * time.Hour
	}
	if opts.DisableAfter <= 0 {
		opts.DisableAfter = 20
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "Arenile-Webhooks/1"
	}
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
// returns the plaintext signing secret (shown once; stored encrypted).
func (d *Deliverer) CreateEndpoint(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in CreateEndpointInput) (*store.WebhookEndpoint, string, error) {
	if d.keys == nil {
		return nil, "", errors.New("event: secret key not configured")
	}
	secret, err := NewWebhookSecret()
	if err != nil {
		return nil, "", err
	}
	enc, err := d.keys.EncryptString(secret)
	if err != nil {
		return nil, "", fmt.Errorf("event: encrypt webhook secret: %w", err)
	}
	now := d.clock.Now()
	ep := &store.WebhookEndpoint{ID: store.NewID(), URL: in.URL, Secret: enc, Events: in.Events, Active: true, CreatedAt: now, UpdatedAt: now}
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
	for i := range due {
		if err := ctx.Err(); err != nil {
			return i, err
		}
		if err := d.attempt(ctx, &due[i]); err != nil {
			d.log.ErrorContext(ctx, "webhook attempt", slog.String("delivery_id", due[i].ID), slog.String("error", err.Error()))
		}
	}
	return len(due), nil
}

func (d *Deliverer) attempt(ctx context.Context, del *store.WebhookDelivery) error {
	ep, err := store.GetWebhookEndpointByID(ctx, d.db, del.EndpointID)
	if err != nil {
		return err
	}
	now := d.clock.Now()
	if !ep.Active {
		reason := "endpoint disabled"
		del.LastError, del.FailedAt, del.NextAttemptAt, del.UpdatedAt = &reason, &now, nil, now
		return store.UpdateWebhookDelivery(ctx, d.db, del)
	}
	row, err := store.GetOutboxEvent(ctx, d.db, del.EventID)
	if err != nil {
		return err
	}
	body, err := envelopeJSON(FromRow(*row))
	if err != nil {
		return err
	}
	secret, err := d.keys.DecryptString(ep.Secret)
	if err != nil {
		return fmt.Errorf("event: decrypt endpoint secret: %w", err)
	}

	status, excerpt, sendErr := d.send(ctx, ep.URL, secret, body, row, now)
	del.Attempt++
	del.UpdatedAt = d.clock.Now()
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
		if next.Sub(del.CreatedAt) > d.opts.MaxAge {
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
		if d.audit == nil {
			return nil
		}
		return d.audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: ep.EstablishmentID, Actor: audit.SystemActor, Action: "webhook.disabled",
			EntityType: "webhook_endpoint", EntityID: ep.ID, Diff: map[string]any{"reason": reason},
		})
	})
}

func (d *Deliverer) send(ctx context.Context, url, secret string, body []byte, row *store.OutboxEvent, now time.Time) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, "", fmt.Errorf("invalid endpoint url: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", d.opts.UserAgent)
	req.Header.Set(SignatureHeader, Sign(secret, body, now))
	req.Header.Set("Arenile-Event-Id", IDPrefix+row.ID)
	req.Header.Set("Arenile-Event-Type", row.Type)
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	excerpt := string(b)
	for !utf8.ValidString(excerpt) && len(excerpt) > 0 {
		excerpt = excerpt[:len(excerpt)-1]
	}
	return resp.StatusCode, excerpt, nil
}
