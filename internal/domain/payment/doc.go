// Package payment records payments and refunds (spec 13): manual payments
// collected at the desk (cash, card, bank transfer, wallet, credit, other),
// manual refunds, counter sales without a booking, and the payments ledger
// that keeps bookings.paid_amount consistent (INV-22, INV-24).
//
// Ownership (M3): the "payments" builder owns this package, the handlers in
// internal/http/admin/payments.go and the repository functions in
// internal/store/payment.go. Model: store.Payment (models_payment.go).
//
// Architecture (ports & adapters, functional core / imperative shell):
//   - vat.go is the pure core: the VAT split frozen on every payment
//     (proportional to the booking price snapshot, largest remainder, then
//     money.VATSplit half-even; INV-16);
//   - service.go is the shell: one unit of work (store.RunInTx) per use
//     case, the same tx passed to every port (BookingLedger, CashDesk,
//     Fiscal, Credits), the audit writer and the outbox;
//   - ports.go declares what the package needs; internal/app wires the
//     adapters (booking.Ledger, cash.Service, fiscal.Service,
//     customer.Service, *app.Registry) and asserts them at compile time.
//
// Drivers (strategy/registry): providers are resolved through Providers
// (*app.Registry). "manual" (builtin.Manual) is always present and records
// desk payments: CreatePayment gives the provider reference, refunds are
// recorded as manual refunds (spec 13.4). Stripe and the online flow arrive
// in M4 and plug into the same registry, ledger and
// BookingLedger.ApplyPayment (webhook -> confirmation stays in the booking
// service, INV-19).
//
// Rules of RecordPayment (one transaction):
//  1. lock the booking (BookingLedger.LoadForPayment) - concurrent payments
//     of a booking are serialized, so the balance check cannot be raced;
//  2. idempotency: an existing payment with the same key is returned as a
//     replay (ErrIdempotencyMismatch when it describes another payment); a
//     concurrent loser hits payments_establishment_idempotency_uidx, rolls
//     back and returns the winner (the offline queue of spec 21.2 never
//     records a payment twice; the HTTP middleware also replays responses);
//  3. amount <= balance due, unless OverpaymentAsCredit: the excess is then
//     refunded at once as customer credit (refund row, method credit), so
//     paid_amount never exceeds the total;
//  4. CashDesk.SessionForPayment: cash requires an open session (409
//     cash_session_closed), other methods attach to the open session of the
//     main register when there is one; method credit never does;
//  5. insert the payment (status succeeded, business_date in the
//     establishment timezone, frozen vat_breakdown), Fiscal.RecordPayment,
//     Credits for method credit, BookingLedger.ApplyPayment, audit
//     ("payment.recorded") and event.PaymentSucceeded (event.PaymentChange);
//  6. manual-driver confirmation: a held / pending_payment booking whose
//     paid amount reaches deposit_due (or the total when no deposit is due)
//     is confirmed through BookingConfirmer in the same tx; the booking
//     service owns the transition.
//
// Refunds (RefundBooking from the API, RecordRefund from the booking
// cancellation in the caller's tx): never above what was paid (INV-24),
// never above the refunded payment when payment_id is given; the VAT split
// follows the refunded payment (or the snapshot); as=credit grants customer
// credit (Credits) with method credit; audit "payment.refunded" and
// event.PaymentRefunded.
//
// Permissions: list -> auth.PermBookingsRead; record payment and counter
// sale -> auth.PermCashOperate; refunds -> auth.PermRefundsIssue (the
// booking service checks it before calling RecordRefund on cancellation).
package payment
