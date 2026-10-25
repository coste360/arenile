package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository of the push notifications (owner: internal/domain/push,
// migration 00013, ADR 0027). Operational functions take an
// EstablishmentScope; the deliverer's queue functions (DuePushDeliveries,
// ClaimPushDelivery, Finish*, DeleteClosedPushDeliveries) and the retention
// of registrations (ListPushRecipientStates, DeleteStalePushRegistration)
// are system operations of the job runner that act on one row by id.

// ---- settings ---------------------------------------------------------------------

// GetPushSettings returns the settings row of es (ErrNotFound = defaults).
func GetPushSettings(ctx context.Context, db bun.IDB, es EstablishmentScope) (*PushSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := new(PushSettings)
	err := db.NewSelect().Model(row).Where("pss.establishment_id = ?", es.ID()).Limit(1).Scan(ctx)
	return row, wrapRead("get push settings", err)
}

// InsertPushSettings inserts the first settings row of es (ErrConflict when
// a concurrent insert won).
func InsertPushSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, row *PushSettings) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	row.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(row).Exec(ctx)
	return wrapWrite("insert push settings", err)
}

// UpdatePushSettings replaces the settings of es if the stored version is
// oldVersion; it reports whether it applied.
func UpdatePushSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, row *PushSettings, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	row.EstablishmentID = es.ID()
	res, err := db.NewUpdate().Model(row).
		Column("enabled", "notify_new_online_booking", "notify_approval_requested", "notify_payment_failed", "version", "updated_at").
		Where("establishment_id = ?", es.ID()).Where("version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update push settings", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ---- registrations ------------------------------------------------------------------

// GetPushRegistrationByDevice loads the registration of a device of es.
func GetPushRegistrationByDevice(ctx context.Context, db bun.IDB, es EstablishmentScope, deviceID string) (*PushRegistration, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	r := new(PushRegistration)
	err := db.NewSelect().Model(r).Where("pr.establishment_id = ?", es.ID()).Where("pr.device_id = ?", deviceID).Scan(ctx)
	return r, wrapRead("get push registration", err)
}

// ReplacePushRegistration stores r as the only registration of its device
// and of its token in es: it deletes any registration of another device
// with the same token hash (a phone paired again), then updates the
// registration of the device in place when one exists, keeping its id (set
// in r.ID) so its queued deliveries survive and the (registration_id,
// event_id) dedupe of a redelivered event keeps holding; otherwise it
// inserts r. Run it in a transaction; a concurrent replacement surfaces as
// ErrConflict.
func ReplacePushRegistration(ctx context.Context, db bun.IDB, es EstablishmentScope, r *PushRegistration) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	r.EstablishmentID = es.ID()
	if _, err := db.NewDelete().Model((*PushRegistration)(nil)).
		Where("establishment_id = ?", es.ID()).
		Where("token_hash = ?", r.TokenHash).Where("device_id <> ?", r.DeviceID).
		Exec(ctx); err != nil {
		return wrapWrite("delete previous push registration", err)
	}
	prev := new(PushRegistration)
	err := db.NewSelect().Model(prev).Column("id").
		Where("pr.establishment_id = ?", es.ID()).Where("pr.device_id = ?", r.DeviceID).Limit(1).Scan(ctx)
	switch {
	case err == nil:
		r.ID = prev.ID
		_, err = db.NewUpdate().Model(r).
			Column("user_id", "platform", "environment", "locale", "topics", "token_ciphertext", "token_hash", "updated_at").
			Where("establishment_id = ?", es.ID()).Where("id = ?", r.ID).Exec(ctx)
		return wrapWrite("update push registration", err)
	case !isNoRows(err):
		return wrapRead("get push registration", err)
	}
	_, err = db.NewInsert().Model(r).Exec(ctx)
	return wrapWrite("insert push registration", err)
}

// DeletePushRegistrationByDevice deletes the registration of a device of
// es (its deliveries cascade) and reports whether one existed.
func DeletePushRegistrationByDevice(ctx context.Context, db bun.IDB, es EstablishmentScope, deviceID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewDelete().Model((*PushRegistration)(nil)).
		Where("establishment_id = ?", es.ID()).Where("device_id = ?", deviceID).Exec(ctx)
	if err != nil {
		return false, wrapWrite("delete push registration", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeletePushRegistration deletes a registration of es by id (the relay
// answered that the token is gone).
func DeletePushRegistration(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewDelete().Model((*PushRegistration)(nil)).
		Where("establishment_id = ?", es.ID()).Where("id = ?", id).Exec(ctx)
	return wrapWrite("delete push registration", err)
}

func pushRecipientQuery(db bun.IDB, es EstablishmentScope) *bun.SelectQuery {
	return db.NewSelect().Model((*PushRegistration)(nil)).
		ColumnExpr("pr.*").
		ColumnExpr("dev.revoked_at AS device_revoked_at, dev.last_used_at AS device_last_used_at, dev.created_at AS device_created_at").
		ColumnExpr("u.active AS user_active").
		Join("JOIN devices AS dev ON dev.id = pr.device_id AND dev.establishment_id = pr.establishment_id").
		Join("JOIN users AS u ON u.id = pr.user_id").
		Where("pr.establishment_id = ?", es.ID())
}

// ListPushRecipients returns every registration of es with the state of its
// device and user (the caller filters topics, roles and expiry).
func ListPushRecipients(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]PushRecipient, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []PushRecipient
	err := pushRecipientQuery(db, es).OrderExpr("pr.id ASC").Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list push recipients: %w", err)
	}
	return out, nil
}

// GetPushRecipient loads one registration of es with its device and user
// state (ErrNotFound when it was deleted).
func GetPushRecipient(ctx context.Context, db bun.IDB, es EstablishmentScope, registrationID string) (*PushRecipient, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	r := new(PushRecipient)
	err := pushRecipientQuery(db, es).Where("pr.id = ?", registrationID).Limit(1).Scan(ctx, r)
	return r, wrapRead("get push recipient", err)
}

// PushRecipientState is a registration (never its token) with the state of
// its device and user, across establishments (retention).
type PushRecipientState struct {
	ID               string     `bun:"id"`
	UpdatedAt        time.Time  `bun:"updated_at"`
	DeviceRevokedAt  *time.Time `bun:"device_revoked_at"`
	DeviceLastUsedAt *time.Time `bun:"device_last_used_at"`
	DeviceCreatedAt  time.Time  `bun:"device_created_at"`
	UserActive       bool       `bun:"user_active"`
}

// ListPushRecipientStates returns every registration of every
// establishment with the state of its device and user, so the retention job
// can apply the device and user rules of the domain (system operation of
// the job runner; no token is read).
func ListPushRecipientStates(ctx context.Context, db bun.IDB) ([]PushRecipientState, error) {
	var out []PushRecipientState
	err := db.NewSelect().Model((*PushRegistration)(nil)).
		ColumnExpr("pr.id, pr.updated_at").
		ColumnExpr("dev.revoked_at AS device_revoked_at, dev.last_used_at AS device_last_used_at, dev.created_at AS device_created_at").
		ColumnExpr("u.active AS user_active").
		Join("JOIN devices AS dev ON dev.id = pr.device_id AND dev.establishment_id = pr.establishment_id").
		Join("JOIN users AS u ON u.id = pr.user_id").
		OrderExpr("pr.id ASC").Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list push recipient states: %w", err)
	}
	return out, nil
}

// DeleteStalePushRegistration deletes a registration by id (its deliveries
// cascade) only if it was not updated since updatedAt, so a registration
// renewed after the retention job read it survives; it reports whether it
// deleted (system operation of the job runner).
func DeleteStalePushRegistration(ctx context.Context, db bun.IDB, id string, updatedAt time.Time) (bool, error) {
	res, err := db.NewDelete().Model((*PushRegistration)(nil)).
		Where("id = ?", id).Where("updated_at = ?", updatedAt).Exec(ctx)
	if err != nil {
		return false, wrapWrite("delete stale push registration", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ---- deliveries ---------------------------------------------------------------------

// InsertPushDelivery queues d unless the registration already has a
// delivery for the event; it reports whether d was inserted.
func InsertPushDelivery(ctx context.Context, db bun.IDB, es EstablishmentScope, d *PushDelivery) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	d.EstablishmentID = es.ID()
	res, err := db.NewInsert().Model(d).On("CONFLICT (registration_id, event_id) DO NOTHING").Exec(ctx)
	if err != nil {
		return false, wrapWrite("insert push delivery", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DuePushDelivery is a delivery the deliverer may claim.
type DuePushDelivery struct {
	ID              string `bun:"id"`
	EstablishmentID string `bun:"establishment_id"`
}

// DuePushDeliveries lists up to limit deliveries due at now: queued and due,
// or sending with an expired lease (a crashed worker).
func DuePushDeliveries(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]DuePushDelivery, error) {
	var out []DuePushDelivery
	err := db.NewSelect().Model((*PushDelivery)(nil)).Column("id", "establishment_id").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("status = ? AND next_attempt_at <= ?", PushDeliveryQueued, now)
			}).WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("status = ? AND locked_until < ?", PushDeliverySending, now)
			})
		}).
		OrderExpr("next_attempt_at ASC, id ASC").Limit(int64(limit)).Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: due push deliveries: %w", err)
	}
	return out, nil
}

// ClaimPushDelivery takes a delivery for one attempt with a lease: the
// conditional update succeeds for exactly one of concurrent workers.
// Reclaiming an expired lease counts the lost attempt. ErrNotFound when
// another worker has it.
func ClaimPushDelivery(ctx context.Context, db bun.IDB, id string, now, leaseUntil time.Time) (*PushDelivery, error) {
	res, err := db.NewUpdate().Model((*PushDelivery)(nil)).
		Set("attempts = CASE WHEN status = ? THEN attempts + 1 ELSE attempts END", PushDeliverySending).
		Set("status = ?", PushDeliverySending).Set("locked_until = ?", leaseUntil).Set("updated_at = ?", now).
		Where("id = ?", id).
		WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.WhereGroup(" OR ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
				return q.Where("status = ? AND next_attempt_at <= ?", PushDeliveryQueued, now)
			}).WhereGroup(" OR ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
				return q.Where("status = ? AND locked_until < ?", PushDeliverySending, now)
			})
		}).Exec(ctx)
	if err != nil {
		return nil, wrapWrite("claim push delivery", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNotFound
	}
	d := new(PushDelivery)
	err = db.NewSelect().Model(d).Where("pd.id = ?", id).Scan(ctx)
	return d, wrapRead("get claimed push delivery", err)
}

// FinishPushDeliverySent marks a claimed delivery sent (only while this
// worker still holds the lease).
func FinishPushDeliverySent(ctx context.Context, db bun.IDB, id string, lease, now time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*PushDelivery)(nil)).
		Set("status = ?", PushDeliverySent).Set("sent_at = ?", now).Set("attempts = attempts + 1").
		Set("locked_until = NULL").Set("next_attempt_at = NULL").Set("last_error = NULL").Set("updated_at = ?", now).
		Where("id = ?", id).Where("status = ?", PushDeliverySending).Where("locked_until = ?", lease).Exec(ctx)
	if err != nil {
		return false, wrapWrite("finish push delivery", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// FinishPushDeliveryUnsent records an attempt that did not deliver: back to
// queued at nextAttempt, or closed with status (failed or expired) when
// nextAttempt is nil. countAttempt is false when nothing was sent (the
// delivery expired or its recipient is no longer eligible).
func FinishPushDeliveryUnsent(ctx context.Context, db bun.IDB, id string, lease, now time.Time, nextAttempt *time.Time, status, lastError string, countAttempt bool) (bool, error) {
	if nextAttempt != nil {
		status = PushDeliveryQueued
	}
	q := db.NewUpdate().Model((*PushDelivery)(nil)).
		Set("status = ?", status).Set("next_attempt_at = ?", nextAttempt).
		Set("locked_until = NULL").Set("last_error = ?", lastError).Set("updated_at = ?", now)
	if countAttempt {
		q = q.Set("attempts = attempts + 1")
	}
	res, err := q.Where("id = ?", id).Where("status = ?", PushDeliverySending).Where("locked_until = ?", lease).Exec(ctx)
	if err != nil {
		return false, wrapWrite("finish push delivery", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteClosedPushDeliveries deletes the sent, failed and expired
// deliveries last updated before cutoff (retention).
func DeleteClosedPushDeliveries(ctx context.Context, db bun.IDB, cutoff time.Time) (int64, error) {
	res, err := db.NewDelete().Model((*PushDelivery)(nil)).
		Where("status IN (?)", bun.List([]string{PushDeliverySent, PushDeliveryFailed, PushDeliveryExpired})).
		Where("updated_at < ?", cutoff).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("delete closed push deliveries", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListPushDeliveries returns the deliveries of es, oldest first (tests and
// diagnostics; never exposes tokens).
func ListPushDeliveries(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]PushDelivery, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []PushDelivery
	err := db.NewSelect().Model(&out).Where("pd.establishment_id = ?", es.ID()).OrderExpr("pd.created_at ASC, pd.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list push deliveries: %w", err)
	}
	return out, nil
}
