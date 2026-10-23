package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository of the notifications (internal/domain/notify): magic links of
// the manage area (magic_link_tokens), outgoing messages (message_log) and
// per-establishment template overrides (notification_templates).
//
// Every function takes an EstablishmentScope except the delivery-side
// functions used by the background job (ListDueMessages, ClaimMessage,
// FinishMessageDelivery), which work across establishments by design: the
// job is trusted system code and each row carries its establishment_id.

// ---- magic links ---------------------------------------------------------------

// InsertMagicLink inserts a magic link of es.
func InsertMagicLink(ctx context.Context, db bun.IDB, es EstablishmentScope, m *MagicLinkToken) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	m.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(m).Exec(ctx)
	return wrapWrite("insert magic link", err)
}

// FindActiveMagicLink returns the link of es with tokenHash that is neither
// revoked nor expired at now (ErrNotFound otherwise, without telling the
// cases apart).
func FindActiveMagicLink(ctx context.Context, db bun.IDB, es EstablishmentScope, tokenHash string, now time.Time) (*MagicLinkToken, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	m := new(MagicLinkToken)
	err := db.NewSelect().Model(m).
		Where("mlt.establishment_id = ?", es.ID()).
		Where("mlt.token_hash = ?", tokenHash).
		Where("mlt.revoked_at IS NULL").
		Where("mlt.expires_at > ?", now).
		Scan(ctx)
	return m, wrapRead("find magic link", err)
}

// TouchMagicLink records the last use of a link of es.
func TouchMagicLink(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().Model((*MagicLinkToken)(nil)).
		Set("last_used_at = ?", at).
		Where("establishment_id = ?", es.ID()).Where("id = ?", id).
		Exec(ctx)
	return wrapWrite("touch magic link", err)
}

// RevokeMagicLinks revokes the active links of a booking of es and returns
// how many were revoked. exceptIDs (optional) stay active.
func RevokeMagicLinks(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string, at time.Time, exceptIDs ...string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	q := db.NewUpdate().Model((*MagicLinkToken)(nil)).
		Set("revoked_at = ?", at).
		Where("establishment_id = ?", es.ID()).
		Where("booking_id = ?", bookingID).
		Where("revoked_at IS NULL")
	if keep := dedupe(exceptIDs); len(keep) > 0 {
		q = q.Where("id NOT IN (?)", bun.List(keep))
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return 0, wrapWrite("revoke magic links", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RevokeMagicLink revokes one link of es (no-op when already revoked).
func RevokeMagicLink(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().Model((*MagicLinkToken)(nil)).
		Set("revoked_at = ?", at).
		Where("establishment_id = ?", es.ID()).Where("id = ?", id).Where("revoked_at IS NULL").
		Exec(ctx)
	return wrapWrite("revoke magic link", err)
}

// FindBookingByCodeAndEmail returns the id of the booking of es with code
// whose customer has the (normalised) email; ErrNotFound otherwise.
// Anonymized customers never match.
func FindBookingByCodeAndEmail(ctx context.Context, db bun.IDB, es EstablishmentScope, code, normalizedEmail string) (string, error) {
	if err := es.mustBeValid(); err != nil {
		return "", err
	}
	var id string
	err := db.NewSelect().TableExpr("bookings AS bk").
		ColumnExpr("bk.id").
		Join("JOIN customers AS cu ON cu.id = bk.customer_id AND cu.establishment_id = bk.establishment_id").
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.code = ?", code).
		Where("LOWER(cu.email) = ?", normalizedEmail).
		Where("cu.anonymized_at IS NULL").
		Limit(1).
		Scan(ctx, &id)
	if err != nil {
		return "", wrapRead("find booking by code and email", err)
	}
	return id, nil
}

// ---- notification templates ----------------------------------------------------

// GetNotificationTemplate returns the enabled override of template in
// locale for es (ErrNotFound when the built-in template applies).
func GetNotificationTemplate(ctx context.Context, db bun.IDB, es EstablishmentScope, template, locale string) (*NotificationTemplate, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(NotificationTemplate)
	err := db.NewSelect().Model(t).
		Where("ntt.establishment_id = ?", es.ID()).
		Where("ntt.template = ?", template).
		Where("ntt.locale = ?", locale).
		Where("ntt.enabled = ?", true).
		Scan(ctx)
	return t, wrapRead("get notification template", err)
}

// ---- message log -----------------------------------------------------------------

// InsertMessage queues m for es unless a message with the same dedupe key
// exists; it returns the stored row (m itself, or the existing one) and
// whether it was inserted now. Safe inside a PostgreSQL transaction (ON
// CONFLICT DO NOTHING never aborts it).
func InsertMessage(ctx context.Context, db bun.IDB, es EstablishmentScope, m *Message) (*Message, bool, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, false, err
	}
	m.EstablishmentID = es.ID()
	if m.Payload == nil {
		m.Payload = map[string]any{}
	}
	res, err := db.NewInsert().Model(m).On("CONFLICT (establishment_id, dedupe_key) DO NOTHING").Exec(ctx)
	if err != nil {
		return nil, false, wrapWrite("insert message", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return m, true, nil
	}
	existing := new(Message)
	err = db.NewSelect().Model(existing).
		Where("msg.establishment_id = ?", es.ID()).Where("msg.dedupe_key = ?", m.DedupeKey).Scan(ctx)
	if err != nil {
		return nil, false, wrapRead("get message by dedupe key", err)
	}
	return existing, false, nil
}

// GetMessage loads a message of es.
func GetMessage(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Message, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	m := new(Message)
	err := db.NewSelect().Model(m).Where("msg.establishment_id = ?", es.ID()).Where("msg.id = ?", id).Scan(ctx)
	return m, wrapRead("get message", err)
}

// MessageFilter filters ListMessages.
type MessageFilter struct {
	BookingID string
	Status    string
}

// ListMessages lists the messages of es, newest first (keyset on the
// UUIDv7 id).
func ListMessages(ctx context.Context, db bun.IDB, es EstablishmentScope, f MessageFilter, cursor string, limit int) (Page[Message], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Message]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[Message]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []Message
	q := db.NewSelect().Model(&items).
		Where("msg.establishment_id = ?", es.ID()).
		OrderExpr("msg.id DESC").
		Limit(int64(limit + 1))
	if f.BookingID != "" {
		q = q.Where("msg.booking_id = ?", f.BookingID)
	}
	if f.Status != "" {
		q = q.Where("msg.status = ?", f.Status)
	}
	if before != "" {
		q = q.Where("msg.id < ?", before)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[Message]{}, fmt.Errorf("store: list messages: %w", err)
	}
	page := Page[Message]{Items: items}
	if page.Items == nil {
		page.Items = []Message{}
	}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	return page, nil
}

// LatestBookingMessage returns the newest message of a booking of es with
// one of the templates (ErrNotFound when none), cancelled ones excluded.
func LatestBookingMessage(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string, templates ...string) (*Message, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	m := new(Message)
	err := db.NewSelect().Model(m).
		Where("msg.establishment_id = ?", es.ID()).
		Where("msg.booking_id = ?", bookingID).
		Where("msg.template IN (?)", bun.List(templates)).
		Where("msg.status <> ?", MessageCancelled).
		OrderExpr("msg.id DESC").
		Limit(1).
		Scan(ctx)
	return m, wrapRead("latest booking message", err)
}

// ListDueMessages returns up to limit messages ready for a delivery
// attempt across establishments, oldest first: queued or failed (retry)
// with next_attempt_at <= now, plus "sending" rows not updated since
// staleBefore (a worker died mid-send). Dead letters (failed with no
// next_attempt_at) are never returned. Delivery job only.
func ListDueMessages(ctx context.Context, db bun.IDB, now, staleBefore time.Time, limit int) ([]Message, error) {
	var out []Message
	err := db.NewSelect().Model(&out).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				WhereOr("msg.status IN (?) AND msg.next_attempt_at IS NOT NULL AND msg.next_attempt_at <= ?", bun.List([]string{MessageQueued, MessageFailed}), now).
				WhereOr("msg.status = ? AND msg.updated_at < ?", MessageSending, staleBefore)
		}).
		OrderExpr("msg.next_attempt_at ASC, msg.id ASC").
		Limit(int64(limit)).
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list due messages: %w", err)
	}
	return out, nil
}

// ClaimMessage moves a due message to "sending" with a conditional update
// (optimistic claim: only one worker or instance wins). It reports whether
// the caller owns the attempt.
func ClaimMessage(ctx context.Context, db bun.IDB, id string, now, staleBefore time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*Message)(nil)).
		Set("status = ?", MessageSending).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.
				WhereOr("status IN (?) AND next_attempt_at IS NOT NULL AND next_attempt_at <= ?", bun.List([]string{MessageQueued, MessageFailed}), now).
				WhereOr("status = ? AND updated_at < ?", MessageSending, staleBefore)
		}).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("claim message", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// FinishMessageDelivery writes the outcome of an attempt on a claimed
// ("sending") message: status, attempts, next_attempt_at, last_error,
// provider_message_id, sent_at and recipient_enc (cleared on cancellation).
// It reports false when the row is no longer in "sending" (lost claim).
func FinishMessageDelivery(ctx context.Context, db bun.IDB, m *Message) (bool, error) {
	res, err := db.NewUpdate().Model(m).
		Column("status", "attempts", "next_attempt_at", "last_error", "provider_message_id", "sent_at", "recipient_enc", "updated_at").
		Where("id = ?", m.ID).
		Where("establishment_id = ?", m.EstablishmentID).
		Where("status = ?", MessageSending).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("finish message delivery", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ForgetCustomerMessages applies the anonymization of a customer of es to
// the message log (spec 23): pending messages are cancelled and the
// encrypted recipient of every message is erased (the hash stays, so the
// log keeps its shape without personal data). It returns the rows changed.
func ForgetCustomerMessages(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, now time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	cancelled, err := db.NewUpdate().Model((*Message)(nil)).
		Set("status = ?", MessageCancelled).
		Set("next_attempt_at = NULL").
		Set("last_error = ?", "customer anonymized").
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("customer_id = ?", customerID).
		Where("status IN (?)", bun.List([]string{MessageQueued, MessageFailed})).
		Exec(ctx)
	if err != nil {
		return 0, wrapWrite("cancel customer messages", err)
	}
	wiped, err := db.NewUpdate().Model((*Message)(nil)).
		Set("recipient_enc = NULL").
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("customer_id = ?", customerID).
		Where("recipient_enc IS NOT NULL").
		Exec(ctx)
	if err != nil {
		return 0, wrapWrite("wipe customer message recipients", err)
	}
	a, _ := cancelled.RowsAffected()
	b, _ := wiped.RowsAffected()
	return int(a + b), nil
}
