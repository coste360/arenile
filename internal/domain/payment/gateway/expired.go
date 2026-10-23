package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/store"
)

// HoldExpirer is an OPTIONAL booking-side port: it expires a held or
// pending_payment booking whose hold elapsed, in tx, with audit and outbox
// (the transition the hold sweeper makes). The gateway uses it when a
// provider success arrives after hold_expires_at but before the sweeper
// ran: the payment then follows the late-payment path (re-allocation
// through LateConfirmer or automatic refund) instead of failing on the
// expired hold and being retried by the provider. Implemented by the
// booking service (internal/app wires it).
type HoldExpirer interface {
	ExpireHold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error
}

// SetHoldExpirer installs the hold expiry path after construction (the
// booking service depends on the payment ledger, so it is built later).
func (s *Service) SetHoldExpirer(h HoldExpirer) { s.d.HoldExpirer = h }

// RefundExpiredBooking schedules the refund of every provider payment of a
// booking whose hold expired with money on it (INV-51): a payment below the
// confirmation threshold because the total changed between the start of
// the payment and the webhook, or a partial payment. One refund intent per
// payment for what is still refundable (ledger amount minus the refunds
// done and in flight), reason expired_unpaid, status created: the
// reconciliation sends them to the provider (created intents are retried)
// and the ledger records them when the provider confirms. Runs in the
// caller's transaction with the booking locked. Idempotent per payment.
// Implements booking.ExpiredRefunder.
func (s *Service) RefundExpiredBooking(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error {
	intents, err := store.ListSucceededPaymentIntents(ctx, tx, es, bookingID)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	x := &txn{s: s, tx: tx, es: es, eventID: "expired:" + bookingID, source: "hold_expiry", out: &applied{}}
	for i := range intents {
		pi := &intents[i]
		if pi.PaymentID == nil {
			continue
		}
		pm, err := store.GetPayment(ctx, tx, es, *pi.PaymentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return fmt.Errorf("gateway: %w", err)
		}
		refunded, err := store.RefundedOfPayment(ctx, tx, es, pm.ID)
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		open, err := store.SumOpenRefundIntents(ctx, tx, es, pi.ID, "")
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		left := pm.Amount - refunded - open
		if left <= 0 {
			continue
		}
		if err := x.schedule(ctx, pi, pm.ID, left, "expired_unpaid",
			"Prenotazione scaduta con un pagamento parziale: importo rimborsato"); err != nil {
			return err
		}
	}
	return nil
}
