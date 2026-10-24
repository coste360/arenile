package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository of the in-panel notifications (admin_notifications, 00009,
// ADR 0019 F). Owner-audience rows are visible only to users.manage,
// manager-audience rows to refunds.issue (NotificationVisibility); read
// state is shared by the staff.

// InsertAdminNotification inserts n for es unless a row with the same
// dedupe key exists (producers are idempotent). It reports whether a row
// was inserted.
func InsertAdminNotification(ctx context.Context, db bun.IDB, es EstablishmentScope, n *AdminNotification) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n.EstablishmentID = es.ID()
	if n.Params == nil {
		n.Params = map[string]string{}
	}
	res, err := db.NewInsert().Model(n).On("CONFLICT (establishment_id, dedupe_key) DO NOTHING").Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: insert admin notification: %w", err)
	}
	k, _ := res.RowsAffected()
	return k > 0, nil
}

// NotificationVisibility is what a principal may see besides the staff
// notifications: Owner (users.manage) and Manager (refunds.issue) rows.
type NotificationVisibility struct {
	Owner   bool
	Manager bool
}

// audiences lists the audience values visible to v.
func (v NotificationVisibility) audiences() []string {
	out := []string{NotificationAudienceStaff}
	if v.Owner {
		out = append(out, NotificationAudienceOwner)
	}
	if v.Manager {
		out = append(out, NotificationAudienceManager)
	}
	return out
}

func notificationAudience(q *bun.SelectQuery, v NotificationVisibility) *bun.SelectQuery {
	return q.Where("an.audience IN (?)", bun.List(v.audiences()))
}

// ListAdminNotifications returns a page of the notifications of es, newest
// first (UUIDv7 id order).
func ListAdminNotifications(ctx context.Context, db bun.IDB, es EstablishmentScope, unreadOnly bool, v NotificationVisibility, cursor string, limit int) (Page[AdminNotification], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[AdminNotification]{}, err
	}
	before, err := DecodeCursor(cursor)
	if err != nil {
		return Page[AdminNotification]{}, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var items []AdminNotification
	q := db.NewSelect().Model(&items).Where("an.establishment_id = ?", es.ID())
	q = notificationAudience(q, v)
	if unreadOnly {
		q = q.Where("an.read_at IS NULL")
	}
	if before != "" {
		q = q.Where("an.id < ?", before)
	}
	if err := q.OrderExpr("an.id DESC").Limit(int64(limit + 1)).Scan(ctx); err != nil && !isNoRows(err) {
		return Page[AdminNotification]{}, fmt.Errorf("store: list admin notifications: %w", err)
	}
	page := Page[AdminNotification]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	return page, nil
}

// CountUnreadAdminNotifications counts the unread notifications of es.
func CountUnreadAdminNotifications(ctx context.Context, db bun.IDB, es EstablishmentScope, v NotificationVisibility) (int64, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	q := db.NewSelect().Model((*AdminNotification)(nil)).Where("an.establishment_id = ?", es.ID()).Where("an.read_at IS NULL")
	n, err := notificationAudience(q, v).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count admin notifications: %w", err)
	}
	return int64(n), nil
}

// MarkAdminNotificationRead marks one notification of es as read (no-op when
// already read). ErrNotFound when it does not exist or is not visible.
func MarkAdminNotificationRead(ctx context.Context, db bun.IDB, es EstablishmentScope, id, userID string, v NotificationVisibility, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	var n AdminNotification
	q := db.NewSelect().Model(&n).Where("an.id = ?", id).Where("an.establishment_id = ?", es.ID())
	if err := notificationAudience(q, v).Scan(ctx); err != nil {
		return wrapRead("get admin notification", err)
	}
	if n.ReadAt != nil {
		return nil
	}
	_, err := db.NewUpdate().Model((*AdminNotification)(nil)).
		Set("read_at = ?", at).Set("read_by_user_id = ?", notificationUser(userID)).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("read_at IS NULL").Exec(ctx)
	return wrapWrite("mark admin notification read", err)
}

// MarkAllAdminNotificationsRead marks every visible unread notification of es
// as read and returns how many.
func MarkAllAdminNotificationsRead(ctx context.Context, db bun.IDB, es EstablishmentScope, userID string, v NotificationVisibility, at time.Time) (int64, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	q := db.NewUpdate().Model((*AdminNotification)(nil)).
		Set("read_at = ?", at).Set("read_by_user_id = ?", notificationUser(userID)).
		Where("establishment_id = ?", es.ID()).Where("read_at IS NULL")
	q = q.Where("audience IN (?)", bun.List(v.audiences()))
	res, err := q.Exec(ctx)
	if err != nil {
		return 0, wrapWrite("mark admin notifications read", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// OwnerRecipients returns the active users with role owner on the
// establishment of es (organization-wide or establishment memberships).
func OwnerRecipients(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]User, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	est, err := GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, err
	}
	var users []User
	err = db.NewSelect().Model(&users).
		Where("active = ?", true).
		Where("id IN (?)", db.NewSelect().Model((*Membership)(nil)).Column("user_id").
			Where("organization_id = ?", est.OrganizationID).Where("role = ?", "owner").
			WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Where("establishment_id IS NULL").WhereOr("establishment_id = ?", es.ID())
			})).
		OrderExpr("id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: owner recipients: %w", err)
	}
	return users, nil
}

func notificationUser(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
