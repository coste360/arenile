// Package stripe is the core Stripe payment driver (spec 13.2): an
// ext.PaymentProvider registered as "stripe" in the application registry.
//
// Ownership (M4): the "payment gateway" builder.
//
// # Design
//
//   - Stateless: the driver never touches the database; per-establishment
//     credentials arrive decrypted in every call (ext.Credentials, keys
//     CredSecretKey, CredWebhookSecret, CredPublishableKey; ADR 0006).
//   - No SDK: plain net/http against the Stripe REST API with form-encoded
//     requests and a pinned Stripe-Version header (APIVersion). No new Go
//     dependency (license and supply-chain surface stay unchanged); the API
//     base URL is configurable so tests run against an httptest.Server.
//   - CreatePayment: Stripe Checkout Session in redirect mode by default
//     (mode=payment, line item with the booking description, amount in
//     cents, currency, customer_email, success_url / cancel_url,
//     client_reference_id = booking id, metadata establishment_id /
//     booking_id / booking_code, payment_intent_data.metadata the same,
//     expires_at = now + 30 minutes); header Idempotency-Key =
//     CreatePaymentRequest.IdempotencyKey ("<booking_id>:<attempt>").
//     PreferredMode embedded -> PaymentIntent with automatic payment
//     methods and ClientSecret (Payment Element), when enabled.
//   - ParseWebhook: verifies Stripe-Signature (t=..., v1=... HMAC-SHA256 of
//     "<t>.<raw body>" with the webhook secret, constant-time compare,
//     tolerance 5 minutes against the injected clock, multiple v1 values
//     accepted for secret rotation); maps checkout.session.completed and
//     payment_intent.succeeded -> ext.PaymentSucceeded,
//     payment_intent.payment_failed -> ext.PaymentFailed (FailureReason =
//     last_payment_error.code, never the message), charge.refunded ->
//     ext.PaymentRefunded (amount = amount_refunded delta of the event);
//     other types -> ext.ErrIgnoredEvent. ProviderRef is the Checkout
//     Session id for checkout events and the PaymentIntent id otherwise;
//     metadata carries booking and establishment ids.
//   - Refund: POST /v1/refunds with payment_intent and amount, idempotent.
//   - Optional capabilities (gateway.PaymentStatusChecker,
//     gateway.CredentialsVerifier): PaymentStatus retrieves a Checkout
//     Session or PaymentIntent; VerifyCredentials calls GET /v1/account and
//     returns the account display name.
//   - Errors: network errors and 5xx/429 wrap gateway-agnostic
//     ErrUnavailable (mapped to 503 payment_provider_unavailable by the
//     gateway); 4xx are returned with the Stripe error code only. Never log
//     keys, signatures or raw bodies.
package stripe
