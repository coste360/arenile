package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

const (
	maxReason         = 500
	maxIdempotencyKey = 255
)

// RefundViaProvider executes a refund with the provider of the payment
// (ADR 0014 "refunds"):
//
//  1. one transaction reserves the amount: the provider payment, then the
//     booking are locked (the order of apply), the cap is checked against
//     the succeeded ledger refunds PLUS the refund intents still waiting
//     for the provider (of the payment and of the booking: INV-24 holds
//     even with refunds in flight, also against the manual refunds and
//     credits of payment.refundInTx, which count the same intents under
//     the same booking lock), and a refund intent is inserted (status
//     created, idempotency key);
//  2. the provider is called outside any transaction with that key;
//  3. the answer is recorded: succeeded -> ledger refund row at once
//     (Stripe confirmed it), pending -> waits for charge.refunded or the
//     reconciliation, refused -> intent failed (amount released).
//
// A provider that cannot be reached leaves the intent created: the
// reconciliation sends it again with the same idempotency key, so the
// money moves exactly once, and the intent is returned as pending. Used by
// the admin refund with via_provider and by customer cancellations.
func (s *Service) RefundViaProvider(ctx context.Context, es store.EstablishmentScope, req RefundRequest) (*store.PaymentIntent, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	var ve configerr.ValidationError
	if req.PaymentID == "" {
		ve.Add("payment_id", "Indica il pagamento online da rimborsare.")
	}
	if req.Amount <= 0 || req.Amount > payment.MaxAmount {
		ve.Add("amount", "L'importo deve essere positivo (massimo 1.000.000,00).")
	}
	if len([]rune(req.Reason)) > maxReason {
		ve.Add("reason", fmt.Sprintf("Massimo %d caratteri.", maxReason))
	}
	if len(req.IdempotencyKey) > maxIdempotencyKey {
		ve.Add("Idempotency-Key", "Chiave di idempotenza troppo lunga.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	var ri *store.PaymentIntent
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		ri, err = s.reserveRefund(ctx, tx, es, req)
		return err
	})
	if err != nil {
		if req.IdempotencyKey != "" && (errors.Is(err, store.ErrConflict) || store.IsUniqueViolation(err)) {
			// Two requests with the same key raced: the winner's intent.
			if prev, rerr := s.replayRefund(ctx, s.d.DB, es, req); rerr != nil || prev != nil {
				return prev, rerr
			}
		}
		return nil, err
	}
	if ri.Status == store.IntentCreated {
		if err := s.executeRefund(ctx, es, ri.ID); err != nil && !errors.Is(err, ErrProviderUnavailable) {
			return nil, err
		}
	}
	out, err := store.GetPaymentIntent(ctx, s.d.DB, es, ri.ID)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	return out, nil
}

// replayRefund returns the refund intent created with req.IdempotencyKey
// when it describes the same refund (nil, nil when the key is unused).
func (s *Service) replayRefund(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req RefundRequest) (*store.PaymentIntent, error) {
	ri, err := store.GetPaymentIntentByIdempotencyKey(ctx, db, es, req.IdempotencyKey)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if ri.Kind != store.IntentKindRefund || ri.Amount != req.Amount || ri.RefundOfIntentID == nil {
		return nil, ErrIdempotencyMismatch
	}
	pay, err := store.GetPaymentIntent(ctx, db, es, *ri.RefundOfIntentID)
	if err != nil || deref(pay.PaymentID) != req.PaymentID {
		return nil, ErrIdempotencyMismatch
	}
	return ri, nil
}

// reserveRefund is step 1 of RefundViaProvider, in tx.
func (s *Service) reserveRefund(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req RefundRequest) (*store.PaymentIntent, error) {
	if req.IdempotencyKey != "" {
		if ri, err := s.replayRefund(ctx, tx, es, req); err != nil || ri != nil {
			return ri, err
		}
	}
	pay, err := store.GetPaymentIntentByPaymentID(ctx, tx, es, req.PaymentID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotOnlinePayment
	}
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if pay, err = store.LockPaymentIntent(ctx, tx, es, pay.ID); err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if pay.Status != store.IntentSucceeded || pay.BookingID == nil || (req.BookingID != "" && *pay.BookingID != req.BookingID) {
		return nil, ErrNotOnlinePayment
	}
	// Lock order of the gateway (intent, then booking): the booking lock
	// serialises the cap with manual refunds and credits, which run under
	// the booking lock (payment.refundInTx) and count open intents.
	if _, err := store.LockBooking(ctx, tx, es, *pay.BookingID); err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if req.RefuseOpenRequests {
		// ADR 0019: no manual provider refund while a guaranteed refund of
		// the booking is open (the job owns it; 409 refund_in_progress).
		// Under the booking lock, so a cancellation committing meanwhile
		// cannot slip through (INV-75).
		if err := payment.CheckNoOpenRefundRequest(ctx, tx, es, *pay.BookingID); err != nil {
			return nil, err
		}
	}
	pp, ok := s.d.Providers.PaymentProvider(pay.Provider)
	if !ok {
		return nil, ErrProviderNotConfigured
	}
	if caps := pp.Capabilities(); !caps.Refunds {
		return nil, ErrRefundNotSupported
	}
	ledgerRow, err := store.GetPayment(ctx, tx, es, req.PaymentID)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if !pp.Capabilities().PartialRefunds && req.Amount != ledgerRow.Amount {
		return nil, configerr.Invalid("amount", "Il provider consente solo il rimborso dell'intero pagamento.")
	}
	refunded, err := store.RefundedOfPayment(ctx, tx, es, ledgerRow.ID)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	inFlight, err := store.SumOpenRefundIntents(ctx, tx, es, pay.ID, "")
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if left := ledgerRow.Amount - refunded - inFlight; req.Amount > left {
		return nil, configerr.Invalid("amount", fmt.Sprintf("Il rimborso supera il residuo rimborsabile del pagamento (%s).", euro(max(left, 0))))
	}
	totals, err := store.BookingPaymentTotals(ctx, tx, es, *pay.BookingID)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	bookingInFlight, err := store.SumOpenRefundIntents(ctx, tx, es, "", *pay.BookingID)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if left := totals.Net() - bookingInFlight; req.Amount > left {
		return nil, configerr.Invalid("amount", fmt.Sprintf("Il rimborso supera quanto pagato (massimo %s).", euro(max(left, 0))))
	}
	now := s.now()
	id := store.NewID()
	key := req.IdempotencyKey
	if key == "" {
		key = "refund:" + id
	}
	ri := &store.PaymentIntent{
		ID: id, BookingID: pay.BookingID, RefundOfIntentID: &pay.ID, Provider: pay.Provider, Mode: pay.Mode,
		Kind: store.IntentKindRefund, ExternalID: placeholderPrefix + id, Status: store.IntentCreated,
		Amount: req.Amount, Currency: pay.Currency, Attempt: 1, IdempotencyKey: &key, RawEventIDs: []string{},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertPaymentIntent(ctx, tx, es, ri); err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if err := s.record(ctx, tx, es, req.Actor, "payment.refund_requested", ri,
		map[string]any{"payment_id": ledgerRow.ID, "reason": req.Reason}, "", event.PaymentChange{}); err != nil {
		return nil, err
	}
	return ri, nil
}

// RefundBooking is RefundViaProvider with the principal check
// (auth.PermRefundsIssue), for internal/http/admin/payments.go.
func (s *Service) RefundBooking(ctx context.Context, p *auth.Principal, establishmentID string, req RefundRequest) (*store.PaymentIntent, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermRefundsIssue)
	if err != nil {
		return nil, err
	}
	req.Actor = p.Actor(reqctx.From(ctx).ClientIP)
	req.RefuseOpenRequests = true // checked under the booking lock in reserveRefund
	return s.RefundViaProvider(ctx, es, req)
}

// executeRefunds sends refund intents to the provider after a commit; a
// failure is logged only (the reconciliation retries created intents).
func (s *Service) executeRefunds(ctx context.Context, es store.EstablishmentScope, ids []string) {
	for _, id := range ids {
		if err := s.executeRefund(ctx, es, id); err != nil {
			s.d.Log.WarnContext(ctx, "gateway: refund not sent, reconciliation will retry", "establishment_id", es.ID(), "intent_id", id, "err", err)
		}
	}
}

// executeRefund sends one created refund intent to its provider (steps 2
// and 3 of RefundViaProvider). Safe to run concurrently and repeatedly:
// the provider deduplicates by idempotency key and the result is recorded
// only while the intent is still created.
func (s *Service) executeRefund(ctx context.Context, es store.EstablishmentScope, id string) error {
	ri, err := store.GetPaymentIntent(ctx, s.d.DB, es, id)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	if ri.Status != store.IntentCreated || ri.Kind != store.IntentKindRefund || ri.RefundOfIntentID == nil {
		return nil
	}
	pay, err := store.GetPaymentIntent(ctx, s.d.DB, es, *ri.RefundOfIntentID)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	cfg, pp, err := s.provider(ctx, s.d.DB, es, ri.Provider, ri.Mode)
	if err != nil {
		return err
	}
	ref := deref(pay.ExternalRef)
	if ref == "" {
		ref = pay.ExternalID
	}
	res, perr := pp.Refund(ctx, ext.RefundRequest{
		EstablishmentID: es.ID(), PaymentID: deref(pay.PaymentID), ProviderRef: ref, Amount: int64(ri.Amount),
		Currency: ri.Currency, IdempotencyKey: deref(ri.IdempotencyKey), Credentials: cfg.Credentials,
	})
	if perr != nil && isTemporary(perr) {
		return providerError("refund", perr)
	}
	var refused error
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if _, err := store.LockPaymentIntent(ctx, tx, es, pay.ID); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		cur, err := store.LockPaymentIntent(ctx, tx, es, ri.ID)
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		cur.UpdatedAt = s.now()
		if perr != nil {
			refused = providerError("refund", perr)
			if cur.Status != store.IntentCreated {
				return nil
			}
			cur.Status = store.IntentFailed
			cur.FailureReason = strPtr(providerCode(perr))
			if err := store.UpdatePaymentIntent(ctx, tx, es, cur, "status", "failure_reason"); err != nil {
				return fmt.Errorf("gateway: %w", err)
			}
			return s.record(ctx, tx, es, audit.SystemActor, "payment.refund_failed", cur,
				map[string]any{"failure_reason": providerCode(perr)}, "", event.PaymentChange{})
		}
		if res.ProviderRef != "" && isPlaceholder(cur.ExternalID) {
			cur.ExternalID = res.ProviderRef
		}
		if cur.Status == store.IntentCreated {
			switch res.Status {
			case ext.RefundSucceeded:
				x := &txn{s: s, tx: tx, es: es, cfg: cfg, ev: &ext.PaymentEvent{}, eventID: "refund:" + res.ProviderRef, source: "refund", out: &applied{}}
				if err := x.completeRefund(ctx, pay, cur); err != nil {
					return err
				}
			case ext.RefundFailed:
				cur.Status, cur.FailureReason = store.IntentFailed, strPtr("refund_failed")
			default:
				cur.Status = store.IntentPending
			}
		}
		return store.UpdatePaymentIntent(ctx, tx, es, cur, "external_id", "status", "payment_id", "failure_reason", "raw_event_ids")
	})
	if err != nil {
		return err
	}
	return refused
}

// euro formats cents for validation messages ("12,50 €").
func euro(c money.Cents) string {
	return fmt.Sprintf("%d,%02d €", int64(c)/100, int64(c)%100)
}
