// Package payment records payments and refunds (spec 13): manual payments
// collected at the desk (cash, card, bank transfer, wallet, credit, other),
// manual refunds, counter sales without a booking, and the payments ledger
// that keeps bookings.paid_amount consistent (INV-22, INV-24).
//
// Ownership (M3): the "payments" builder owns this package, the handlers in
// internal/http/admin/payments.go and the repository functions in
// internal/store/payment.go. Model: store.Payment (models_payment.go).
//
// Drivers (strategy/registry): providers are resolved through Providers,
// implemented by the app extension registry (*app.Registry): "manual"
// (builtin.Manual) is always present; Stripe and the online flow arrive in
// M4 and plug into the same registry, the same ledger and
// BookingLedger.ApplyPayment (webhook -> confirmation stays in the booking
// service, INV-19).
//
// Rules:
//   - one transaction per payment: insert the payments row (amount > 0,
//     direction in|refund, status succeeded for manual), apply the signed
//     amount to the booking through BookingLedger, attach the open cash
//     session through CashDesk (required for method cash: 409
//     cash_session_closed), freeze the VAT split (vat_breakdown) from the
//     booking price snapshot proportionally (money.VATSplit, INV-16), call
//     Fiscal.RecordPayment, write the audit log and the outbox event
//     (event.PaymentSucceeded / PaymentRefunded, payload event.PaymentChange);
//   - idempotency: the HTTP middleware replays responses by Idempotency-Key
//     and the service also stores it in payments.idempotency_key (unique per
//     establishment) so that the offline queue (spec 21.2) never records a
//     payment twice;
//   - refunds never exceed what was paid (INV-24, configerr validation);
//     refunds as credit go to the customer ledger through Credits;
//   - method credit consumes the customer credit balance (Credits).
//
// Permissions: list -> auth.PermBookingsRead; record payment and counter
// sale -> auth.PermCashOperate; refunds -> auth.PermRefundsIssue (the
// booking service checks it before calling RecordRefund on cancellation).
package payment
