package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// ---- outbox --------------------------------------------------------------------

// InsertOutboxEvent appends an event to the outbox. Call it with the same
// bun.Tx as the state change it describes.
func InsertOutboxEvent(ctx context.Context, db bun.IDB, e *OutboxEvent) error {
	_, err := db.NewInsert().Model(e).Exec(ctx)
	return wrapWrite("insert outbox event", err)
}

// ListPendingOutboxEvents returns undispatched events, oldest first.
func ListPendingOutboxEvents(ctx context.Context, db bun.IDB, limit int) ([]OutboxEvent, error) {
	var evs []OutboxEvent
	err := db.NewSelect().Model(&evs).Where("dispatched_at IS NULL").OrderExpr("created_at ASC, id ASC").Limit(int64(limit)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list pending outbox: %w", err)
	}
	return evs, nil
}

// MarkOutboxEventDispatched marks an event dispatched; it reports false if it
// was already dispatched by someone else.
func MarkOutboxEventDispatched(ctx context.Context, db bun.IDB, id string, at time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*OutboxEvent)(nil)).
		Set("dispatched_at = ?", at).Where("id = ?", id).Where("dispatched_at IS NULL").Exec(ctx)
	if err != nil {
		return false, wrapWrite("mark outbox dispatched", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// GetOutboxEvent loads an event by id.
func GetOutboxEvent(ctx context.Context, db bun.IDB, id string) (*OutboxEvent, error) {
	e := new(OutboxEvent)
	err := db.NewSelect().Model(e).Where("id = ?", id).Scan(ctx)
	return e, wrapRead("get outbox event", err)
}

// ---- webhooks ------------------------------------------------------------------

// InsertWebhookEndpoint inserts an endpoint in the establishment of es.
func InsertWebhookEndpoint(ctx context.Context, db bun.IDB, es EstablishmentScope, e *WebhookEndpoint) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	e.EstablishmentID = es.ID()
	if e.Events == nil {
		e.Events = []string{}
	}
	_, err := db.NewInsert().Model(e).Exec(ctx)
	return wrapWrite("insert webhook endpoint", err)
}

// GetWebhookEndpoint loads an endpoint of the establishment of es.
func GetWebhookEndpoint(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*WebhookEndpoint, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	e := new(WebhookEndpoint)
	err := db.NewSelect().Model(e).Where("id = ?", id).Where("establishment_id = ?", es.ID()).Scan(ctx)
	return e, wrapRead("get webhook endpoint", err)
}

// ListActiveWebhookEndpoints lists the active endpoints of an establishment.
// Used by the dispatcher (system scope from the event itself).
func ListActiveWebhookEndpoints(ctx context.Context, db bun.IDB, establishmentID string) ([]WebhookEndpoint, error) {
	var es []WebhookEndpoint
	err := db.NewSelect().Model(&es).
		Where("establishment_id = ?", establishmentID).Where("active = ?", true).
		OrderExpr("id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list webhook endpoints: %w", err)
	}
	return es, nil
}

// GetWebhookEndpointByID loads an endpoint by id (delivery job, system scope).
func GetWebhookEndpointByID(ctx context.Context, db bun.IDB, id string) (*WebhookEndpoint, error) {
	e := new(WebhookEndpoint)
	err := db.NewSelect().Model(e).Where("id = ?", id).Scan(ctx)
	return e, wrapRead("get webhook endpoint", err)
}

// RecordWebhookEndpointResult resets (success) or increments (failure) the
// consecutive failure counter and returns the new value.
func RecordWebhookEndpointResult(ctx context.Context, db bun.IDB, id string, success bool, at time.Time) (int, error) {
	q := db.NewUpdate().Model((*WebhookEndpoint)(nil)).Set("updated_at = ?", at).Where("id = ?", id)
	if success {
		q = q.Set("consecutive_failures = 0")
	} else {
		q = q.Set("consecutive_failures = consecutive_failures + 1")
	}
	if _, err := q.Exec(ctx); err != nil {
		return 0, wrapWrite("record webhook result", err)
	}
	var n int
	err := db.NewSelect().Model((*WebhookEndpoint)(nil)).Column("consecutive_failures").Where("id = ?", id).Scan(ctx, &n)
	return n, wrapRead("read webhook failures", err)
}

// DisableWebhookEndpoint deactivates an endpoint with a reason.
func DisableWebhookEndpoint(ctx context.Context, db bun.IDB, id, reason string, at time.Time) error {
	_, err := db.NewUpdate().Model((*WebhookEndpoint)(nil)).
		Set("active = ?", false).Set("disabled_at = ?", at).Set("disabled_reason = ?", reason).Set("updated_at = ?", at).
		Where("id = ?", id).Exec(ctx)
	return wrapWrite("disable webhook endpoint", err)
}

// InsertWebhookDeliveryIfAbsent creates a delivery for (endpoint, event)
// unless it already exists.
func InsertWebhookDeliveryIfAbsent(ctx context.Context, db bun.IDB, d *WebhookDelivery) error {
	_, err := db.NewInsert().Model(d).On("CONFLICT (endpoint_id, event_id) DO NOTHING").Exec(ctx)
	return wrapWrite("insert webhook delivery", err)
}

// ListDueWebhookDeliveries returns deliveries whose next attempt is due.
func ListDueWebhookDeliveries(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]WebhookDelivery, error) {
	var ds []WebhookDelivery
	err := db.NewSelect().Model(&ds).
		Where("delivered_at IS NULL").Where("failed_at IS NULL").
		Where("next_attempt_at IS NOT NULL").Where("next_attempt_at <= ?", now).
		OrderExpr("next_attempt_at ASC").Limit(int64(limit)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list due deliveries: %w", err)
	}
	return ds, nil
}

// UpdateWebhookDelivery persists the outcome of a delivery attempt.
func UpdateWebhookDelivery(ctx context.Context, db bun.IDB, d *WebhookDelivery) error {
	_, err := db.NewUpdate().Model(d).
		Column("attempt", "status_code", "response_excerpt", "last_error", "next_attempt_at", "delivered_at", "failed_at", "updated_at").
		WherePK().Exec(ctx)
	return wrapWrite("update webhook delivery", err)
}

// GetWebhookDelivery loads a delivery by id.
func GetWebhookDelivery(ctx context.Context, db bun.IDB, id string) (*WebhookDelivery, error) {
	d := new(WebhookDelivery)
	err := db.NewSelect().Model(d).Where("id = ?", id).Scan(ctx)
	return d, wrapRead("get webhook delivery", err)
}

// ---- idempotency ---------------------------------------------------------------

// ClaimIdempotencyKey inserts a pending key. It reports false when the key
// already exists (the existing row is then returned).
func ClaimIdempotencyKey(ctx context.Context, db bun.IDB, k *IdempotencyKey) (bool, *IdempotencyKey, error) {
	res, err := db.NewInsert().Model(k).On("CONFLICT (scope, key) DO NOTHING").Exec(ctx)
	if err != nil {
		return false, nil, wrapWrite("claim idempotency key", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return true, k, nil
	}
	existing := new(IdempotencyKey)
	if err := db.NewSelect().Model(existing).Where("scope = ?", k.Scope).Where("key = ?", k.Key).Scan(ctx); err != nil {
		return false, nil, wrapRead("load idempotency key", err)
	}
	return false, existing, nil
}

// CompleteIdempotencyKey stores the response of a claimed key.
func CompleteIdempotencyKey(ctx context.Context, db bun.IDB, scope, key string, status int, contentType string, body []byte) error {
	_, err := db.NewUpdate().Model((*IdempotencyKey)(nil)).
		Set("response_status = ?", status).Set("response_content_type = ?", contentType).Set("response_body = ?", body).
		Where("scope = ?", scope).Where("key = ?", key).Exec(ctx)
	return wrapWrite("complete idempotency key", err)
}

// ReleaseIdempotencyKey deletes a claimed key (the request failed and may be retried).
func ReleaseIdempotencyKey(ctx context.Context, db bun.IDB, scope, key string) error {
	_, err := db.NewDelete().Model((*IdempotencyKey)(nil)).Where("scope = ?", scope).Where("key = ?", key).Exec(ctx)
	return wrapWrite("release idempotency key", err)
}

// DeleteExpiredIdempotencyKeys removes expired keys.
func DeleteExpiredIdempotencyKeys(ctx context.Context, db bun.IDB, now time.Time) (int64, error) {
	res, err := db.NewDelete().Model((*IdempotencyKey)(nil)).Where("expires_at <= ?", now).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("delete expired idempotency keys", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
