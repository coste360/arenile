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

// M5B (spec 13.4, INV-71): what a booking received through the payment
// method credit goes back as credit, never as money.
//
//   - A money refund of a cancellation is split with the single rule of
//     payment.CreditRefundSplit (money first): the other methods cover the
//     refund as long as they can, only the excess, up to the credit-paid
//     amount, is recorded as a credit (source cancellation / weather, expiry
//     from the policy snapshot) in the same transaction; the rest follows
//     the usual path (recorded refund or refund_due).
//   - A booking that ends before its confirmation (expired hold, declined
//     renewal) gives back the whole credit-paid amount as credit in the same
//     transaction; the money part follows the provider refund of the
//     gateway (INV-51).

// creditRefundPart is the part of the money refund rf that goes back as
// credit (payment.CreditRefundSplit).
func creditRefundPart(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, rf RefundPreview) (money.Cents, error) {
	if rf.As != SettleRefund || rf.Amount <= 0 {
		return 0, nil
	}
	part, err := payment.CreditRefundSplit(ctx, tx, es, bookingID, rf.Amount)
	if err != nil {
		return 0, fmt.Errorf("booking: credit refund: %w", err)
	}
	return part, nil
}

// recordCreditRefund gives amount back as customer credit (booking.Payments
// in tx; p nil for the customer's own cancellation and for the system).
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

// returnCreditOnEnd adds to o the return, as credit, of everything the
// booking b paid with credit (a held or pending_payment booking that
// expires or is declined: INV-71). It runs in o.after before any refund the
// outcome already schedules, so the gateway sees only the money part left.
// Without a payment service (unit fixtures) nothing is added.
func (s *Service) returnCreditOnEnd(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, o *outcome) error {
	if b.PaidAmount <= 0 || s.d.Payments == nil {
		return nil
	}
	back, err := payment.CreditToReturn(ctx, tx, es, b.ID)
	if err != nil {
		return fmt.Errorf("booking: credit return: %w", err)
	}
	if back <= 0 {
		return nil
	}
	if o.diff == nil {
		o.diff = map[string]any{}
	}
	o.diff["refund_credit_part"] = int64(back)
	next := o.after
	o.after = func(ctx context.Context, tx bun.IDB, b *store.Booking) error {
		if _, err := s.recordCreditRefund(ctx, tx, nil, es, b.ID, back, false); err != nil {
			return err
		}
		if next != nil {
			return next(ctx, tx, b)
		}
		return nil
	}
	return nil
}
