package payment

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Ledger entries of online providers (M4). Ownership: the "payment gateway"
// builder (internal/domain/payment/gateway) implements these two methods;
// they implement gateway.Ledger and are asserted in internal/app/m4.go.
//
// Both run in the caller's transaction (the webhook or reconciliation unit
// of work that also records webhook_events and updates payment_intents),
// so the ledger row, bookings.paid_amount, the confirmation of the booking,
// audit and outbox commit together (spec 13.3, INV-18, INV-19).

// ProviderPayment is a payment confirmed by an online provider.
type ProviderPayment struct {
	BookingID string
	// Provider is the driver code ("stripe"); ProviderRef the provider
	// reference stored in payments.provider_ref (unique per provider: a
	// replayed confirmation finds the existing row and changes nothing).
	Provider    string
	ProviderRef string
	Amount      money.Cents
	// Method is store.PaymentMethodCard or store.PaymentMethodWallet.
	Method string
	// IntentID is the payment_intents row (saved in payments.metadata).
	IntentID string
	// EventID is the provider event that confirmed the payment (audit only).
	EventID string
}

// ProviderRefund is a refund executed (or confirmed) by an online provider.
type ProviderRefund struct {
	BookingID string
	// PaymentID is the refunded ledger row (direction in, same provider).
	PaymentID   string
	Provider    string
	ProviderRef string
	Amount      money.Cents
	Reason      string
	IntentID    string
	EventID     string
}

// RecordProviderPayment writes the succeeded ledger row of a provider
// payment in tx (idempotent on provider + provider_ref), applies it to the
// booking (BookingLedger.ApplyPayment) and, when paid reaches the policy
// threshold, confirms the booking through BookingConfirmer (INV-19). Audit
// "payment.recorded" with actor system and event payment.succeeded.
func (s *Service) RecordProviderPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in ProviderPayment) (*Result, error) {
	return nil, configerr.ErrNotImplemented
}

// RecordProviderRefund writes the refund ledger row confirmed by the
// provider in tx (idempotent on provider + provider_ref, never above what
// was paid: INV-24) and applies it to the booking. Audit
// "payment.refunded" and event payment.refunded.
func (s *Service) RecordProviderRefund(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in ProviderRefund) (*store.Payment, error) {
	return nil, configerr.ErrNotImplemented
}
