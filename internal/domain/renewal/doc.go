// Package renewal implements season pass renewals with the right of
// pre-emption (spec 10.4, 19; M5B): campaigns from a source season to a
// target season, one proposal per pass holder, sending with a magic link,
// acceptance online (deposit or full payment through the M4 gateway, with
// optional customer credit) or from the panel, decline, bulk actions and the
// release of unrenewed spots at the deadline.
//
// Model (migration 00008):
//   - renewal_campaigns: unique (source, target) per establishment.
//   - renewal_proposals: unique (campaign, source pass) and at most one open
//     (proposed/accepted) proposal per source pass. Each proposal owns a
//     booking of the target season created through the booking service
//     (booking.CreateInput{Status: held, IsSeasonPass: true, Formula:
//     season, SeasonID: target, RenewalOfBookingID: source pass, same spot,
//     pack, slots and extras, Force}); the booking service caps the hold of
//     a new booking at 24 hours, so the hold is then moved to DeadlineAt in
//     the same transaction (booking.ExtendHold). The booking's allocations hold
//     the spot (DB-enforced, ADR 0003); the operational map already shows
//     held bookings with renewal_of_booking_id as `renewal_offer`.
//   - Price: the target season engine (booking service quote/pricing),
//     never copied from the old pass; previous_amount is informative.
//   - Period (PassInterval, plan.go): the whole target season when the
//     source pass ran to the end of its season, else its dates shifted by
//     the year difference (pricing.ShiftDate) and clamped.
//   - Credit (optional ports Credits / CreditApplier): shown and applied
//     only from the verified magic link, never in the anonymous checkout.
//
// Lifecycle (every change: audit + outbox in the same transaction):
//
//	proposed --accept(online: payment started)--> accepted (booking pending_payment)
//	proposed|accepted --payment webhook / manual driver / panel confirm--> converted (booking confirmed)
//	proposed|accepted --decline--> declined (booking cancelled, spot released)
//	proposed|accepted --deadline passed / campaign closed--> expired
//
// Confirmation happens only from the payment webhook or the manual driver
// (panel accept with confirm=true), never from the browser return URL.
// The booking state changes always go through internal/domain/booking:
// missing operations (extend the hold of a held booking, move held ->
// pending_payment with a partial amount) are added there in a new file
// (internal/domain/booking/renewal.go), not reimplemented here.
//
// Expiry: booking.Service.ExpireHolds already expires held bookings at
// hold_expires_at (booking.expired, allocation released). The renewal job
// (JobSync, every minute) marks proposals whose booking left the
// held/pending_payment state: confirmed -> converted, expired/cancelled ->
// expired (or declined when declined). Events: season.renewal_offered
// (first send), season.renewal_accepted, season.renewal_declined,
// season.renewal_expired; conversion is booking.confirmed.
//
// Released spots trigger the waitlist through allocation.released (the
// waitlist sink), nothing renewal-specific is needed.
package renewal
