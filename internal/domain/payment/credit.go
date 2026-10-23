package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// M5B customer credit as a payment method from a verified customer link
// (spec 13.4, ADR 0017) and the "credit back as credit" rule of refunds.

// ErrNoCreditAvailable: the customer has no spendable credit (422
// credit_insufficient through the credit package error).
var ErrNoCreditAvailable = fmt.Errorf("%w: nessun credito disponibile", configerr.ErrValidation)

// creditPayment are the options of a payment of method credit.
type creditPayment struct {
	// Amount to pay (0 = the minimum of available credit, balance due and Max).
	Amount money.Cents
	// Max caps an automatic amount (0 = no cap).
	Max money.Cents
	// AllowHeld accepts a held / pending_payment booking (renewal or
	// waitlist acceptance); the payment may then confirm it through the
	// manual driver like a desk payment (INV-19).
	AllowHeld      bool
	IdempotencyKey string
}

// PayWithCredit records a payment of method credit on a confirmed (or
// checked-in) booking for the holder of its magic link, without a
// principal: amount 0 = the minimum of the available credit and the
// balance due; more than the balance due is refused. One transaction:
// booking lock, customer lock (INV-47), payment through the manual driver
// with its fiscal document, redemption (credit ledger, conditional balance
// update), paid_amount, audit and payment.succeeded. Implements
// credit.Payments.
func (s *Service) PayWithCredit(ctx context.Context, es store.EstablishmentScope, bookingID string, amount money.Cents, idempotencyKey string) (*store.Payment, error) {
	pm, err := s.payCredit(ctx, es, bookingID, creditPayment{Amount: amount, IdempotencyKey: idempotencyKey})
	if err == nil && pm == nil {
		return nil, ErrNoCreditAvailable
	}
	return pm, err
}

// PayHeldWithCredit spends up to max of the customer credit on a booking
// that may still be held (renewal or waitlist acceptance from a verified
// link): nil payment and no error when no credit is available. When the
// paid amount reaches the deposit (or the total) the booking is confirmed
// in the same transaction through the manual driver, as at the desk.
// Implements credit.Payments.
func (s *Service) PayHeldWithCredit(ctx context.Context, es store.EstablishmentScope, bookingID string, maxAmount money.Cents, idempotencyKey string) (*store.Payment, error) {
	return s.payCredit(ctx, es, bookingID, creditPayment{Max: maxAmount, AllowHeld: true, IdempotencyKey: idempotencyKey})
}

func (s *Service) payCredit(ctx context.Context, es store.EstablishmentScope, bookingID string, in creditPayment) (*store.Payment, error) {
	var ve configerr.ValidationError
	if in.Amount < 0 || in.Max < 0 {
		ve.Add("amount", "L'importo non può essere negativo.")
	}
	validateKey(in.IdempotencyKey, &ve)
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	want := func(pm *store.Payment) bool {
		return pm.BookingID != nil && *pm.BookingID == bookingID && pm.Direction == store.PaymentDirectionIn &&
			pm.Method == store.PaymentMethodCredit && (in.Amount == 0 || pm.Amount == in.Amount)
	}
	res, err := s.runIdempotent(ctx, es, in.IdempotencyKey, want, func(ctx context.Context, tx bun.Tx) (*Result, error) {
		b, err := s.d.Bookings.LoadForPayment(ctx, tx, es, bookingID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrBookingNotFound
			}
			return nil, fmt.Errorf("payment: load booking: %w", err)
		}
		if r, err := replay(ctx, tx, es, in.IdempotencyKey, want); err != nil || r != nil {
			return r, err
		}
		switch b.Status {
		case "confirmed", "checked_in":
		case bookingHeld, bookingPendingPayment:
			if !in.AllowHeld {
				return nil, ErrBookingNotPayable
			}
		default:
			return nil, ErrBookingNotPayable
		}
		due := b.TotalAmount - b.PaidAmount
		if due <= 0 {
			if in.AllowHeld {
				return &Result{}, nil
			}
			return nil, configerr.Invalid("amount", "La prenotazione è già saldata.")
		}
		info, err := s.establishment(ctx, tx, es)
		if err != nil {
			return nil, err
		}
		if err := store.LockCustomer(ctx, tx, es, b.CustomerID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrCustomerNotFound
			}
			return nil, fmt.Errorf("payment: %w", err)
		}
		avail, err := s.d.Credits.CreditBalance(ctx, tx, es, b.CustomerID, info.today)
		if err != nil {
			return nil, fmt.Errorf("payment: credit balance: %w", err)
		}
		pay := in.Amount
		if pay == 0 {
			pay = min(avail, due)
			if in.Max > 0 {
				pay = min(pay, in.Max)
			}
		}
		if pay <= 0 {
			return &Result{}, nil
		}
		if pay > due {
			return nil, configerr.Invalid("amount", fmt.Sprintf("L'importo supera il saldo dovuto (%s).", euro(due)))
		}
		pp, err := s.manual()
		if err != nil {
			return nil, err
		}
		intent, err := pp.CreatePayment(ctx, ext.CreatePaymentRequest{
			EstablishmentID: es.ID(), BookingID: b.ID, BookingCode: b.Code, Amount: int64(pay),
			Currency: info.est.Currency, IdempotencyKey: in.IdempotencyKey, PreferredMode: ext.PaymentModeNone,
		})
		if err != nil {
			return nil, fmt.Errorf("payment: manual driver: %w", err)
		}
		now := s.now()
		pm := &store.Payment{
			ID: store.NewID(), BookingID: &b.ID, CustomerID: optional(b.CustomerID), Provider: pp.Code(),
			Method: store.PaymentMethodCredit, Direction: store.PaymentDirectionIn, Amount: pay, Status: store.PaymentSucceeded,
			ProviderRef: optional(intent.ProviderRef), IdempotencyKey: optional(in.IdempotencyKey),
			Description: "Prenotazione " + b.Code, VATBreakdown: SplitBySnapshot(b.CurrentPrice(), pay, info.defaultVAT),
			BusinessDate: info.today, Metadata: map[string]any{"by": "customer_link"}, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.insert(ctx, tx, es, pm); err != nil {
			return nil, err
		}
		// The customer row is already locked; the ledger refuses an
		// overdraft with the conditional balance update.
		if err := s.grantCredit(ctx, tx, nil, es, b.CustomerID, -pay, store.CreditSourceRedemption, nil, pm, ""); err != nil {
			return nil, err
		}
		updated, err := s.d.Bookings.ApplyPayment(ctx, tx, es, b.ID, pay)
		if err != nil {
			return nil, fmt.Errorf("payment: apply to booking: %w", err)
		}
		if err := s.record(ctx, tx, nil, "", es, "payment.recorded", event.PaymentSucceeded, pm, map[string]any{"by": "customer_link"}); err != nil {
			return nil, err
		}
		res := &Result{Payment: *pm, Booking: updated}
		if in.AllowHeld {
			if res.Confirmed, err = s.maybeConfirm(ctx, tx, nil, es, updated, pm); err != nil {
				return nil, err
			}
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	if res.Payment.ID == "" {
		return nil, nil
	}
	return &res.Payment, nil
}

// splitCreditRefund handles the part of a money refund that the booking
// received as customer credit: that part goes back as credit (spec 13.4),
// never as money. A refund of a specific payment of method credit becomes a
// credit refund entirely. Otherwise, when the money refund exceeds what was
// paid with other methods (net of refunds and in-flight provider refunds),
// the excess (up to the credit-paid amount) is recorded first as a credit
// refund and req.Amount is reduced to the money part. It returns the result
// of the credit part and whether nothing is left to refund as money.
func (s *Service) splitCreditRefund(ctx context.Context, tx bun.IDB, p *auth.Principal, ip string, es store.EstablishmentScope, b *store.Booking, info *estInfo, req *RefundRequest) (*Result, bool, error) {
	if req.As != RefundAsRefund {
		return nil, false, nil
	}
	if req.PaymentID != "" {
		orig, err := store.GetPayment(ctx, tx, es, req.PaymentID)
		if err == nil && orig.Method == store.PaymentMethodCredit {
			req.As, req.Method = RefundAsCredit, store.PaymentMethodCredit
			if req.CreditSource == "" {
				req.CreditSource = store.CreditSourceManual
			}
		}
		return nil, false, nil
	}
	creditNet, err := store.BookingCreditNet(ctx, tx, es, b.ID)
	if err != nil {
		return nil, false, fmt.Errorf("payment: %w", err)
	}
	if creditNet <= 0 {
		return nil, false, nil
	}
	totals, err := store.BookingPaymentTotals(ctx, tx, es, b.ID)
	if err != nil {
		return nil, false, fmt.Errorf("payment: %w", err)
	}
	inFlight, err := store.SumOpenRefundIntents(ctx, tx, es, "", b.ID)
	if err != nil {
		return nil, false, fmt.Errorf("payment: %w", err)
	}
	moneyLeft := max(totals.Net()-inFlight-creditNet, 0)
	if req.Amount <= moneyLeft {
		return nil, false, nil
	}
	part := min(req.Amount-moneyLeft, creditNet)
	src := req.CreditSource
	if src == "" {
		src = store.CreditSourceManual
	}
	rest := req.Amount - part
	creditReq := RefundRequest{
		BookingID: b.ID, Amount: part, As: RefundAsCredit, Method: store.PaymentMethodCredit,
		Reason: req.Reason, CreditSource: src, CreditExpiresOn: req.CreditExpiresOn,
	}
	if rest == 0 {
		creditReq.IdempotencyKey = req.IdempotencyKey
	}
	r, err := s.refundInTx(ctx, tx, p, ip, es, b, info, creditReq)
	if err != nil {
		return nil, false, err
	}
	req.Amount = rest
	return r, rest == 0, nil
}
