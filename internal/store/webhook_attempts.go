package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Full attempt history of outgoing webhook deliveries
// (webhook_delivery_attempts, 00009, ADR 0019 F): one row per HTTP attempt,
// written by event.Deliverer in the transaction of the attempt outcome and
// purged by the webhook retention job.

// InsertWebhookDeliveryAttempt records one attempt (idempotent on
// (delivery_id, attempt): a replayed outcome inserts nothing).
func InsertWebhookDeliveryAttempt(ctx context.Context, db bun.IDB, a *WebhookDeliveryAttempt) error {
	_, err := db.NewInsert().Model(a).On("CONFLICT (delivery_id, attempt) DO NOTHING").Exec(ctx)
	return wrapWrite("insert webhook delivery attempt", err)
}

// ListWebhookDeliveryAttempts returns a page of the attempts of a delivery
// of es, newest first.
func ListWebhookDeliveryAttempts(ctx context.Context, db bun.IDB, es EstablishmentScope, deliveryID, cursor string, limit int) (Page[WebhookDeliveryAttempt], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[WebhookDeliveryAttempt]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[WebhookDeliveryAttempt]{}, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var items []WebhookDeliveryAttempt
	q := db.NewSelect().Model(&items).
		Where("wda.establishment_id = ?", es.ID()).Where("wda.delivery_id = ?", deliveryID)
	if before != "" {
		q = q.Where("wda.id < ?", before)
	}
	if err := q.OrderExpr("wda.id DESC").Limit(int64(limit + 1)).Scan(ctx); err != nil && !isNoRows(err) {
		return Page[WebhookDeliveryAttempt]{}, fmt.Errorf("store: list webhook delivery attempts: %w", err)
	}
	page := Page[WebhookDeliveryAttempt]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	return page, nil
}

// PurgeWebhookDeliveryAttempts deletes the attempts made before cutoff and
// returns how many (retention, ARENILE_WEBHOOK_ATTEMPT_RETENTION_DAYS).
func PurgeWebhookDeliveryAttempts(ctx context.Context, db bun.IDB, cutoff time.Time) (int64, error) {
	res, err := db.NewDelete().Model((*WebhookDeliveryAttempt)(nil)).Where("attempted_at < ?", cutoff).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("purge webhook delivery attempts", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
