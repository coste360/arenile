// Package device owns the paired devices of an establishment (spec 29.2,
// shared by the desktop app in remote mode, M6, and the mobile app, M7):
// one-time pairing codes shown as a QR in the panel, the exchange of a code
// for a per-device token, device token authentication, listing and
// revocation.
//
// Ownership (M6): the "devices" builder owns this package, the handlers in
// internal/http/admin/devices.go and internal/http/device (pair, self), the
// repository functions in internal/store/device.go and the models devices,
// device_pairing_codes (internal/store/models_device.go).
//
// # Architecture (functional core, imperative shell)
//
//   - token.go, qr.go (pure, tested): token format ar_dev_<prefix>_<secret>
//     (NewToken, ParseToken, HashToken), pairing codes of 8 Crockford base32
//     characters shown as ABCD-EFGH (NewPairingCode, NormalizePairingCode,
//     HashPairingCode) and the QR text arenile://pair?d=<base64url JSON>
//     (EncodeQR, DecodeQR).
//   - service.go (shell): use cases with *auth.Principal, each in one
//     store.RunInTx with an audit entry ("device.pairing_code_created",
//     "device.paired", "device.revoked"; never the code or the token).
//   - authenticator.go: BearerAuthenticator (strategy on the token scheme:
//     ar_dev_ -> device, anything else -> API key) for /api/admin, and
//     DeviceOnly for /api/device.
//
// # Ports (declared here, implemented elsewhere, asserted in internal/app/m6.go)
//
//   - Principals (auth.Service): builds the device principal with the
//     user's grants narrowed to the device establishment.
//   - Instance: the address and TLS fingerprint put in the QR (base URL on
//     the server edition; LAN URL and self-signed certificate fingerprint on
//     the desktop edition in LAN mode; ErrLANModeRequired in local mode).
//   - BridgeRevoker (fiscalbridge.Service): deactivates the fiscal bridge
//     registration of a revoked device in the same transaction.
//
// # Invariants
//
//   - Tokens and codes are stored only as SHA-256 hex (token_hash,
//     code_hash); the raw values appear once in an API response and never
//     in logs or audit diffs.
//   - A pairing code is consumed at most once: UPDATE ... SET used_at WHERE
//     used_at IS NULL AND expires_at > now; zero rows -> ErrPairingCodeInvalid
//     (unknown, expired and used codes are indistinguishable). Covered by a
//     TestConcurrency_ test with 100 parallel exchanges of one code.
//   - A revoked device (revoked_at set), an inactive user or a user without
//     a role on the establishment never authenticates.
//   - last_used_at is touched at most once a minute per device.
//
// # Permissions (spec 16.1)
//
// Any authenticated user creates codes for themselves and lists / revokes
// their own devices; codes for another user and the devices of others
// require auth.PermUsersManage.
package device
