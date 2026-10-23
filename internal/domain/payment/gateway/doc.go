// Package gateway runs online payments through the registered payment
// providers (spec 13): it starts provider payments for the public flow and
// the manage area, receives and deduplicates signed webhooks, reconciles
// open payments and refunds with the provider, and executes refunds
// through the provider. Decisions are recorded in ADR 0014.
//
// Ownership (M4): the "payment gateway" builder owns this package, the
// Stripe driver (internal/domain/payment/stripe and its fake API
// stripetest), payment.Service RecordProviderPayment / RecordProviderRefund
// (internal/domain/payment/provider.go), the handlers in
// internal/http/webhooks/, internal/http/admin/reconciliation.go and
// paymentsettings.go, the via_provider branch of RecordBookingRefund in
// internal/http/admin/payments.go, and internal/store/payment_intent.go
// (payment_intents, webhook_events).
//
// # Architecture
//
// Ports and adapters: the gateway declares what it needs (ProviderSettings
// = online.Service, Ledger = payment.Service, Providers = app.Registry,
// optional LateConfirmer = booking service) and drivers implement
// ext.PaymentProvider plus optional capabilities made only of pkg/ext types
// (PaymentStatusChecker, CredentialsVerifier, PaymentCanceller), so a
// third-party driver gets reconciliation without importing internal
// packages and pkg/ext stays unchanged. Provider calls (network) never run
// while the gateway holds its own locks: refunds reserve in one short
// transaction, call the provider, and record the answer in another.
//
// # Flows
//
// Start (called by checkout inside its transaction):
//
//	ProviderSettings.ActiveProvider(es)          // decrypted credentials, must be online
//	previous open attempts -> PaymentCanceller   // no double payment (best effort)
//	ext.PaymentProvider.CreatePayment            // idempotency key "<booking_id>:<attempt>",
//	                                             // expiry hint = booking hold
//	insert payment_intents (pending) + audit + outbox payment.pending
//
// HandleWebhook (POST /webhooks/payments/{provider}/{establishment_id}):
//
//	establishment + provider configured, else 404 (ErrWebhookTarget)
//	ext.PaymentProvider.ParseWebhook(r, creds)   // ErrInvalidSignature -> 400
//	apply, ONE transaction:
//	  insert webhook_events (provider, event_id) // unique: duplicate -> 200, no effect (INV-23)
//	  resolve + lock the intent (scoped to es)   // unknown but expected -> 503, provider retries
//	  succeeded -> Ledger.RecordProviderPayment  // ledger, paid_amount, confirmation through
//	                                             // the booking service, audit, outbox (INV-18/19)
//	  late payment / overpayment -> refund intent + payment.refund_scheduled
//	  failed / expired -> intent closed + payment.failed (booking waits for its hold)
//	  refunded (cumulative) -> open refund intents completed, remainder recorded
//	after commit: scheduled refunds sent to the provider
//
// RefundViaProvider: reserve (lock payment intent, cap = captured - ledger
// refunds - refunds in flight, also per booking: INV-24) -> provider Refund
// with the intent's idempotency key -> succeeded: ledger row now; pending:
// ledger row on charge.refunded or reconciliation; refused: intent failed;
// unreachable: intent stays created and the reconciliation resends it.
//
// Reconcile (job payments.reconcile every 15 minutes, and POST
// payments/reconciliation): open payment intents older than PendingAfter
// (10 minutes) are looked up with PaymentStatusChecker and their outcome
// applied through apply with a synthetic event id
// "reconcile:<external_id>:<type>[:<outcome>]", so the fix happens once and
// a late webhook is a no-op on the state; created refund intents are resent;
// pending refunds are looked up. Report lists intents with their issue
// (missing_ledger_row, amount_mismatch, pending_too_long, status_mismatch,
// provider_unreachable).
//
// # Never
//
//   - confirm from the browser return URL (only webhook or reconciliation);
//   - log credentials, signatures, raw bodies, emails or tokens;
//   - process the same provider event twice (webhook_events unique index);
//   - refund more than was captured (reservation includes refunds in flight).
package gateway
