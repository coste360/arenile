// Package push implements the optional push notifications of the mobile
// app through the Coste360 relay (spec 29.2, ADR 0027).
//
// Consent has three levels: the server operator sets a relay key
// (Relay.Configured), the manager turns push on for the establishment
// (push_settings, off by default) and the user registers the native
// APNs/FCM token of a paired device from the app (push_registrations).
//
// Flow:
//
//	outbox event -> Sink (topic, recipients) -> push_deliveries (queued)
//	push.deliver job -> claim with lease -> re-check eligibility -> Relay.Send
//
//	booking.confirmed (online channel) -> booking_new_online          (bookings.read)
//	booking.approval_requested         -> booking_approval_requested  (bookings.write)
//	payment.failed (provider payments) -> payment_failed              (refunds.issue)
//
// Privacy: what leaves the instance is the closed pushrelay.Payload: the
// native token, the platform, the APNs environment and a message key with
// its locale. Event payloads, names, codes and amounts never reach the
// relay. Tokens are encrypted at rest with the key ring (AAD = device id)
// and never logged, audited or returned.
//
// Idempotency: one delivery per (registration, event) by unique index, so a
// redelivered event (at-least-once outbox) queues nothing new; the claim is
// a conditional update with a lease, so two workers (or two instances)
// never send the same row at the same time. No network call happens in a
// request or inside a transaction.
package push
