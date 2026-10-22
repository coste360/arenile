// Package team manages the users of an establishment and their roles
// (spec 16): list members, create or invite a user with a role, change role,
// deactivate, revoke access. Every operation requires auth.PermUsersManage
// (owner only), except List which requires auth.PermBookingsRead (view).
//
// Ownership (M1): the "team" builder owns this package, the handlers in
// internal/http/admin/team.go and the repository functions in
// internal/store/team.go. Users and memberships tables already exist (00001);
// models are store.User and store.Membership.
//
// Rules: an owner can never remove or demote the last owner of an
// organization (ErrLastOwner); every mutation writes the audit log in the same
// transaction (actions "team.member_added", "team.role_changed",
// "team.member_deactivated", "team.member_removed"). Never log emails or
// passwords in clear.
package team
