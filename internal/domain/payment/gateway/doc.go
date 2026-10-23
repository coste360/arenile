// Package gateway runs online payments through the registered payment
// providers (spec 13): it starts provider payments for the public flow and
// the manage area, receives and deduplicates signed webhooks, reconciles
// pending payments with the provider, and executes refunds through the
// provider.
//
// Ownership (M4): the "payment gateway" builder owns this package, the
// Stripe driver (internal/domain/payment/stripe), payment.Service
// RecordProviderPayment / RecordProviderRefund
// (internal/domain/payment/provider.go), the handlers in
// internal/http/webhooks/ and internal/http/admin/reconciliation.go, the
// via_provider branch of recordBookingRefund in
// internal/http/admin/payments.go, and the repository functions in
// internal/store/payment_intent.go (models payment_intents, webhook_events).
//
// # Flows
//
// Start (called by checkout inside its transaction):
//
//	ProviderSettings.ActiveProvider(es)              // decrypted credentials
//	insert payment_intents (status created, attempt n,
//	        idempotency key "<booking_id>:<attempt>", spec 13.2)
//	ext.PaymentProvider.CreatePayment(...)          // outside any DB lock is
//	                                                // preferable: see note
//	update intent (external_id, status pending, expires_at)
//	outbox payment.pending
//
// The provider call is network I/O: Start must not keep a booking row lock
// while waiting. The intended shape is two short transactions (reserve the
// attempt, call the provider, record the reference) with the idempotency
// key making a retry after a crash return the same provider session.
//
// HandleWebhook (POST /webhooks/payments/{provider}/{establishment_id}):
//
//	read raw body (max 256 KB) -> ProviderSettings.ProviderConfig(es, provider)
//	ext.PaymentProvider.ParseWebhook(r, creds)      // ErrInvalidSignature -> 400
//	ONE transaction:
//	  insert webhook_events (provider, event_id)    // unique: duplicate -> 200, no effect
//	  lock + update payment_intents
//	  succeeded -> Ledger.RecordProviderPayment     // ledger, paid_amount and
//	                                                // confirmation via the booking
//	                                                // service (INV-19)
//	  failed    -> intent failed, outbox payment.failed (booking stays
//	               pending_payment until its hold expires; the page may retry)
//	  refunded  -> Ledger.RecordProviderRefund
//
// Reconcile (job every 15 minutes, and POST payments/reconciliation): for
// intents pending for more than 10 minutes, ask the provider
// (PaymentStatusChecker, optional) and apply the outcome through the same
// code path as the webhook, with a synthetic event id
// "reconcile:<external_id>:<status>" so a late webhook is still deduplicated.
//
// # Optional driver capability
//
// ext.PaymentProvider has no way to query a payment by reference, which
// reconciliation needs. Instead of changing pkg/ext (a breaking change for
// plugins), drivers MAY implement PaymentStatusChecker (declared here, made
// only of pkg/ext types, so third-party drivers implement it structurally
// without importing internal packages). Drivers without it are reported as
// provider_unreachable in the reconciliation report.
//
// # Never
//
//   - confirm from the browser return URL (only webhook or reconciliation);
//   - log credentials, signatures, raw bodies, emails or tokens;
//   - process the same provider event twice (webhook_events unique index).
package gateway
