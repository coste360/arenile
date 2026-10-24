// Package refund guarantees the provider refund of a cancellation (ADR
// 0019, feature B): the refund is reserved as a refund_requests row in the
// same transaction as the cancellation and a job sends it to the provider
// with retries and backoff until the provider confirms.
//
// Ownership (deferred features, builder B): this package, the handlers in
// internal/http/admin/refunds.go, the repository functions on
// store.RefundRequest (new file internal/store/refund.go), the job
// registration in internal/app and the hook in
// internal/domain/booking/online.go (customer self-cancellation, today it
// only records "refund_due") and service.go (panel cancellation with
// settlement refund to an online payment).
//
// # Rules
//
//   - Reserve: booking.CancelByCustomer computes the refundable part paid
//     through the provider (refundablePaid) and inserts one RefundRequest
//     per source payment intent (status pending, next_attempt_at = now,
//     IdempotencyKey = "refund:<booking id>:<intent id>") inside the
//     cancellation transaction, together with the audit and the outbox
//     event. The credit-paid part keeps going back as credit (M5B).
//   - Send: the job claims due rows (pending/processing with
//     next_attempt_at <= now) with a lease, calls the provider Refund with
//     the stored idempotency key (the provider deduplicates), records the
//     refund intent (payment_intents kind refund) and moves to processing.
//     The provider webhook (or reconciliation) confirms: ledger row
//     (payments direction refund), status succeeded, completed_at,
//     booking paid_amount updated, event payment.refunded, email "refund".
//   - Retry: exponential backoff (1m, 5m, 30m, 2h, 6h, then every 12h) up
//     to ARENILE_REFUND_MAX_ATTEMPTS (default 10), then failed + panel
//     notification refund_failed (severity error); retryRefundRequest
//     resets a failed row to pending with the same idempotency key.
//   - Never lost, never doubled: the row is in the cancellation
//     transaction; the unique index (booking_id, payment_intent_id) over
//     non-cancelled rows forbids a second request; a manual refund on the
//     same booking while a request is open answers 409 refund_in_progress.
//   - Visible: BookingDetail.refunds (panel) and ManageBooking.refund
//     (customer: status, amount, dates only).
package refund
