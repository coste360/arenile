package booking

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// M5B (spec 13.4): what a booking received through the payment method
// credit goes back as credit on cancellation, never as money. A money
// refund of the cancellation is split: the credit-paid part is recorded as
// a credit (source cancellation, expiry from the policy snapshot) in the
// same transaction, the rest follows the usual path (recorded refund or
// refund_due).

// creditRefundPart is the part of the money refund rf that was paid with
// customer credit and has not been given back yet.
func creditRefundPart(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, rf RefundPreview) (money.Cents, error) {
	if rf.As != SettleRefund || rf.Amount <= 0 {
		return 0, nil
	}
	net, err := store.BookingCreditNet(ctx, tx, es, bookingID)
	if err != nil {
		return 0, fmt.Errorf("booking: credit refund: %w", err)
	}
	return min(rf.Amount, max(net, 0)), nil
}

// recordCreditRefund gives amount back as customer credit (booking.Payments
// in tx; p nil for the customer's own cancellation).
func (s *Service) recordCreditRefund(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, bookingID string, amount money.Cents, weather bool) (*store.Payment, error) {
	src := store.CreditSourceCancellation
	if weather {
		src = store.CreditSourceWeather
	}
	return s.d.Payments.RecordRefund(ctx, tx, p, es, payment.RefundRequest{
		BookingID: bookingID, Amount: amount, As: payment.RefundAsCredit, Method: store.PaymentMethodCredit,
		Reason: "cancellation: credit-paid part returned as credit", CreditSource: src,
	})
}
