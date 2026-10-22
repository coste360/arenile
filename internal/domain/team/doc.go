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
//
// Decisions (M1):
//   - New accounts get the password given by the owner or a generated
//     temporary one returned once (TeamMemberCreated.initial_password, sent
//     with Cache-Control: no-store). No email is sent: password reset and
//     invitations by email belong to the notifications module (spec 19).
//   - After a role/scope change the member has exactly one membership
//     applying to the establishment; moving to scope "establishment" deletes
//     the organization-wide membership (and its access to the other
//     establishments). Organization-wide changes, and deactivating accounts
//     with access beyond this establishment, need an organization-wide owner.
//   - Deactivation is account-wide (users.active, sessions deleted) and is
//     refused for accounts that also belong to other organizations.
//   - The last-owner rule (CheckLastOwner) runs under a lock on the
//     organization row.
package team
