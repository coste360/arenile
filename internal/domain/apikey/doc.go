// Package apikey manages the API keys of an establishment from the panel
// (spec 16, 17, 22): list, create with optional expiry, rename, change
// scopes or expiry, revoke.
//
// Ownership (M5A): the "integrations backend" builder owns this package,
// the handlers in internal/http/admin/apikeys.go and the repository
// functions in internal/store/apikey.go (new file; internal/store/auth.go
// keeps the existing InsertAPIKey, GetActiveAPIKeyByPrefix, TouchAPIKey,
// RevokeAPIKey).
//
// # Division of labour with internal/domain/auth (extend, don't duplicate)
//
// auth keeps the key format and the authentication path: NewAPIKey
// ("ar_live_<8 base32 prefix>_<secret>"), ParseAPIKey, HashToken (SHA-256,
// only the hash is stored), DisplayPrefix, AuthenticateAPIKey and the
// Principal with Kind PrincipalAPIKey and Scopes. This package uses those
// helpers and owns the management use cases. auth.Service.CreateAPIKey (M0,
// used by tests) stays as is; Create here is the panel path (expiry,
// scope restrictions, created_by_name). M5A change required in auth:
// AuthenticateAPIKey must reject keys whose expires_at is in the past
// (store.GetActiveAPIKeyByPrefix gains a "now" argument or the check is
// done after loading) — covered by a test that fails without it.
//
// # Rules
//
//   - Permission auth.PermUsersManage (owner). A principal of Kind
//     PrincipalAPIKey is always refused with auth.ErrForbidden: a key never
//     manages keys (nor webhooks).
//   - Scopes are auth.Permissions except auth.PermUsersManage; at least one;
//     duplicates removed; the creator must hold every scope on the
//     establishment (no privilege escalation).
//   - Name 1..100 runes, trimmed. ExpiresAt, when set, must be in the future
//     (clock.Clock). Status is derived: revoked (revoked_at set), expired
//     (expires_at <= now), active.
//   - The plaintext key is returned only by Create, never stored, never
//     logged, and the HTTP response carries Cache-Control: no-store. No
//     Idempotency-Key on creation (the stored replay would contain the key).
//   - Every change writes the audit log in the same transaction
//     ("api_key.created", "api_key.updated", "api_key.revoked"; diffs carry
//     name, scopes, display prefix, expires_at, never the key or its hash).
//   - Revoke is idempotent and immediate (AuthenticateAPIKey only loads
//     non-revoked keys).
package apikey
