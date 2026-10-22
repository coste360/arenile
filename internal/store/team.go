package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of the team area (users and memberships of an
// establishment, spec 16). A membership applies to an establishment when it
// belongs to the establishment's organization and is organization-wide
// (establishment_id NULL) or bound to that establishment.

// TeamMember is a user with the memberships that apply to one establishment.
type TeamMember struct {
	User        User
	Memberships []Membership
}

// TeamGrant is a membership that applies to an establishment, with the
// active flag of its user (used for the "last owner" rule).
type TeamGrant struct {
	MembershipID    string  `bun:"id"`
	UserID          string  `bun:"user_id"`
	EstablishmentID *string `bun:"establishment_id"`
	Role            string  `bun:"role"`
	UserActive      bool    `bun:"active"`
}

// teamApplicable restricts a select on memberships to those applying to the
// establishment; prefix qualifies the columns ("tm." or "").
func teamApplicable(q *bun.SelectQuery, es EstablishmentScope, prefix string) *bun.SelectQuery {
	return q.Where(prefix+"organization_id = ?", es.OrganizationID()).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where(prefix+"establishment_id IS NULL").WhereOr(prefix+"establishment_id = ?", es.ID())
		})
}

// ListTeamMembers lists, by user id, the users with at least one membership
// applying to the establishment, with those memberships.
func ListTeamMembers(ctx context.Context, db bun.IDB, es EstablishmentScope, cursor string, limit int) (Page[TeamMember], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[TeamMember]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[TeamMember]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	sub := teamApplicable(db.NewSelect().TableExpr("memberships AS tm").ColumnExpr("tm.user_id"), es, "tm.")
	var users []User
	q := db.NewSelect().Model(&users).Where("id IN (?)", sub).OrderExpr("id ASC").Limit(int64(limit + 1))
	if after != "" {
		q = q.Where("id > ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[TeamMember]{}, fmt.Errorf("store: list team members: %w", err)
	}
	page := Page[TeamMember]{}
	if len(users) > limit {
		users = users[:limit]
		page.NextCursor = EncodeCursor(users[limit-1].ID)
	}
	if len(users) == 0 {
		return page, nil
	}
	ids := make([]string, len(users))
	for i := range users {
		ids[i] = users[i].ID
	}
	ms, err := teamMemberships(ctx, db, es, ids)
	if err != nil {
		return Page[TeamMember]{}, err
	}
	page.Items = make([]TeamMember, len(users))
	for i := range users {
		page.Items[i] = TeamMember{User: users[i], Memberships: ms[users[i].ID]}
	}
	return page, nil
}

// GetTeamMember loads a user and the memberships applying to the
// establishment; ErrNotFound when the user has none.
func GetTeamMember(ctx context.Context, db bun.IDB, es EstablishmentScope, userID string) (*TeamMember, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	ms, err := teamMemberships(ctx, db, es, []string{userID})
	if err != nil {
		return nil, err
	}
	if len(ms[userID]) == 0 {
		return nil, ErrNotFound
	}
	u, err := GetUser(ctx, db, userID)
	if err != nil {
		return nil, err
	}
	return &TeamMember{User: *u, Memberships: ms[userID]}, nil
}

func teamMemberships(ctx context.Context, db bun.IDB, es EstablishmentScope, userIDs []string) (map[string][]Membership, error) {
	var ms []Membership
	q := db.NewSelect().Model(&ms).Where("user_id IN (?)", bun.List(userIDs)).OrderExpr("id ASC")
	if err := teamApplicable(q, es, "").Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: team memberships: %w", err)
	}
	out := make(map[string][]Membership, len(userIDs))
	for _, m := range ms {
		out[m.UserID] = append(out[m.UserID], m)
	}
	return out, nil
}

// ListTeamGrants returns every membership applying to the establishment with
// the active flag of its user.
func ListTeamGrants(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]TeamGrant, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []TeamGrant
	q := db.NewSelect().TableExpr("memberships AS tm").Join("JOIN users AS tu ON tu.id = tm.user_id").
		ColumnExpr("tm.id, tm.user_id, tm.establishment_id, tm.role, tu.active").OrderExpr("tm.id ASC")
	if err := teamApplicable(q, es, "tm.").Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list team grants: %w", err)
	}
	return out, nil
}

// TeamLockOrganization takes a write lock on the organization row without
// changing it, serializing concurrent team changes (last owner rule).
func TeamLockOrganization(ctx context.Context, db bun.IDB, es EstablishmentScope) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().Model((*Organization)(nil)).Set("updated_at = updated_at").
		Where("id = ?", es.OrganizationID()).Exec(ctx)
	return wrapWrite("lock organization", err)
}

// TeamUpdateMembershipRole changes the role of a membership of the
// establishment's organization.
func TeamUpdateMembershipRole(ctx context.Context, db bun.IDB, es EstablishmentScope, membershipID, role string, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewUpdate().Model((*Membership)(nil)).Set("role = ?", role).Set("updated_at = ?", at).
		Where("id = ?", membershipID).Where("organization_id = ?", es.OrganizationID()).Exec(ctx)
	if err != nil {
		return wrapWrite("update membership role", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TeamDeleteMembership deletes a membership of the establishment's
// organization.
func TeamDeleteMembership(ctx context.Context, db bun.IDB, es EstablishmentScope, membershipID string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewDelete().Model((*Membership)(nil)).
		Where("id = ?", membershipID).Where("organization_id = ?", es.OrganizationID()).Exec(ctx)
	if err != nil {
		return wrapWrite("delete membership", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TeamSetUserActive activates or deactivates a user account.
func TeamSetUserActive(ctx context.Context, db bun.IDB, userID string, active bool, at time.Time) error {
	res, err := db.NewUpdate().Model((*User)(nil)).Set("active = ?", active).Set("updated_at = ?", at).
		Where("id = ?", userID).Exec(ctx)
	if err != nil {
		return wrapWrite("set user active", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
