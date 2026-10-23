package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Ledger entries of online providers (M4). Ownership: the "payment gateway"
// builder (internal/domain/payment/gateway) calls these two methods; they
// implement gateway.Ledger and are asserted in internal/app/m4.go.
//
// Both run in the caller's transaction (the webhook or reconciliation unit
// of work that also records webhook_events and updates payment_intents),
// so the ledger row, bookings.paid_amount, the confirmation of the booking,
// audit and outbox commit together (spec 13.3, INV-18, INV-19). Both are
// idempotent on (provider, provider_ref): a replayed confirmation finds the
// existing row and changes nothing.

// ErrRefundExceedsPaid: a provider refund would take the refunds of the
// booking (or of the refunded payment) above what was paid (INV-24).
var ErrRefundExceedsPaid = fmt.Errorf("%w: il rimborso supera quanto pagato", configerr.ErrConflict)

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
	// NoConfirm records the money without confirming the booking (late
	// payment whose resources are gone: the gateway schedules a refund).
	NoConfirm bool
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
//
// The money arrived at the provider, so the row is written whatever the
// booking status (a late payment on an expired booking included): the
// ledger mirrors reality and the gateway refunds what is not due.
func (s *Service) RecordProviderPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in ProviderPayment) (*Result, error) {
	if in.Provider == "" || in.ProviderRef == "" || in.Amount <= 0 {
		return nil, fmt.Errorf("payment: provider payment: provider, reference and a positive amount are required")
	}
	if in.Method == "" {
		in.Method = store.PaymentMethodCard
	}
	b, err := s.d.Bookings.LoadForPayment(ctx, tx, es, in.BookingID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("payment: load booking: %w", err)
	}
	if pm, err := store.GetPaymentByProviderRef(ctx, tx, es, in.Provider, in.ProviderRef); err == nil {
		return &Result{Payment: *pm, Booking: b, Replayed: true}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("payment: %w", err)
	}
	info, err := s.establishment(ctx, tx, es)
	if err != nil {
		return nil, err
	}
	now := s.now()
	ref := in.ProviderRef
	meta := map[string]any{"online": true}
	if in.IntentID != "" {
		meta["payment_intent_id"] = in.IntentID
	}
	if in.EventID != "" {
		meta["provider_event_id"] = in.EventID
	}
	pm := &store.Payment{
		ID: store.NewID(), BookingID: &b.ID, CustomerID: optional(b.CustomerID), Provider: in.Provider,
		Method: in.Method, Direction: store.PaymentDirectionIn, Amount: in.Amount, Status: store.PaymentSucceeded,
		ProviderRef: &ref, Description: "Prenotazione online " + b.Code,
		VATBreakdown: SplitBySnapshot(b.CurrentPrice(), in.Amount, info.defaultVAT), BusinessDate: info.today,
		Metadata: meta, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.insert(ctx, tx, es, pm); err != nil {
		return nil, err
	}
	updated, err := s.d.Bookings.ApplyPayment(ctx, tx, es, b.ID, in.Amount)
	if err != nil {
		return nil, fmt.Errorf("payment: apply to booking: %w", err)
	}
	extra := map[string]any{"provider_ref": ref}
	if in.EventID != "" {
		extra["provider_event_id"] = in.EventID
	}
	if err := s.record(ctx, tx, nil, "", es, "payment.recorded", event.PaymentSucceeded, pm, extra); err != nil {
		return nil, err
	}
	res := &Result{Payment: *pm, Booking: updated}
	if in.NoConfirm {
		return res, nil
	}
	confirmed, err := s.maybeConfirm(ctx, tx, nil, es, updated, pm)
	if err != nil {
		return nil, err
	}
	res.Confirmed = confirmed
	if confirmed {
		if res.Booking, err = s.d.Bookings.LoadForPayment(ctx, tx, es, b.ID); err != nil {
			return nil, fmt.Errorf("payment: reload booking: %w", err)
		}
	}
	return res, nil
}

// RecordProviderRefund writes the refund ledger row confirmed by the
// provider in tx (idempotent on provider + provider_ref, never above what
// was paid: INV-24) and applies it to the booking. Audit
// "payment.refunded" and event payment.refunded.
func (s *Service) RecordProviderRefund(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in ProviderRefund) (*store.Payment, error) {
	if in.Provider == "" || in.ProviderRef == "" || in.Amount <= 0 || in.PaymentID == "" {
		return nil, fmt.Errorf("payment: provider refund: provider, reference, payment and a positive amount are required")
	}
	b, err := s.d.Bookings.LoadForPayment(ctx, tx, es, in.BookingID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("payment: load booking: %w", err)
	}
	if pm, err := store.GetPaymentByProviderRef(ctx, tx, es, in.Provider, in.ProviderRef); err == nil {
		return pm, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("payment: %w", err)
	}
	orig, err := store.GetPayment(ctx, tx, es, in.PaymentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("payment: %w", err)
	}
	if orig.BookingID == nil || *orig.BookingID != b.ID || orig.Direction != store.PaymentDirectionIn ||
		orig.Status != store.PaymentSucceeded || orig.Provider != in.Provider {
		return nil, fmt.Errorf("%w: il pagamento rimborsato non è un incasso online di questa prenotazione", configerr.ErrConflict)
	}
	totals, err := store.BookingPaymentTotals(ctx, tx, es, b.ID)
	if err != nil {
		return nil, fmt.Errorf("payment: %w", err)
	}
	done, err := store.RefundedOfPayment(ctx, tx, es, orig.ID)
	if err != nil {
		return nil, fmt.Errorf("payment: %w", err)
	}
	if in.Amount > totals.Net() || done+in.Amount > orig.Amount {
		return nil, ErrRefundExceedsPaid
	}
	info, err := s.establishment(ctx, tx, es)
	if err != nil {
		return nil, err
	}
	weights := rateWeights(b.CurrentPrice())
	if len(orig.VATBreakdown) > 0 {
		weights = orig.VATBreakdown
	}
	now := s.now()
	ref := in.ProviderRef
	meta := map[string]any{"online": true, "settlement": string(RefundAsRefund)}
	if in.IntentID != "" {
		meta["payment_intent_id"] = in.IntentID
	}
	if in.EventID != "" {
		meta["provider_event_id"] = in.EventID
	}
	pm := &store.Payment{
		ID: store.NewID(), BookingID: &b.ID, CustomerID: optional(b.CustomerID), Provider: in.Provider,
		Method: orig.Method, Direction: store.PaymentDirectionRefund, Amount: in.Amount, Status: store.PaymentSucceeded,
		ProviderRef: &ref, RefundOfPaymentID: &orig.ID, Description: "Rimborso online prenotazione " + b.Code,
		Note: truncate(strings.TrimSpace(in.Reason), maxNote), VATBreakdown: SplitByWeights(weights, in.Amount, info.defaultVAT),
		BusinessDate: info.today, Metadata: meta, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.insert(ctx, tx, es, pm); err != nil {
		return nil, err
	}
	if _, err := s.d.Bookings.ApplyPayment(ctx, tx, es, b.ID, -in.Amount); err != nil {
		return nil, fmt.Errorf("payment: apply to booking: %w", err)
	}
	if err := s.record(ctx, tx, nil, "", es, "payment.refunded", event.PaymentRefunded, pm, map[string]any{"provider_ref": ref, "settlement": "provider"}); err != nil {
		return nil, err
	}
	return pm, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
