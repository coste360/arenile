package gateway

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Booking statuses read by the gateway (mirrors of booking.Status: the
// booking package depends on payment, so the gateway cannot import it).
const (
	bookingHeld           = "held"
	bookingPendingPayment = "pending_payment"
	bookingExpired        = "expired"
)

// source of an applied event (audit only).
const (
	sourceWebhook   = "webhook"
	sourceReconcile = "reconciliation"
)

// HandleWebhook verifies and applies one provider notification for the
// establishment of the URL. r.Body is read raw (MaxWebhookBody, bounded by
// the handler). The establishment and the provider must exist and be
// configured (ErrWebhookTarget otherwise); the driver verifies the
// signature with the establishment's credentials (ErrSignatureInvalid).
// Live and test configurations of the provider are both tried (spec 15.4:
// the test-mode switch never decides which secret verifies a delivery),
// and the one that verifies also fixes the mode of the intents the event
// may touch. After the commit, refunds scheduled by the event are sent to
// the provider (best effort: reconciliation retries them).
func (s *Service) HandleWebhook(ctx context.Context, provider, establishmentID string, r *http.Request) (*WebhookOutcome, error) {
	if _, err := uuid.Parse(establishmentID); err != nil {
		return nil, ErrWebhookTarget
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, establishmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrWebhookTarget
		}
		return nil, fmt.Errorf("gateway: %w", err)
	}
	candidates, pp, err := s.webhookCandidates(ctx, es, provider)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxWebhookBody+1))
	if err != nil || len(body) > MaxWebhookBody {
		// Unreadable or oversized: the same answer the drivers give (an
		// unverifiable delivery), never a server error.
		s.d.Log.DebugContext(ctx, "gateway: webhook body rejected", "provider", provider, "establishment_id", es.ID())
		return nil, ErrSignatureInvalid
	}
	var (
		cfg *online.ProviderConfig
		ev  *ext.PaymentEvent
	)
	for _, c := range candidates {
		r.Body = io.NopCloser(bytes.NewReader(body))
		ev, err = pp.ParseWebhook(ctx, r, c.Credentials)
		if errors.Is(err, ext.ErrInvalidSignature) {
			continue
		}
		cfg = c
		break
	}
	switch {
	case errors.Is(err, ext.ErrIgnoredEvent):
		return &WebhookOutcome{Ignored: true}, nil
	case errors.Is(err, ext.ErrInvalidSignature):
		// Debug level: anyone can post to the URL, and the HTTP layer
		// throttles repeated rejections per IP.
		s.d.Log.DebugContext(ctx, "gateway: webhook signature rejected", "provider", provider, "establishment_id", es.ID())
		return nil, ErrSignatureInvalid
	case err != nil:
		return nil, fmt.Errorf("gateway: parse webhook: %w", err)
	}
	if ev.ProviderEventID == "" {
		// Without an id the event cannot be deduplicated (INV-23).
		return &WebhookOutcome{Ignored: true}, nil
	}
	out, err := s.apply(ctx, es, pp.Code(), cfg, ev, sourceWebhook)
	if err != nil {
		if errors.Is(err, errRetryLater) {
			s.d.Log.InfoContext(ctx, "gateway: webhook for a reference not known yet, provider will retry", "provider", provider, "establishment_id", es.ID(), "event_id", ev.ProviderEventID)
		}
		return nil, err
	}
	s.executeRefunds(ctx, es, out.refunds)
	return &WebhookOutcome{Duplicate: out.duplicate, Ignored: out.ignored}, nil
}

// webhookCandidates returns the configurations of provider for es a
// delivery may be verified with (live first, then test; ErrWebhookTarget
// when none exists) and its driver.
func (s *Service) webhookCandidates(ctx context.Context, es store.EstablishmentScope, provider string) ([]*online.ProviderConfig, ext.PaymentProvider, error) {
	var (
		out []*online.ProviderConfig
		pp  ext.PaymentProvider
	)
	for _, mode := range []string{store.ProviderModeLive, store.ProviderModeTest} {
		cfg, drv, err := s.provider(ctx, s.d.DB, es, provider, mode)
		if errors.Is(err, ErrProviderNotConfigured) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		out, pp = append(out, cfg), drv
	}
	if len(out) == 0 {
		return nil, nil, ErrWebhookTarget
	}
	return out, pp, nil
}

// applied is the outcome of apply.
type applied struct {
	duplicate, ignored bool
	// changed: the intent status changed (reconciliation statistics).
	changed bool
	// refunds are refund intents created by the event, to send after commit.
	refunds []string
}

// syntheticEventID is the dedupe id of an outcome found by reconciliation:
// one per reference and outcome, so the same outcome is applied once and a
// late webhook of the same outcome is a no-op on the intent state.
func syntheticEventID(ref string, ev *ext.PaymentEvent) string {
	id := "reconcile:" + ref + ":" + string(ev.Type)
	if o := ev.Metadata[MetaOutcome]; o != "" {
		id += ":" + o
	}
	if ev.Type == ext.PaymentRefunded {
		id += ":" + strconv.FormatInt(ev.Amount, 10)
	}
	return id
}

// apply runs one verified provider event in ONE transaction (spec 13.3):
//
//	event of another establishment -> ignored, nothing recorded
//	insert webhook_events (establishment, provider, event_id)   unique: duplicate -> no effect (INV-23)
//	resolve + lock the intent (scoped to es)
//	payment intent:
//	  succeeded -> Ledger.RecordProviderPayment (ledger, paid_amount,
//	               confirmation through the booking service: INV-19);
//	               late payment / overpayment -> refund intent (ADR 0014)
//	  pending   -> provider reference saved
//	  failed    -> failed / expired + payment.failed (terminal outcomes),
//	               failure reason only for a failed attempt
//	  refunded  -> payment ensured, provider refunds settled into the ledger
//	refund intent:
//	  refunded  -> Ledger.RecordProviderRefund; failed -> failed
//
// Intent states only move forward: a late "expired" never undoes a
// success, a success after "failed"/"expired"/"cancelled" is applied (the
// money arrived), so out-of-order deliveries converge.
func (s *Service) apply(ctx context.Context, es store.EstablishmentScope, provider string, cfg *online.ProviderConfig, ev *ext.PaymentEvent, source string) (*applied, error) {
	if ev.Metadata == nil {
		ev.Metadata = map[string]string{}
	}
	eventID := ev.ProviderEventID
	if eventID == "" {
		eventID = syntheticEventID(ev.ProviderRef, ev)
	}
	out := &applied{}
	if ev.EstablishmentID != "" && ev.EstablishmentID != es.ID() {
		// Another establishment's payment (one provider account, two
		// endpoints): authentic but not ours. Nothing is recorded, so the
		// delivery to the right endpoint is applied normally (INV-23 is per
		// establishment).
		s.d.Log.InfoContext(ctx, "gateway: event of another establishment ignored", "provider", provider, "establishment_id", es.ID(), "event_id", eventID)
		return &applied{ignored: true}, nil
	}
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		*out = applied{}
		now := s.now()
		we := &store.WebhookEvent{
			ID: store.NewID(), Provider: provider, EventID: eventID, EventType: eventType(ev),
			Status: store.WebhookEventProcessed, ReceivedAt: now, ProcessedAt: &now,
		}
		if err := store.InsertWebhookEvent(ctx, tx, es, we); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return errDuplicate
			}
			return fmt.Errorf("gateway: %w", err)
		}
		pi, err := s.resolve(ctx, tx, es, provider, ev)
		if errors.Is(err, store.ErrNotFound) {
			if s.mayArriveLater(es, ev) {
				return errRetryLater
			}
			return s.ignore(ctx, tx, es, we, out, "unknown_reference")
		}
		if err != nil {
			return err
		}
		if cfg != nil && cfg.Mode != "" && pi.Mode != "" && pi.Mode != cfg.Mode {
			// An event verified with the credentials of one mode never
			// touches an intent of the other (spec 15.4, INV-74): a test
			// delivery cannot confirm a real booking.
			s.d.Log.WarnContext(ctx, "gateway: event mode does not match the intent", "provider", provider, "establishment_id", es.ID(), "event_id", eventID, "event_mode", cfg.Mode, "intent_mode", pi.Mode)
			return s.ignore(ctx, tx, es, we, out, "mode_mismatch")
		}
		before := pi.Status
		x := &txn{s: s, tx: tx, es: es, cfg: cfg, ev: ev, eventID: eventID, source: source, out: out}
		testKeys, err := x.testKeysOnRealBooking(ctx, pi)
		if err != nil {
			return err
		}
		if testKeys {
			// A test-mode session opened for a real booking before INV-88
			// blocked such starts: a test card never confirms a real
			// booking (not even as a late payment). An open intent fails,
			// nothing is recorded.
			s.d.Log.WarnContext(ctx, "gateway: test-mode payment of a real booking not applied", "provider", provider, "establishment_id", es.ID(), "event_id", eventID, "intent_id", pi.ID)
			if !isOpen(pi.Status) {
				return s.ignore(ctx, tx, es, we, out, failureTestKeys)
			}
		}
		switch {
		case testKeys:
			err = x.close(ctx, pi, store.IntentFailed, failureTestKeys)
		case pi.Kind == store.IntentKindPayment:
			err = x.onPayment(ctx, pi)
		case pi.Kind == store.IntentKindRefund:
			err = x.onRefund(ctx, pi)
		}
		if err != nil {
			return err
		}
		appendEvent(pi, eventID)
		pi.UpdatedAt = s.now()
		if source == sourceReconcile {
			pi.LastCheckedAt = &now
		}
		if err := store.UpdatePaymentIntent(ctx, tx, es, pi,
			"status", "external_ref", "payment_id", "raw_event_ids", "failure_reason", "last_checked_at"); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		out.changed = pi.Status != before
		we.PaymentIntentID = &pi.ID
		if _, err := tx.NewUpdate().Model(we).Column("payment_intent_id").WherePK().Exec(ctx); err != nil {
			return fmt.Errorf("gateway: webhook event: %w", err)
		}
		return nil
	})
	switch {
	case errors.Is(err, errDuplicate):
		return &applied{duplicate: true}, nil
	case err != nil && store.IsUniqueViolation(err):
		// Two deliveries of the same event raced past the insert (the
		// loser's insert failed only at commit time on some drivers).
		if ok, xerr := store.WebhookEventExists(ctx, s.d.DB, es, provider, eventID); xerr == nil && ok {
			return &applied{duplicate: true}, nil
		}
		return nil, err
	case err != nil:
		return nil, err
	}
	return out, nil
}

func eventType(ev *ext.PaymentEvent) string {
	t := string(ev.Type)
	if o := ev.Metadata[MetaOutcome]; o != "" {
		t += ":" + o
	}
	return t
}

// ignore marks the event as authentic but not applicable (recorded, so it
// is not reprocessed).
func (s *Service) ignore(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, we *store.WebhookEvent, out *applied, reason string) error {
	out.ignored = true
	we.Status, we.Error = store.WebhookEventIgnored, &reason
	if _, err := tx.NewUpdate().Model(we).Column("status", "error").WherePK().Exec(ctx); err != nil {
		return fmt.Errorf("gateway: webhook event: %w", err)
	}
	return nil
}

// resolve finds the intent of an event in es: by provider reference
// (external_id, then external_ref), then by the intent id in metadata.
// The returned intent is locked for the rest of tx.
func (s *Service) resolve(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, provider string, ev *ext.PaymentEvent) (*store.PaymentIntent, error) {
	pi, err := store.FindPaymentIntentByRef(ctx, tx, es, provider, ev.ProviderRef)
	if errors.Is(err, store.ErrNotFound) && ev.Metadata[MetaIntentID] != "" {
		if _, perr := uuid.Parse(ev.Metadata[MetaIntentID]); perr == nil {
			pi, err = store.GetPaymentIntent(ctx, tx, es, ev.Metadata[MetaIntentID])
			if err == nil && pi.Provider != provider {
				err = store.ErrNotFound
			}
		}
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("gateway: %w", err)
	}
	// Lock order: the provider payment before its refunds, then the booking.
	if pi.Kind == store.IntentKindRefund && pi.RefundOfIntentID != nil {
		if _, err := store.LockPaymentIntent(ctx, tx, es, *pi.RefundOfIntentID); err != nil {
			return nil, fmt.Errorf("gateway: %w", err)
		}
	}
	locked, err := store.LockPaymentIntent(ctx, tx, es, pi.ID)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	return locked, nil
}

// mayArriveLater reports whether an event for an unknown reference should
// be retried by the provider rather than ignored: it names an Arenile
// intent or this establishment (the intent may not be committed yet), or
// it is a recent refund of a payment whose reference is not stored yet.
func (s *Service) mayArriveLater(es store.EstablishmentScope, ev *ext.PaymentEvent) bool {
	if ev.Metadata[MetaIntentID] != "" || ev.EstablishmentID == es.ID() {
		return true
	}
	return ev.Type == ext.PaymentRefunded && !ev.OccurredAt.IsZero() && s.now().Sub(ev.OccurredAt) < unknownRefRetryWindow
}

// txn carries the state of one apply transaction.
type txn struct {
	s       *Service
	tx      bun.IDB
	es      store.EstablishmentScope
	cfg     *online.ProviderConfig
	ev      *ext.PaymentEvent
	eventID string
	source  string
	out     *applied
}

func (x *txn) onPayment(ctx context.Context, pi *store.PaymentIntent) error {
	ev := x.ev
	if ref := ev.Metadata[MetaPaymentIntent]; ref != "" && ref != pi.ExternalID && pi.ExternalRef == nil {
		pi.ExternalRef = &ref
	}
	switch ev.Type {
	case ext.PaymentSucceeded:
		return x.succeed(ctx, pi, money.Cents(ev.Amount))
	case ext.PaymentPending:
		if pi.Status == store.IntentCreated {
			pi.Status = store.IntentPending
		}
		return nil
	case ext.PaymentFailed:
		reason := ev.FailureReason
		switch ev.Metadata[MetaOutcome] {
		case OutcomeAttemptFailed:
			// The session (or payment intent) is still payable.
			if isOpen(pi.Status) {
				pi.FailureReason = strPtr(reason)
			}
			return nil
		case OutcomeExpired:
			return x.close(ctx, pi, store.IntentExpired, reason)
		default:
			return x.close(ctx, pi, store.IntentFailed, reason)
		}
	case ext.PaymentRefunded:
		if ev.Metadata[MetaOutcome] != OutcomeRefundTotal {
			return nil
		}
		if pi.Status != store.IntentSucceeded {
			// A refund implies the payment succeeded: apply it first
			// (out-of-order delivery).
			captured, _ := strconv.ParseInt(ev.Metadata[MetaAmountCaptured], 10, 64)
			if err := x.succeed(ctx, pi, money.Cents(captured)); err != nil {
				return err
			}
		}
		return x.settleRefunds(ctx, pi, money.Cents(ev.Amount))
	}
	return nil
}

// failureTestKeys is the failure reason of a test-mode payment of a real
// booking (INV-88).
const failureTestKeys = "test_keys_real_booking"

// testKeysOnRealBooking reports whether a success event would pay a real
// booking (bookings.test = false) through an intent in mode test not yet
// succeeded: a session opened before INV-88 blocked such starts. Refunds
// and payments already recorded are never affected.
func (x *txn) testKeysOnRealBooking(ctx context.Context, pi *store.PaymentIntent) (bool, error) {
	if x.ev.Type != ext.PaymentSucceeded || pi.Kind != store.IntentKindPayment || pi.Mode != store.ProviderModeTest ||
		pi.Status == store.IntentSucceeded || pi.BookingID == nil {
		return false, nil
	}
	b, err := store.GetBooking(ctx, x.tx, x.es, *pi.BookingID)
	if err != nil {
		return false, fmt.Errorf("gateway: %w", err)
	}
	return !b.Test, nil
}

// close moves an open payment intent to a terminal failure (payment.failed).
// The booking stays pending_payment until its hold expires (the page may
// retry).
func (x *txn) close(ctx context.Context, pi *store.PaymentIntent, status, reason string) error {
	if !isOpen(pi.Status) {
		return nil
	}
	pi.Status = status
	pi.FailureReason = strPtr(reason)
	return x.s.record(ctx, x.tx, x.es, audit.SystemActor, "payment."+status, pi,
		map[string]any{"provider_event_id": x.eventID, "source": x.source, "failure_reason": reason},
		event.PaymentFailed, change(pi, "", store.PaymentDirectionIn, store.PaymentFailed))
}

// succeed applies a provider success to a payment intent (idempotent).
//
// Booking held or pending_payment: the ledger row confirms it (INV-19)
// when the paid amount reaches the threshold; below it (a total changed
// after the payment started) the booking stays held until its hold
// expires, when the booking service asks RefundExpiredBooking to refund
// what was paid. A hold already elapsed is expired first (HoldExpirer) and
// the payment follows the late-payment path.
// Booking expired (late payment): the money is recorded, then the
// LateConfirmer re-allocates and confirms inside a savepoint; if it is
// missing or fails, the whole payment is refunded automatically.
// Booking cancelled, rejected or closed: recorded and refunded.
// Any other status (confirmed, awaiting approval, checked in): recorded;
// the part above the booking total is refunded (double payment).
func (x *txn) succeed(ctx context.Context, pi *store.PaymentIntent, amount money.Cents) error {
	if pi.Status == store.IntentSucceeded && pi.PaymentID != nil {
		return nil
	}
	if amount <= 0 {
		amount = pi.Amount
	}
	if pi.BookingID == nil {
		return fmt.Errorf("gateway: payment intent %s without booking", pi.ID)
	}
	// Lock the booking before reading its status: the hold expiry job
	// cannot expire it between this decision and the ledger write.
	b, err := store.LockBooking(ctx, x.tx, x.es, *pi.BookingID)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	normal := b.Status == bookingHeld || b.Status == bookingPendingPayment
	if normal && b.HoldExpiresAt != nil && !b.HoldExpiresAt.After(x.s.now()) && x.s.d.HoldExpirer != nil {
		// The hold elapsed but the sweeper has not run yet: confirming would
		// fail on the expired hold and the whole delivery would be retried.
		// Expire the booking here (audit, outbox, resources released) and
		// treat the money as a late payment (INV-05, INV-51).
		if err := x.s.d.HoldExpirer.ExpireHold(ctx, x.tx, x.es, b.ID); err != nil {
			return fmt.Errorf("gateway: expire elapsed hold: %w", err)
		}
		b.Status, normal = bookingExpired, false
	}
	terminal := isTerminalBooking(b.Status)
	record := func() (*payment.Result, error) {
		return x.s.d.Ledger.RecordProviderPayment(ctx, x.tx, x.es, payment.ProviderPayment{
			BookingID: b.ID, Provider: pi.Provider, ProviderRef: pi.ExternalID, Amount: amount,
			Method: store.PaymentMethodCard, IntentID: pi.ID, EventID: x.eventID, NoConfirm: terminal,
			Test: pi.Mode == store.ProviderModeTest,
		})
	}
	res, err := x.inSavepoint(ctx, normal && x.s.d.HoldExpirer != nil, record)
	if errors.Is(err, availability.ErrHoldExpired) && normal && x.s.d.HoldExpirer != nil {
		// The hold ran out between the check above and the confirmation
		// (the inventory decides with its own clock): same late-payment
		// path as an elapsed hold, after the savepoint rolled the partial
		// ledger write back.
		if xerr := x.s.d.HoldExpirer.ExpireHold(ctx, x.tx, x.es, b.ID); xerr != nil {
			return fmt.Errorf("gateway: expire elapsed hold: %w", xerr)
		}
		b.Status, normal, terminal = bookingExpired, false, true
		res, err = record()
	}
	if err != nil {
		return fmt.Errorf("gateway: record provider payment: %w", err)
	}
	pi.Status, pi.FailureReason = store.IntentSucceeded, nil
	pi.PaymentID = &res.Payment.ID
	if res.Replayed {
		return nil
	}
	diff := map[string]any{"provider_event_id": x.eventID, "source": x.source, "payment_id": res.Payment.ID, "booking_status": b.Status}
	if amount != pi.Amount {
		diff["expected_amount"] = int64(pi.Amount)
		diff["received_amount"] = int64(amount)
	}
	if err := x.s.record(ctx, x.tx, x.es, audit.SystemActor, "payment.provider_succeeded", pi, diff, "", event.PaymentChange{}); err != nil {
		return err
	}
	switch {
	case normal:
		return x.refundExcess(ctx, pi, res, amount)
	case b.Status == bookingExpired:
		if x.lateConfirm(ctx, b.ID, &res.Payment) {
			return x.refundExcess(ctx, pi, res, amount)
		}
		return x.schedule(ctx, pi, res.Payment.ID, amount, "late_payment", "Pagamento arrivato dopo la scadenza della prenotazione: risorse non più disponibili")
	case terminal:
		return x.schedule(ctx, pi, res.Payment.ID, amount, "booking_closed", "Pagamento di una prenotazione non più attiva")
	default:
		return x.refundExcess(ctx, pi, res, amount)
	}
}

// isTerminalBooking: statuses where a payment can no longer confirm
// anything (expired is handled separately as a late payment).
func isTerminalBooking(status string) bool {
	switch status {
	case bookingExpired, "cancelled", "rejected", "completed", "no_show", "draft":
		return true
	}
	return false
}

// savepointer is the part of bun.Tx that opens a nested transaction.
type savepointer interface {
	BeginTx(context.Context, *sql.TxOptions) (bun.Tx, error)
}

// inSavepoint runs fn inside a savepoint when wanted (and the transaction
// supports it), so a failure leaves tx usable; otherwise fn runs directly.
func (x *txn) inSavepoint(ctx context.Context, wanted bool, fn func() (*payment.Result, error)) (*payment.Result, error) {
	nested, ok := x.tx.(savepointer)
	if !wanted || !ok {
		return fn()
	}
	sp, err := nested.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("gateway: savepoint: %w", err)
	}
	outer := x.tx
	x.tx = sp
	res, err := fn()
	x.tx = outer
	if err != nil {
		_ = sp.Rollback()
		return nil, err
	}
	if err := sp.Commit(); err != nil {
		return nil, fmt.Errorf("gateway: release savepoint: %w", err)
	}
	return res, nil
}

// lateConfirm tries the LateConfirmer inside a savepoint.
func (x *txn) lateConfirm(ctx context.Context, bookingID string, pm *store.Payment) bool {
	lc := x.s.d.LateConfirmer
	if lc == nil {
		return false
	}
	nested, ok := x.tx.(savepointer)
	if !ok {
		return false
	}
	sp, err := nested.BeginTx(ctx, nil)
	if err != nil {
		x.s.d.Log.WarnContext(ctx, "gateway: savepoint for late payment", "err", err)
		return false
	}
	if err := lc.ConfirmLatePayment(ctx, sp, x.es, bookingID, pm); err != nil {
		_ = sp.Rollback()
		x.s.d.Log.InfoContext(ctx, "gateway: late payment cannot confirm the booking, refunding", "establishment_id", x.es.ID(), "booking_id", bookingID, "reason", err.Error())
		return false
	}
	if err := sp.Commit(); err != nil {
		x.s.d.Log.WarnContext(ctx, "gateway: release savepoint", "err", err)
		return false
	}
	return true
}

// refundExcess schedules the refund of what a payment adds above the
// booking total (double payment of two attempts).
func (x *txn) refundExcess(ctx context.Context, pi *store.PaymentIntent, res *payment.Result, amount money.Cents) error {
	b, err := store.GetBooking(ctx, x.tx, x.es, *pi.BookingID)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	excess := min(b.PaidAmount-b.TotalAmount, amount)
	if excess <= 0 {
		return nil
	}
	return x.schedule(ctx, pi, res.Payment.ID, excess, "overpayment", "Pagamento eccedente il totale della prenotazione")
}

// schedule creates a refund intent of pi (status created, sent to the
// provider after commit) and writes payment.refund_scheduled. Idempotent
// per payment intent and reason.
func (x *txn) schedule(ctx context.Context, pi *store.PaymentIntent, paymentID string, amount money.Cents, reason, note string) error {
	key := "auto-refund:" + pi.ID + ":" + reason
	if _, err := store.GetPaymentIntentByIdempotencyKey(ctx, x.tx, x.es, key); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("gateway: %w", err)
	}
	now := x.s.now()
	id := store.NewID()
	ri := &store.PaymentIntent{
		ID: id, BookingID: pi.BookingID, RefundOfIntentID: &pi.ID, Provider: pi.Provider, Mode: pi.Mode,
		Kind: store.IntentKindRefund, ExternalID: placeholderPrefix + id, Status: store.IntentCreated,
		Amount: amount, Currency: pi.Currency, Attempt: 1, IdempotencyKey: &key, RawEventIDs: []string{},
		FailureReason: nil, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertPaymentIntent(ctx, x.tx, x.es, ri); err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	x.out.refunds = append(x.out.refunds, ri.ID)
	return x.s.record(ctx, x.tx, x.es, audit.SystemActor, "payment.refund_scheduled", ri,
		map[string]any{"reason": reason, "note": note, "payment_id": paymentID, "refund_of_intent_id": pi.ID},
		EventRefundScheduled, change(ri, paymentID, store.PaymentDirectionRefund, store.PaymentPending))
}

// settleRefunds brings the ledger refunds of a provider payment up to the
// cumulative amount refunded at the provider: open refund intents are
// completed oldest first, a remainder (refund made in the provider
// dashboard) is recorded with a reference derived from the cumulative
// amount, so a replay records nothing.
func (x *txn) settleRefunds(ctx context.Context, pi *store.PaymentIntent, cumulative money.Cents) error {
	if pi.PaymentID == nil || cumulative <= 0 {
		return nil
	}
	recorded, err := store.RefundedOfPayment(ctx, x.tx, x.es, *pi.PaymentID)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	open, err := store.ListRefundIntents(ctx, x.tx, x.es, pi.ID, true)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	for i := range open {
		ri := &open[i]
		if recorded+ri.Amount > cumulative {
			break
		}
		if err := x.completeRefund(ctx, pi, ri); err != nil {
			return err
		}
		if err := store.UpdatePaymentIntent(ctx, x.tx, x.es, ri, "status", "payment_id", "raw_event_ids", "failure_reason"); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		recorded += ri.Amount
	}
	if rest := cumulative - recorded; rest > 0 {
		ref := fmt.Sprintf("%s:refunded:%d", pi.ExternalID, int64(cumulative))
		if _, err := x.s.d.Ledger.RecordProviderRefund(ctx, x.tx, x.es, payment.ProviderRefund{
			BookingID: *pi.BookingID, PaymentID: *pi.PaymentID, Provider: pi.Provider, ProviderRef: ref, Amount: rest,
			Reason: "Rimborso eseguito dal pannello del provider", IntentID: pi.ID, EventID: x.eventID,
		}); err != nil {
			return fmt.Errorf("gateway: record provider refund: %w", err)
		}
	}
	return nil
}

// completeRefund marks a refund intent succeeded and writes its ledger row
// (provider_ref = the refund intent id: stable whether or not the provider
// refund id is already known).
func (x *txn) completeRefund(ctx context.Context, pay, ri *store.PaymentIntent) error {
	if pay.PaymentID == nil || ri.BookingID == nil {
		return fmt.Errorf("gateway: refund intent %s without payment", ri.ID)
	}
	pm, err := x.s.d.Ledger.RecordProviderRefund(ctx, x.tx, x.es, payment.ProviderRefund{
		BookingID: *ri.BookingID, PaymentID: *pay.PaymentID, Provider: ri.Provider, ProviderRef: ri.ID, Amount: ri.Amount,
		Reason: "Rimborso tramite " + ri.Provider, IntentID: ri.ID, EventID: x.eventID,
	})
	if err != nil {
		return fmt.Errorf("gateway: record provider refund: %w", err)
	}
	ri.Status, ri.FailureReason, ri.PaymentID = store.IntentSucceeded, nil, &pm.ID
	appendEvent(ri, x.eventID)
	return nil
}

func (x *txn) onRefund(ctx context.Context, ri *store.PaymentIntent) error {
	if !isOpen(ri.Status) || ri.RefundOfIntentID == nil {
		return nil
	}
	switch {
	case x.ev.Type == ext.PaymentRefunded && x.ev.Metadata[MetaOutcome] == OutcomeRefundSingle:
		pay, err := store.GetPaymentIntent(ctx, x.tx, x.es, *ri.RefundOfIntentID) // locked by resolve
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		return x.completeRefund(ctx, pay, ri)
	case x.ev.Type == ext.PaymentFailed:
		ri.Status = store.IntentFailed
		ri.FailureReason = strPtr(x.ev.FailureReason)
		return x.s.record(ctx, x.tx, x.es, audit.SystemActor, "payment.refund_failed", ri,
			map[string]any{"provider_event_id": x.eventID, "source": x.source, "failure_reason": x.ev.FailureReason}, "", event.PaymentChange{})
	}
	return nil
}
