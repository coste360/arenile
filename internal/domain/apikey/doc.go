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
// scope restrictions, created_by_name). auth.AuthenticateAPIKey rejects
// keys whose expires_at is reached (TestAPIKeyService_ExpiredKeyRejected).
//
// # Key format and storage
//
// "ar_live_<8 base32 chars prefix>_<52 base32 chars secret>": the secret is
// 32 random bytes (256 bits). Only SHA-256 of the whole key is stored
// (key_hash) and compared in constant time; the prefix is stored in clear for
// lookup and display. A slow hash (argon2id) or a pepper from the key ring
// would add nothing against a 256-bit random secret (no dictionary, no brute
// force is possible) while costing CPU on every API request (a cheap DoS
// lever on small self-hosted servers) and, for the pepper, binding every key
// to the current ARENILE_SECRET_KEY; passwords, chosen by people, keep
// argon2id.
//
// # Around the package
//
//   - Per-key request budget: internal/http/middleware.APIKeyRateLimit
//     (default 300/min, fixed window per instance), 429 with Retry-After.
//   - last_used_at is touched by AuthenticateAPIKey at most once a minute.
//   - Bookings and holds created with a key are always channel "api"
//     (auth.ChannelFor, called by booking and inventory): they consume and
//     respect the online quota (ADR 0011 §1, INV-52).
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
