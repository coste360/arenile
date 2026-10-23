// Package stripe is the core Stripe payment driver (spec 13.2): an
// ext.PaymentProvider registered as "stripe" in the application registry.
//
// Ownership (M4): the "payment gateway" builder.
//
// # Design
//
//   - Official SDK: github.com/stripe/stripe-go/v87 (MIT), API version
//     pinned by the SDK (APIVersion). The SDK backends are built once with
//     the configured HTTP client and base URL (tests point them at an
//     httptest.Server; no network in unit tests), retries limited to
//     MaxNetworkRetries with the same idempotency key, telemetry and SDK
//     logging off. ADR 0014 records why this supersedes the "no SDK"
//     sketch of ADR 0013 §7.
//   - Stateless: the driver never touches the database; per-establishment
//     credentials arrive decrypted in every call (ext.Credentials, keys
//     CredSecretKey, CredWebhookSecret, CredPublishableKey; ADR 0006) and a
//     short-lived SDK client is created per call around the shared backends.
//   - CreatePayment: Stripe Checkout Session in redirect mode by default
//     (mode=payment, one line item with the booking description, amount in
//     cents, currency, customer_email, success_url / cancel_url,
//     client_reference_id = booking id, metadata establishment_id /
//     booking_id / booking_code plus the gateway ids, the same metadata on
//     payment_intent_data, locale and expires_at from the request hints
//     HintLocale / HintExpiresAt, expiry clamped to Stripe's 30 min..24 h
//     window); the request IdempotencyKey is sent as Idempotency-Key.
//     PreferredMode embedded -> PaymentIntent with automatic payment
//     methods and ClientSecret (Payment Element).
//   - ParseWebhook: signature verified with the SDK (constant-time HMAC,
//     every v1 value tried for secret rotation), timestamp checked against
//     the injected clock (SignatureTolerance, replay protection); mapping
//     documented on ParseWebhook. Failure reasons are Stripe codes, never
//     messages.
//   - Refund: POST /v1/refunds on the PaymentIntent, idempotent.
//   - Optional capabilities (declared by the gateway, made of pkg/ext types):
//     PaymentStatus (reconciliation), VerifyCredentials ("test connection")
//     and CancelPayment (a new attempt expires the previous session).
//   - Errors: network errors, 5xx and 429 are ErrUnavailable (Temporary()
//     true, mapped to 503 payment_provider_unavailable by the gateway); a
//     reused idempotency key is ErrIdempotencyConflict; other 4xx are
//     *APIError with the Stripe type and code only. Keys, signatures, raw
//     bodies and Stripe messages are never logged or wrapped.
package stripe
