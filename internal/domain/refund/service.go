package refund

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// JobSend is the name of the job that sends due refund requests.
const JobSend = "refunds.send"

// DefaultMaxAttempts is ARENILE_REFUND_MAX_ATTEMPTS when not configured.
const DefaultMaxAttempts = 10

const (
	// jobInterval is the period of JobSend.
	jobInterval = 30 * time.Second
	// jobBatch bounds the requests handled by one run.
	jobBatch = 100
	// checkInterval is the delay between two looks at a refund the
	// provider accepted but has not confirmed yet (the webhook or the
	// gateway reconciliation completes the refund intent; this job only
	// mirrors its outcome).
	checkInterval = 2 * time.Minute
)

// IdempotencyWindow is how long a provider is assumed to remember an
// idempotency key (Stripe: 24 hours). A refund intent still created (the
// provider's answer was lost) is never resent with the same key after it:
// the provider could treat the resend as a new refund. The request is dead
// lettered (idempotency_window_elapsed) for a manual reconciliation on the
// provider's dashboard before Riprova.
const IdempotencyWindow = 24 * time.Hour

// backoffSteps is the delay before attempt n+1 after attempt n failed
// (index n-1); later attempts wait the last step.
var backoffSteps = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 12 * time.Hour}

// Audit actions (entity booking, so they appear in the booking history).
const (
	ActionReserved  = "booking.refund_reserved"
	ActionSucceeded = "booking.refund_succeeded"
	ActionFailed    = "booking.refund_failed"
	ActionRetried   = "booking.refund_retried"
)

// NotificationKindRefundFailed is the admin notification of a request that
// exhausted its attempts.
const NotificationKindRefundFailed = "refund_failed"

// Sender sends a refund to the provider of a payment. Implemented by
// *gateway.Service (RefundViaProvider): it reserves a refund intent with
// the given idempotency key (replayed when the key was already used),
// calls the provider and records the answer.
type Sender interface {
	RefundViaProvider(ctx context.Context, es store.EstablishmentScope, req gateway.RefundRequest) (*store.PaymentIntent, error)
}

// Alerter is the OPTIONAL e-mail alert to the owner when a request
// exhausts its attempts (the in-panel notification is always written).
// Called after the commit; an error is logged only.
type Alerter interface {
	RefundFailed(ctx context.Context, es store.EstablishmentScope, rr store.RefundRequest) error
}

// Deps are the dependencies of the refund service.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Log    *slog.Logger
	Sender Sender
	// Alerter is optional.
	Alerter Alerter
	// MaxAttempts is ARENILE_REFUND_MAX_ATTEMPTS (default DefaultMaxAttempts).
	MaxAttempts int
}

// Service reserves, sends and exposes the guaranteed refunds.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.MaxAttempts <= 0 {
		d.MaxAttempts = DefaultMaxAttempts
	}
	return &Service{d: d}
}

// SetAlerter installs the owner e-mail alert after construction.
func (s *Service) SetAlerter(a Alerter) { s.d.Alerter = a }

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// Backoff is the delay after the n-th failed attempt (n >= 1).
func Backoff(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	if n > len(backoffSteps) {
		return backoffSteps[len(backoffSteps)-1]
	}
	return backoffSteps[n-1]
}

// IdempotencyKey is the key of the request of a booking and source intent.
func IdempotencyKey(bookingID, paymentIntentID string) string {
	return "refund:" + bookingID + ":" + paymentIntentID
}

// intentKey is the idempotency key of the refund intent of attempt n:
// the request key for the first intent, a derived key for an intent that
// replaces one the provider refused (failed: no money moved).
func intentKey(rr *store.RefundRequest, n int) string {
	if n <= 1 {
		return rr.IdempotencyKey
	}
	return rr.IdempotencyKey + ":a" + strconv.Itoa(n)
}

// ---- reserve ----------------------------------------------------------------------

// Reservation is the outcome of Reserve.
type Reservation struct {
	// Amount is the part of the requested amount reserved on provider
	// payments; the rest (desk payments, or nothing left) stays a manual
	// refund due.
	Amount   money.Cents
	Requests []store.RefundRequest
}

// ReserveInput describes a cancellation refund.
type ReserveInput struct {
	Booking *store.Booking
	Amount  money.Cents
	// Reason is store.RefundReason*.
	Reason    string
	CreatedBy *string
}

// Reserve inserts, in the caller's transaction (the cancellation, with the
// booking locked), one pending refund request per provider payment of the
// booking for up to in.Amount, newest payment first. Each part is capped
// by what is still refundable on that payment (amount minus confirmed
// refunds, refund intents in flight and open requests), so the total never
// exceeds what was captured (INV-24). The manual driver is not a provider
// payment. Implements booking.CancelRefunds.
func (s *Service) Reserve(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in ReserveInput) (*Reservation, error) {
	out := &Reservation{}
	b := in.Booking
	if b == nil || in.Amount <= 0 {
		return out, nil
	}
	intents, err := store.ListSucceededPaymentIntents(ctx, tx, es, b.ID)
	if err != nil {
		return nil, fmt.Errorf("refund: %w", err)
	}
	now := s.now()
	left := in.Amount
	for i := len(intents) - 1; i >= 0 && left > 0; i-- {
		pi := &intents[i]
		if pi.PaymentID == nil || pi.Provider == "" || pi.Provider == "manual" {
			continue
		}
		pm, err := store.GetPayment(ctx, tx, es, *pi.PaymentID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("refund: %w", err)
		}
		refundable, err := refundableOf(ctx, tx, es, pi, pm)
		if err != nil {
			return nil, err
		}
		part := min(left, refundable)
		if part <= 0 {
			continue
		}
		at := now
		rr := store.RefundRequest{
			ID: store.NewID(), BookingID: b.ID, PaymentIntentID: pi.ID, Provider: pi.Provider, Amount: part,
			Currency: pi.Currency, Reason: in.Reason, Status: store.RefundRequestPending,
			IdempotencyKey: IdempotencyKey(b.ID, pi.ID), NextAttemptAt: &at, Test: b.Test || pm.Test,
			CreatedByUserID: in.CreatedBy, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if rr.Currency == "" {
			rr.Currency = "EUR"
		}
		if err := store.InsertRefundRequest(ctx, tx, es, &rr); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return nil, ErrRefundInProgress
			}
			return nil, fmt.Errorf("refund: %w", err)
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: audit.SystemActor, Action: ActionReserved, EntityType: "booking", EntityID: b.ID,
			Diff: map[string]any{"refund_request_id": rr.ID, "payment_intent_id": pi.ID, "amount": int64(part), "reason": in.Reason},
		}); err != nil {
			return nil, err
		}
		out.Requests = append(out.Requests, rr)
		out.Amount += part
		left -= part
	}
	return out, nil
}

// refundableOf is what can still be refunded on one provider payment.
func refundableOf(ctx context.Context, db bun.IDB, es store.EstablishmentScope, pi *store.PaymentIntent, pm *store.Payment) (money.Cents, error) {
	done, err := store.RefundedOfPayment(ctx, db, es, pm.ID)
	if err != nil {
		return 0, fmt.Errorf("refund: %w", err)
	}
	inFlight, err := store.SumOpenRefundIntents(ctx, db, es, pi.ID, "")
	if err != nil {
		return 0, fmt.Errorf("refund: %w", err)
	}
	open, err := store.SumOpenRefundRequests(ctx, db, es, "", pi.ID)
	if err != nil {
		return 0, fmt.Errorf("refund: %w", err)
	}
	return pm.Amount - done - inFlight - open, nil
}

// ---- job --------------------------------------------------------------------------

// Jobs returns the background job sending due refund requests.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name: JobSend, Interval: jobInterval, RunOnStart: true, Timeout: 5 * time.Minute,
		Run: func(ctx context.Context) error {
			_, err := s.RunDue(ctx)
			return err
		},
	}}
}

// RunDue handles the due requests across establishments and returns how
// many were handled. A failure on one request is logged and never stops
// the others.
func (s *Service) RunDue(ctx context.Context) (int, error) {
	refs, err := store.ListDueRefundRequests(ctx, s.d.DB, s.now(), jobBatch)
	if err != nil {
		return 0, fmt.Errorf("refund: %w", err)
	}
	scopes := map[string]store.EstablishmentScope{}
	n := 0
	for _, ref := range refs {
		if ctx.Err() != nil {
			break
		}
		es, ok := scopes[ref.EstablishmentID]
		if !ok {
			if es, err = store.SystemEstablishmentScope(ctx, s.d.DB, ref.EstablishmentID); err != nil {
				continue
			}
			scopes[ref.EstablishmentID] = es
		}
		if err := s.Process(ctx, es, ref.ID); err != nil {
			s.d.Log.WarnContext(ctx, "refund: request not processed", slog.String("establishment_id", es.ID()),
				slog.String("refund_request_id", ref.ID), slog.String("error", err.Error()))
			continue
		}
		n++
	}
	return n, nil
}

// SendNow processes the given requests at once (best effort, after the
// cancellation committed): the customer usually sees the refund accepted
// by the provider before the page reloads. Errors are logged only: the
// job retries.
func (s *Service) SendNow(ctx context.Context, es store.EstablishmentScope, ids []string) {
	for _, id := range ids {
		if err := s.Process(ctx, es, id); err != nil {
			s.d.Log.WarnContext(ctx, "refund: immediate send failed, the job will retry", slog.String("establishment_id", es.ID()),
				slog.String("refund_request_id", id), slog.String("error", err.Error()))
		}
	}
}

// Process advances one request if it is due: a pending request is sent
// (claimed first with a lease, so concurrent workers send it once), a
// processing one mirrors the state of its refund intent. Safe to run
// concurrently and repeatedly.
func (s *Service) Process(ctx context.Context, es store.EstablishmentScope, id string) error {
	rr, err := store.GetRefundRequest(ctx, s.d.DB, es, id)
	if err != nil {
		return fmt.Errorf("refund: %w", err)
	}
	now := s.now()
	if rr.NextAttemptAt == nil || rr.NextAttemptAt.After(now) {
		return nil
	}
	switch rr.Status {
	case store.RefundRequestPending:
		return s.send(ctx, es, rr)
	case store.RefundRequestProcessing:
		if rr.RefundIntentID == nil {
			return s.send(ctx, es, rr)
		}
		ri, err := store.GetPaymentIntent(ctx, s.d.DB, es, *rr.RefundIntentID)
		if err != nil {
			return fmt.Errorf("refund: %w", err)
		}
		if ri.Status == store.IntentCreated {
			// Accepted locally but the provider was not reached: send again
			// with the same key.
			return s.send(ctx, es, rr)
		}
		return s.observe(ctx, es, rr.ID, rr.Version, ri, "")
	}
	return nil
}

// send claims rr (attempts+1, next_attempt_at = now + backoff as the
// lease), sends it to the provider and records the outcome.
func (s *Service) send(ctx context.Context, es store.EstablishmentScope, rr *store.RefundRequest) error {
	now := s.now()
	expected := rr.Version
	rr.Attempts++
	lease := now.Add(Backoff(s.cycleAttempt(rr.Attempts)))
	rr.NextAttemptAt, rr.UpdatedAt = &lease, now
	if err := store.UpdateRefundRequest(ctx, s.d.DB, es, rr, expected, "attempts", "next_attempt_at"); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil // another worker claimed it
		}
		return fmt.Errorf("refund: claim: %w", err)
	}
	pay, err := store.GetPaymentIntent(ctx, s.d.DB, es, rr.PaymentIntentID)
	if err != nil {
		return fmt.Errorf("refund: %w", err)
	}
	if pay.PaymentID == nil {
		return s.fail(ctx, es, rr.ID, rr.Version, "payment_missing", true)
	}
	key, err := s.keyFor(ctx, es, rr)
	if err != nil {
		return err
	}
	if stale, err := s.staleIntent(ctx, es, key, now); err != nil {
		return err
	} else if stale != nil {
		return s.fail(ctx, es, rr.ID, rr.Version, "idempotency_window_elapsed", true, stale.ID)
	}
	ri, serr := s.d.Sender.RefundViaProvider(ctx, es, gateway.RefundRequest{
		BookingID: rr.BookingID, PaymentID: *pay.PaymentID, Amount: rr.Amount, Reason: rr.Reason,
		IdempotencyKey: key, Actor: audit.SystemActor,
	})
	if serr != nil {
		// A refusal leaves a failed intent under key: link it, so the next
		// attempt uses a new key.
		if prev, err := store.GetPaymentIntentByIdempotencyKey(ctx, s.d.DB, es, key); err == nil {
			ri = prev
		}
		if ri != nil && ri.Status != store.IntentFailed {
			return s.observe(ctx, es, rr.ID, rr.Version, ri, errorCode(serr))
		}
		return s.fail(ctx, es, rr.ID, rr.Version, errorCode(serr), permanent(serr), linkIntent(ri))
	}
	return s.observe(ctx, es, rr.ID, rr.Version, ri, "")
}

// staleIntent returns the refund intent under key when it is still created
// (the provider's answer was lost) and older than IdempotencyWindow, after
// marking it failed (failure_reason idempotency_window_elapsed): the key is
// never reused past the window (INV-75). nil otherwise.
func (s *Service) staleIntent(ctx context.Context, es store.EstablishmentScope, key string, now time.Time) (*store.PaymentIntent, error) {
	ri, err := store.GetPaymentIntentByIdempotencyKey(ctx, s.d.DB, es, key)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("refund: %w", err)
	}
	if ri.Status != store.IntentCreated || now.Sub(ri.CreatedAt) <= IdempotencyWindow {
		return nil, nil
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.LockPaymentIntent(ctx, tx, es, ri.ID)
		if err != nil {
			return err
		}
		if cur.Status != store.IntentCreated {
			return nil
		}
		reason := "idempotency_window_elapsed"
		cur.Status, cur.FailureReason, cur.UpdatedAt = store.IntentFailed, &reason, now
		if err := store.UpdatePaymentIntent(ctx, tx, es, cur, "status", "failure_reason"); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: audit.SystemActor, Action: "payment.refund_failed",
			EntityType: "payment_intent", EntityID: cur.ID,
			Diff: map[string]any{"failure_reason": reason, "idempotency_window_hours": int(IdempotencyWindow.Hours())},
		})
	})
	if err != nil {
		return nil, fmt.Errorf("refund: stale intent: %w", err)
	}
	return ri, nil
}

// keyFor is the idempotency key of the next send of rr: the key of its
// refund intent while that intent is alive (created, pending, succeeded),
// the key of an intent a previous attempt created but could not record
// (crash between the provider call and the update), a new key otherwise.
func (s *Service) keyFor(ctx context.Context, es store.EstablishmentScope, rr *store.RefundRequest) (string, error) {
	if rr.RefundIntentID != nil {
		ri, err := store.GetPaymentIntent(ctx, s.d.DB, es, *rr.RefundIntentID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", fmt.Errorf("refund: %w", err)
		}
		if err == nil && ri.Status != store.IntentFailed && ri.IdempotencyKey != nil {
			return *ri.IdempotencyKey, nil
		}
	}
	for n := 1; n < rr.Attempts; n++ {
		ri, err := store.GetPaymentIntentByIdempotencyKey(ctx, s.d.DB, es, intentKey(rr, n))
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("refund: %w", err)
		}
		if ri.Status != store.IntentFailed {
			return intentKey(rr, n), nil
		}
	}
	return intentKey(rr, rr.Attempts), nil
}

// observe records the state of the refund intent ri on the request
// (version expected): succeeded -> request succeeded; pending at the
// provider -> processing, looked at again after checkInterval; created
// (provider not reached) -> pending until the leased retry; failed ->
// a failed attempt.
func (s *Service) observe(ctx context.Context, es store.EstablishmentScope, id string, expected int64, ri *store.PaymentIntent, lastErr string) error {
	switch ri.Status {
	case store.IntentSucceeded:
		if ri.PaymentID != nil {
			return s.complete(ctx, es, id, expected, ri)
		}
	case store.IntentFailed:
		code := lastErr
		if code == "" {
			code = "provider_refused"
		}
		return s.fail(ctx, es, id, expected, code, false, ri.ID)
	case store.IntentCreated:
		if lastErr == "" {
			lastErr = "provider_unavailable"
		}
		return s.update(ctx, es, id, expected, func(rr *workRow) ([]string, error) {
			rr.RefundIntentID, rr.LastError = &ri.ID, &lastErr
			if s.exhausted(rr.Attempts) {
				return s.markFailed(ctx, rr)
			}
			rr.Status = store.RefundRequestPending
			return []string{"refund_intent_id", "last_error", "status"}, nil
		})
	}
	// Pending at the provider (or succeeded without its ledger row yet).
	return s.update(ctx, es, id, expected, func(rr *workRow) ([]string, error) {
		next := s.now().Add(checkInterval)
		rr.RefundIntentID, rr.Status, rr.NextAttemptAt, rr.LastError = &ri.ID, store.RefundRequestProcessing, &next, nil
		return []string{"refund_intent_id", "status", "next_attempt_at", "last_error"}, nil
	})
}

// complete marks the request succeeded with the ledger row of ri.
func (s *Service) complete(ctx context.Context, es store.EstablishmentScope, id string, expected int64, ri *store.PaymentIntent) error {
	return s.update(ctx, es, id, expected, func(rr *workRow) ([]string, error) {
		now := s.now()
		rr.Status, rr.RefundIntentID, rr.PaymentID = store.RefundRequestSucceeded, &ri.ID, ri.PaymentID
		rr.CompletedAt, rr.NextAttemptAt, rr.LastError = &now, nil, nil
		rr.postAudit = &audit.Entry{
			Action: ActionSucceeded,
			Diff:   map[string]any{"refund_request_id": rr.ID, "amount": int64(rr.Amount), "payment_id": *ri.PaymentID},
		}
		return []string{"status", "refund_intent_id", "payment_id", "completed_at", "next_attempt_at", "last_error"}, nil
	})
}

// linkIntent returns the id of ri when known.
func linkIntent(ri *store.PaymentIntent) string {
	if ri == nil {
		return ""
	}
	return ri.ID
}

// fail records a failed attempt: dead letter (failed, notification) when
// the error is permanent or the attempts of the cycle are exhausted,
// pending until the leased retry otherwise.
func (s *Service) fail(ctx context.Context, es store.EstablishmentScope, id string, expected int64, code string, perm bool, intentID ...string) error {
	return s.update(ctx, es, id, expected, func(rr *workRow) ([]string, error) {
		rr.LastError = &code
		cols := []string{"last_error"}
		if len(intentID) > 0 && intentID[0] != "" {
			iid := intentID[0]
			rr.RefundIntentID = &iid
			cols = append(cols, "refund_intent_id")
		}
		if perm || s.exhausted(rr.Attempts) {
			more, err := s.markFailed(ctx, rr)
			return append(cols, more...), err
		}
		rr.Status = store.RefundRequestPending
		return append(cols, "status"), nil
	})
}

// markFailed moves rr to failed and queues the notification.
func (s *Service) markFailed(_ context.Context, rr *workRow) ([]string, error) {
	rr.Status, rr.NextAttemptAt = store.RefundRequestFailed, nil
	rr.deadLetter = true
	rr.postAudit = &audit.Entry{
		Action: ActionFailed,
		Diff:   map[string]any{"refund_request_id": rr.ID, "amount": int64(rr.Amount), "attempts": rr.Attempts, "last_error": deref(rr.LastError)},
	}
	return []string{"status", "next_attempt_at"}, nil
}

// cycleAttempt is the position of attempt n in its retry cycle (a manual
// retry starts a new cycle of MaxAttempts; attempts never decrease, so the
// derived idempotency keys stay unique).
func (s *Service) cycleAttempt(n int) int {
	return (n-1)%s.d.MaxAttempts + 1
}

// exhausted reports whether attempt n is the last of its cycle.
func (s *Service) exhausted(n int) bool { return n > 0 && n%s.d.MaxAttempts == 0 }

// workRow wraps a request with the side effects of an update.
type workRow struct {
	*store.RefundRequest
	postAudit  *audit.Entry
	deadLetter bool
}

// update applies fn to the current request in one transaction, only when
// its version is still expected (a concurrent worker that changed it
// first wins: nothing is written), with the audit and, for a dead letter,
// the in-panel notification; the owner alert is sent after the commit.
func (s *Service) update(ctx context.Context, es store.EstablishmentScope, id string, expected int64, fn func(rr *workRow) ([]string, error)) error {
	var dead *store.RefundRequest
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetRefundRequest(ctx, tx, es, id)
		if err != nil {
			return fmt.Errorf("refund: %w", err)
		}
		if cur.Version != expected {
			return nil
		}
		w := &workRow{RefundRequest: cur}
		cols, err := fn(w)
		if err != nil {
			return err
		}
		w.UpdatedAt = s.now()
		if err := store.UpdateRefundRequest(ctx, tx, es, w.RefundRequest, expected, cols...); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return nil
			}
			return fmt.Errorf("refund: %w", err)
		}
		if w.postAudit != nil {
			e := *w.postAudit
			e.EstablishmentID, e.Actor, e.EntityType, e.EntityID = es.ID(), audit.SystemActor, "booking", cur.BookingID
			if err := s.d.Audit.Record(ctx, tx, e); err != nil {
				return err
			}
		}
		if w.deadLetter {
			if err := s.notifyFailed(ctx, tx, es, w.RefundRequest); err != nil {
				return err
			}
			cp := *w.RefundRequest
			dead = &cp
		}
		return nil
	})
	if err != nil {
		return err
	}
	if dead != nil && s.d.Alerter != nil {
		if err := s.d.Alerter.RefundFailed(ctx, es, *dead); err != nil {
			s.d.Log.WarnContext(ctx, "refund: owner alert not sent", slog.String("establishment_id", es.ID()),
				slog.String("refund_request_id", dead.ID), slog.String("error", err.Error()))
		}
	}
	return nil
}

// notifyFailed writes the in-panel notification of a dead letter (owner
// audience, ids and amounts only: no personal data).
func (s *Service) notifyFailed(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, rr *store.RefundRequest) error {
	booking, rt := rr.BookingID, "booking"
	code := ""
	if b, err := store.GetBooking(ctx, tx, es, rr.BookingID); err == nil {
		code = b.Code
	}
	_, err := store.InsertRefundFailedNotification(ctx, tx, es, &store.AdminNotification{
		ID: store.NewID(), Kind: NotificationKindRefundFailed, Severity: "error", Audience: store.NotificationAudienceManager,
		Params: map[string]string{
			"refund_request_id": rr.ID, "booking_id": rr.BookingID, "booking_code": code,
			"amount": strconv.FormatInt(int64(rr.Amount), 10), "attempts": strconv.Itoa(rr.Attempts),
		},
		ResourceType: &rt, ResourceID: &booking,
		DedupeKey: "refund_failed:" + rr.ID + ":" + strconv.Itoa(rr.Attempts),
		CreatedAt: s.now(),
	})
	if err != nil {
		return fmt.Errorf("refund: %w", err)
	}
	return nil
}

// ---- panel ------------------------------------------------------------------------

// List returns a page of the requests of an establishment (auth.PermBookingsRead).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f store.RefundRequestFilter) (*store.Page[store.RefundRequestWithBooking], error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	switch f.Status {
	case "", store.RefundRequestPending, store.RefundRequestProcessing, store.RefundRequestSucceeded, store.RefundRequestFailed, store.RefundRequestCancelled:
	default:
		return nil, configerr.Invalid("status", "Stato non valido.")
	}
	if f.Cursor, err = store.DecodeCursor(f.Cursor); err != nil {
		return nil, err
	}
	return store.ListRefundRequests(ctx, s.d.DB, es, f)
}

// Retry moves a failed request back to pending, due now, starting a new
// cycle of attempts (auth.PermRefundsIssue). The request keeps its
// idempotency key. ErrNotRetryable (409 invalid_transition) in the other
// statuses. The send happens right away (best effort) and in the job.
func (s *Service) Retry(ctx context.Context, p *auth.Principal, ip, establishmentID, id string) (*store.RefundRequest, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermRefundsIssue)
	if err != nil {
		return nil, err
	}
	var out *store.RefundRequest
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		rr, err := store.GetRefundRequest(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if rr.Status != store.RefundRequestFailed {
			return ErrNotRetryable
		}
		now := s.now()
		expected := rr.Version
		rr.Status, rr.NextAttemptAt, rr.UpdatedAt = store.RefundRequestPending, &now, now
		// A new cycle: the attempts of the exhausted one stay counted (the
		// derived provider keys stay unique), the next failure count starts
		// again from the cycle start.
		if !s.exhausted(rr.Attempts) {
			rr.Attempts += s.d.MaxAttempts - s.cycleAttempt(rr.Attempts)
		}
		if err := store.UpdateRefundRequest(ctx, tx, es, rr, expected, "status", "next_attempt_at", "attempts"); err != nil {
			return err
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: ActionRetried, EntityType: "booking", EntityID: rr.BookingID,
			Diff: map[string]any{"refund_request_id": rr.ID, "amount": int64(rr.Amount)},
		}); err != nil {
			return err
		}
		out = rr
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.SendNow(ctx, es, []string{out.ID})
	if fresh, err := store.GetRefundRequest(ctx, s.d.DB, es, id); err == nil {
		out = fresh
	}
	return out, nil
}

// ---- errors -----------------------------------------------------------------------

// errorCode is the short code stored in last_error (never the provider
// message, which may echo request data).
func errorCode(err error) string {
	switch {
	case errors.Is(err, gateway.ErrProviderUnavailable):
		return "provider_unavailable"
	case errors.Is(err, gateway.ErrProviderRefused):
		return "provider_refused"
	case errors.Is(err, gateway.ErrProviderNotConfigured):
		return "provider_not_configured"
	case errors.Is(err, gateway.ErrRefundNotSupported):
		return "refund_not_supported"
	case errors.Is(err, gateway.ErrNotOnlinePayment):
		return "not_online_payment"
	case errors.Is(err, gateway.ErrIdempotencyMismatch):
		return "idempotency_mismatch"
	case errors.Is(err, configerr.ErrValidation):
		return "exceeds_refundable"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "interrupted"
	}
	return "internal_error"
}

// permanent reports whether retrying cannot help (dead letter at once).
func permanent(err error) bool {
	return errors.Is(err, gateway.ErrRefundNotSupported) || errors.Is(err, gateway.ErrNotOnlinePayment) ||
		errors.Is(err, gateway.ErrIdempotencyMismatch) || errors.Is(err, configerr.ErrValidation)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ReserveCancellationRefund is Reserve for the booking service port
// (booking.CancelRefunds): the reserved amount and the request ids.
func (s *Service) ReserveCancellationRefund(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, amount money.Cents, reason string, createdBy *string) (money.Cents, []string, error) {
	res, err := s.Reserve(ctx, tx, es, ReserveInput{Booking: b, Amount: amount, Reason: reason, CreatedBy: createdBy})
	if err != nil {
		return 0, nil, err
	}
	ids := make([]string, len(res.Requests))
	for i := range res.Requests {
		ids[i] = res.Requests[i].ID
	}
	return res.Amount, ids, nil
}
