// Package testmode implements the test mode of an establishment (spec
// 15.4, ADR 0019): the panel switch, the preview link of the public page
// and the bulk purge of test bookings.
//
// Ownership (deferred features, builder A): this package, the handlers in
// internal/http/admin/testmode.go, the repository functions on
// store.TestModeSettings (new file internal/store/testmode.go) and the small
// hooks listed below in other packages.
//
// # Rules
//
//   - No row in test_mode_settings = off. Enabled is read inside the
//     transaction that creates a booking or a payment (booking, checkout,
//     cash services) and copied to bookings.test / payments.test; a payment
//     of a booking always inherits the booking flag. refund_requests.test
//     follows the booking.
//   - Test rows are excluded from reports, KPIs (Oggi), corrispettivi,
//     fiscal emission, list exports and outgoing webhooks/emails to real
//     customers (emails are still sent, with a "[PROVA]" subject prefix).
//     Every such query filters "test = false" (index *_test_idx is partial
//     on test rows, so the filter is cheap).
//   - Stripe: while enabled, the checkout uses payment_provider_settings with
//     mode = test for the active provider when it exists (TestMode.
//     stripe_test_configured), otherwise the manual driver.
//   - Public page: while enabled and !PublicEnabled, createPublicBooking
//     requires a valid X-Arenile-Preview token (HMAC of establishment id and
//     expiry with the key ring, 2 hours) and answers 403
//     test_mode_preview_required otherwise; PublicEstablishment.test_mode is
//     true when the request may book in test mode (banner).
//   - Purge: one transaction, only test = true rows of the establishment
//     (bookings and their allocations, payments, intents, refund requests,
//     fiscal documents, message log); audit_log rows stay; one audit entry
//     "test_bookings.purged" with the counts. Never touches test = false.
//   - Switch changes write audit "establishment.test_mode_changed" and the
//     outbox event of the same name in the same transaction; the admin SPA
//     shows a persistent banner in the shell header while enabled.
package testmode
