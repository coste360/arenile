package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of the outgoing webhook management (M5A, spec 18.2).
// The delivery machinery of M0 (dispatch, due list, claim, attempt outcome)
// stays in events.go; this file adds what the panel needs: endpoint CRUD,
// secret rotation columns, the delivery log, redelivery, the synchronous
// claim of a single delivery and the retention purge.

// WebhookDeliveryState filters the delivery log.
type WebhookDeliveryState string

// Delivery states (derived from delivered_at / failed_at).
const (
	WebhookDeliveryAny       WebhookDeliveryState = ""
	WebhookDeliveryPending   WebhookDeliveryState = "pending"
	WebhookDeliveryDelivered WebhookDeliveryState = "delivered"
	WebhookDeliveryFailed    WebhookDeliveryState = "failed"
)

// ListWebhookEndpoints lists every endpoint of the establishment of es,
// oldest first (UUIDv7 ids are time ordered).
func ListWebhookEndpoints(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]WebhookEndpoint, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []WebhookEndpoint
	err := db.NewSelect().Model(&out).Where("establishment_id = ?", es.ID()).OrderExpr("id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list webhook endpoints: %w", err)
	}
	return out, nil
}

// UpdateWebhookEndpoint writes the given columns of e (plus updated_at) for
// an endpoint of the establishment of es. It returns ErrNotFound when the
// endpoint does not belong to es.
func UpdateWebhookEndpoint(ctx context.Context, db bun.IDB, es EstablishmentScope, e *WebhookEndpoint, columns ...string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if e.Events == nil {
		e.Events = []string{}
	}
	res, err := db.NewUpdate().Model(e).Column(append(columns, "updated_at")...).
		WherePK().Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("update webhook endpoint", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWebhookEndpoint deletes an endpoint of the establishment of es and
// its deliveries (explicitly, not relying on ON DELETE CASCADE being
// enabled on the SQLite connection).
func DeleteWebhookEndpoint(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if _, err := GetWebhookEndpoint(ctx, db, es, id); err != nil {
		return err
	}
	if _, err := db.NewDelete().Model((*WebhookDelivery)(nil)).Where("endpoint_id = ?", id).Exec(ctx); err != nil {
		return wrapWrite("delete webhook deliveries", err)
	}
	_, err := db.NewDelete().Model((*WebhookEndpoint)(nil)).Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	return wrapWrite("delete webhook endpoint", err)
}

// SetWebhookEndpointLastDelivery records the time of the last delivery
// attempt of an endpoint (delivery job, system scope).
func SetWebhookEndpointLastDelivery(ctx context.Context, db bun.IDB, id string, at time.Time) error {
	_, err := db.NewUpdate().Model((*WebhookEndpoint)(nil)).Set("last_delivery_at = ?", at).Where("id = ?", id).Exec(ctx)
	return wrapWrite("set webhook last delivery", err)
}

// ClaimWebhookDeliveryNow takes a pending delivery for one immediate attempt
// (test event, synchronous attempt) by moving next_attempt_at to leaseUntil.
// Unlike ClaimWebhookDelivery it also accepts a delivery that was never
// scheduled (next_attempt_at NULL). It reports false when the delivery is
// finished or currently leased by another worker, so a synchronous attempt
// never races a background one.
func ClaimWebhookDeliveryNow(ctx context.Context, db bun.IDB, id string, now, leaseUntil time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*WebhookDelivery)(nil)).
		Set("next_attempt_at = ?", leaseUntil).Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("delivered_at IS NULL").Where("failed_at IS NULL").
		WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.Where("next_attempt_at IS NULL").WhereOr("next_attempt_at <= ?", now)
		}).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("claim webhook delivery now", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// InsertWebhookDelivery inserts a delivery row (test event).
func InsertWebhookDelivery(ctx context.Context, db bun.IDB, d *WebhookDelivery) error {
	_, err := db.NewInsert().Model(d).Exec(ctx)
	return wrapWrite("insert webhook delivery", err)
}

// GetEndpointWebhookDelivery loads a delivery of the given endpoint (the
// caller has already proven the endpoint belongs to its establishment).
func GetEndpointWebhookDelivery(ctx context.Context, db bun.IDB, endpointID, id string) (*WebhookDelivery, error) {
	d := new(WebhookDelivery)
	err := db.NewSelect().Model(d).Where("id = ?", id).Where("endpoint_id = ?", endpointID).Scan(ctx)
	return d, wrapRead("get webhook delivery", err)
}

// ListEndpointWebhookDeliveries returns a page of the deliveries of an
// endpoint, newest first (UUIDv7 id order, stable across redeliveries),
// optionally filtered by state.
func ListEndpointWebhookDeliveries(ctx context.Context, db bun.IDB, endpointID string, state WebhookDeliveryState, cursor string, limit int) (Page[WebhookDelivery], error) {
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[WebhookDelivery]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []WebhookDelivery
	q := db.NewSelect().Model(&items).Where("endpoint_id = ?", endpointID).OrderExpr("id DESC").Limit(int64(limit + 1))
	switch state {
	case WebhookDeliveryPending:
		q = q.Where("delivered_at IS NULL").Where("failed_at IS NULL")
	case WebhookDeliveryDelivered:
		q = q.Where("delivered_at IS NOT NULL")
	case WebhookDeliveryFailed:
		q = q.Where("failed_at IS NOT NULL")
	}
	if before != "" {
		q = q.Where("id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[WebhookDelivery]{}, fmt.Errorf("store: list webhook deliveries: %w", err)
	}
	page := Page[WebhookDelivery]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(items[limit-1].ID)
	}
	return page, nil
}

// OutboxEventTypes returns the type of each listed outbox event, by id.
func OutboxEventTypes(ctx context.Context, db bun.IDB, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   string `bun:"id"`
		Type string `bun:"type"`
	}
	err := db.NewSelect().Model((*OutboxEvent)(nil)).Column("id", "type").Where("id IN (?)", bun.List(ids)).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: outbox event types: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.Type
	}
	return out, nil
}

// RequeueWebhookDelivery makes a finished delivery due again now: it clears
// delivered_at, failed_at and last_error, sets next_attempt_at = now and
// moves created_at to now, because created_at is the age reference of the
// retry window (DeliveryOptions.MaxAge): a redelivery gets a fresh 24 hours
// of retries. The attempt counter keeps counting. It reports false when the
// delivery is still pending (nothing to requeue).
func RequeueWebhookDelivery(ctx context.Context, db bun.IDB, endpointID, id string, now time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*WebhookDelivery)(nil)).
		Set("delivered_at = NULL").Set("failed_at = NULL").Set("last_error = NULL").
		Set("next_attempt_at = ?", now).Set("created_at = ?", now).Set("updated_at = ?", now).
		Where("id = ?", id).Where("endpoint_id = ?", endpointID).
		WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.Where("delivered_at IS NOT NULL").WhereOr("failed_at IS NOT NULL")
		}).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("requeue webhook delivery", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// PurgeWebhookDeliveries deletes finished deliveries (delivered or failed)
// last updated before cutoff and returns how many (retention).
func PurgeWebhookDeliveries(ctx context.Context, db bun.IDB, cutoff time.Time) (int64, error) {
	res, err := db.NewDelete().Model((*WebhookDelivery)(nil)).
		WhereGroup(" AND ", func(q *bun.DeleteQuery) *bun.DeleteQuery {
			return q.Where("delivered_at IS NOT NULL").WhereOr("failed_at IS NOT NULL")
		}).
		Where("updated_at < ?", cutoff).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("purge webhook deliveries", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// FailAbandonedWebhookDeliveries marks failed the pending deliveries that
// were never scheduled (next_attempt_at NULL: a synchronous test whose
// process stopped before the attempt) and were created before cutoff.
func FailAbandonedWebhookDeliveries(ctx context.Context, db bun.IDB, cutoff, now time.Time, reason string) (int64, error) {
	res, err := db.NewUpdate().Model((*WebhookDelivery)(nil)).
		Set("failed_at = ?", now).Set("last_error = ?", reason).Set("updated_at = ?", now).
		Where("delivered_at IS NULL").Where("failed_at IS NULL").Where("next_attempt_at IS NULL").
		Where("created_at < ?", cutoff).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("fail abandoned webhook deliveries", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteDispatchedOutboxEventsOfType deletes dispatched outbox events of one
// type created before cutoff that no delivery references any more (used for
// the synthetic webhook.test events).
func DeleteDispatchedOutboxEventsOfType(ctx context.Context, db bun.IDB, eventType string, cutoff time.Time) (int64, error) {
	refs := db.NewSelect().Model((*WebhookDelivery)(nil)).Column("event_id")
	res, err := db.NewDelete().Model((*OutboxEvent)(nil)).
		Where("type = ?", eventType).Where("dispatched_at IS NOT NULL").Where("created_at < ?", cutoff).
		Where("id NOT IN (?)", refs).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("delete outbox events", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
