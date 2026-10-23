// Package waitlist implements the waitlist (spec 10.5, 19, 23; M5B):
// requests for a date range / pack / optional zone or spot that found no
// availability, kept in priority order (priority ASC, then created_at), and
// offered automatically when inventory frees.
//
// Flow:
//   - Join: from the panel (any channel) or the public page (only when
//     waitlist_settings.public_join; 202 without data, no enumeration). The
//     public entry gets a personal link token (256 bit, only its SHA-256 in
//     waitlist_entries.access_token_hash) sent by email
//     (notify.TemplateWaitlistJoined).
//   - Trigger: the service is an ext.EventSink (Name "waitlist") that reacts
//     to allocation.released, booking.cancelled, booking.expired and
//     season.day_released (cause cancellation/release/expiry), idempotent on
//     the event id. For the freed dates it scans compatible waiting entries
//     in priority order and, when auto_offer, creates an offer for the first
//     one that fits: a hold (holds + allocations, DB-enforced, never an
//     app-level check) of offer_hold_minutes, a waitlist_offers row
//     (pending; one pending offer per entry and per hold, unique indexes),
//     entry -> offered, audit + outbox waitlist.offered in the same
//     transaction, then the email with the link (notify.TemplateWaitlistOffer).
//     A conflict on the hold (someone booked first) just tries the next
//     entry / gives up.
//   - Accept (link): converts the offer hold into a held booking with a
//     checkout session (same response as createPublicBooking) and the
//     customer attached; the customer continues in the M4 checkout (pay,
//     webhook confirms). Offer -> accepted, entry -> converted.
//   - Decline / expiry: offer -> declined/expired, hold released, entry back
//     to waiting (or removed with leave=true); the freed inventory triggers
//     the next entry. JobExpire (every minute) expires pending offers past
//     expires_at and entries whose ends_on is in the past.
//   - Remove (panel or link): entry -> removed, pending offer revoked.
//
// GDPR (spec 23): entries cascade with the customer; anonymisation removes
// the customer's entries (status removed, token cleared). Events and logs
// carry ids only.
package waitlist
