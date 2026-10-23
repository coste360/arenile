// Package credit owns the customer credit ledger (spec 9, 13.4; M5B):
// credits issued (cancellation, season pass release, weather, goodwill,
// manual), consumed as the payment method "credit" (source redemption) and
// expired (source expiry), the balance per customer and the expiry policy.
//
// It extends the M3 ledger (table customer_credits, 00003) instead of
// duplicating it; migration 00008 adds the sources goodwill / expiry /
// adjustment, sign checks per source, reference_credit_id and the table
// customer_credit_balances.
//
// Rules (invariants to add to docs/invariants.md with their tests):
//   - Every movement goes through Service.GrantCredit inside the caller's
//     transaction: it inserts the customer_credits row AND updates
//     customer_credit_balances with a conditional UPDATE
//     (balance = balance + :amount WHERE balance + :amount >= 0; insert the
//     row on first grant). Zero rows on a debit = ErrInsufficient. So
//     balance = SUM(amount) per customer and is never negative, enforced by
//     the database (CHECK balance >= 0) even under concurrency.
//   - Redemptions are written only by the payment service (method credit:
//     payment.Service.redeemCredit, PayWithCredit, PayHeldWithCredit), which
//     locks the customer row first (store.LockCustomer, INV-47). GrantCredit
//     locks the balance row and materialises the expired credit (ExpireDue)
//     before every debit in the same transaction, so expired credit is
//     never spent.
//   - Reversals (negative cancellation / season_release / weather
//     movements, e.g. pass.UnmarkResold) are capped at what is left of the
//     balance: spent credit cannot be taken back and the reversal never
//     blocks the cancellation that causes it.
//   - Expiry is materialised: a grant with expires_on < today and a
//     remaining amount gets one expiry row (amount = -remaining,
//     reference_credit_id = grant; unique per grant, so the job is
//     idempotent). Remaining amounts are computed FIFO by expiry date
//     (earliest expiring credit is consumed first; grants without expiry
//     last). Available() = balance minus what ExpireDue would write today.
//   - Expiry of new grants comes from the policy of the booking that caused
//     them: policy.Rules.CreditValidityDays (0/absent = never expires);
//     goodwill/manual credits take an explicit expires_on.
//   - Every issued/consumed/expired movement writes the audit log and an
//     outbox event (credit.issued, credit.consumed, credit.expired) in the
//     same transaction; payloads carry ids and cents only, never PII.
//
// Service implements payment.Credits and pass.Credits (GrantCredit,
// CreditBalance). internal/app makes customer.Service (the Credits port of
// the payment and pass services) delegate to it with
// customer.UseCreditLedger, so every movement maintains the balance table.
// It also serves the panel ledger (Ledger, Grant), the magic-link balance
// and payment (Summary, ApplyFromLink) and renewal.CreditApplier
// (ApplyCreditToBooking).
package credit
