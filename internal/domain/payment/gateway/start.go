package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Start creates a provider payment for a booking of es and records the
// intent row with payment.started in tx. The checkout service calls it
// AFTER the transaction that moved the booking to pending_payment has
// committed (the provider call is network I/O and must not hold database
// locks), so the two writes are separate units of work: a crash or a
// provider error in between leaves a pending_payment booking with no
// intent, which the page retries (a new attempt) and the hold sweeper
// expires like any unpaid booking.
//
//  1. the active provider of es must take online payments;
//  2. open intents of earlier attempts are closed at the provider
//     (PaymentCanceller, best effort) and marked cancelled, so the customer
//     cannot pay two sessions; a payment that still slips through is
//     refunded automatically as an overpayment (apply);
//  3. attempt n = previous max + 1, idempotency key "<booking_id>:<n>"
//     (spec 13.2): a retry after a crash returns the same provider session.
//     If the caller's transaction rolled back after the provider call, the
//     retry reuses the key with new parameters (a later expiry): the
//     provider refuses it and the gateway retries once with a fresh key;
//  4. the session expiry is aligned with the booking hold (hint
//     MetaExpiresAt; Stripe needs at least 30 minutes, ADR 0014).
func (s *Service) Start(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, req StartRequest) (*StartResult, error) {
	if req.Amount <= 0 || req.Amount > payment.MaxAmount {
		return nil, configerr.Invalid("amount", "L'importo da pagare deve essere positivo.")
	}
	switch req.Purpose {
	case PurposeFull, PurposeDeposit, PurposeBalance:
	case "":
		req.Purpose = PurposeFull
	default:
		return nil, configerr.Invalid("purpose", "Valori ammessi: full, deposit, balance.")
	}
	cfg, err := s.d.Settings.ActiveProvider(ctx, tx, es)
	if err != nil {
		return nil, fmt.Errorf("gateway: active provider: %w", err)
	}
	if cfg == nil || !cfg.Online {
		return nil, ErrProviderNotConfigured
	}
	pp, ok := s.d.Providers.PaymentProvider(cfg.Code)
	if !ok || !pp.Capabilities().Online {
		return nil, ErrProviderNotConfigured
	}
	b, err := store.GetBooking(ctx, tx, es, req.BookingID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, payment.ErrBookingNotFound
		}
		return nil, fmt.Errorf("gateway: %w", err)
	}
	s.cancelOpen(ctx, tx, es, cfg.Credentials, pp, b.ID)

	attempt, err := store.MaxPaymentAttempt(ctx, tx, es, b.ID)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	attempt++
	currency := strings.ToUpper(req.Currency)
	if currency == "" {
		currency = "EUR"
	}
	id := store.NewID()
	meta := map[string]string{
		MetaIntentID: id, MetaPurpose: string(req.Purpose), MetaLocale: req.Locale,
	}
	if b.HoldExpiresAt != nil {
		meta[MetaExpiresAt] = b.HoldExpiresAt.UTC().Format(time.RFC3339)
	}
	mode := ext.PaymentModeRedirect
	if cfg.Embedded && pp.Capabilities().EmbeddedMode {
		mode = ext.PaymentModeEmbedded
	}
	cpr := ext.CreatePaymentRequest{
		EstablishmentID: es.ID(), BookingID: b.ID, BookingCode: b.Code, Amount: int64(req.Amount), Currency: currency,
		Description: req.Description, CustomerEmail: req.CustomerEmail,
		ReturnURL: s.orDefault(req.ReturnURL), CancelURL: s.orDefault(req.CancelURL),
		PreferredMode: mode, IdempotencyKey: b.ID + ":" + strconv.Itoa(attempt), Metadata: meta, Credentials: cfg.Credentials,
	}
	res, err := pp.CreatePayment(ctx, cpr)
	if err != nil && isIdempotencyConflict(err) {
		cpr.IdempotencyKey += ":" + strconv.FormatInt(s.now().UnixNano(), 36)
		res, err = pp.CreatePayment(ctx, cpr)
	}
	if err != nil {
		return nil, providerError("create payment", err)
	}
	now := s.now()
	pi := &store.PaymentIntent{
		ID: id, BookingID: &b.ID, Provider: pp.Code(), Mode: cfg.Mode, Kind: store.IntentKindPayment,
		ExternalID: res.ProviderRef, Status: store.IntentPending, Amount: req.Amount, Currency: currency,
		Attempt: attempt, IdempotencyKey: &cpr.IdempotencyKey, RawEventIDs: []string{}, ExpiresAt: res.ExpiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
	if pi.Mode == "" {
		pi.Mode = store.ProviderModeTest
	}
	if err := store.InsertPaymentIntent(ctx, tx, es, pi); err != nil {
		if errors.Is(err, store.ErrConflict) {
			// Same provider session already recorded (idempotent replay).
			if prev, perr := store.FindPaymentIntentByRef(ctx, tx, es, pp.Code(), res.ProviderRef); perr == nil {
				return s.startResult(prev, cfg.PublishableKey, res), nil
			}
		}
		return nil, fmt.Errorf("gateway: %w", err)
	}
	diff := map[string]any{"attempt": attempt, "purpose": string(req.Purpose), "mode": pi.Mode}
	if err := s.record(ctx, tx, es, audit.SystemActor, "payment.started", pi, diff, event.PaymentPending, change(pi, "", store.PaymentDirectionIn, store.PaymentPending)); err != nil {
		return nil, err
	}
	return s.startResult(pi, cfg.PublishableKey, res), nil
}

func (s *Service) startResult(pi *store.PaymentIntent, publishable string, res *ext.CreatePaymentResult) *StartResult {
	out := &StartResult{
		Provider: pi.Provider, Mode: res.Mode, RedirectURL: res.RedirectURL, ClientSecret: res.ClientSecret,
		IntentID: pi.ID, Amount: pi.Amount, ExpiresAt: pi.ExpiresAt,
	}
	if res.Mode == ext.PaymentModeEmbedded {
		out.PublishableKey = publishable
	}
	return out
}

func (s *Service) orDefault(u string) string {
	if u != "" {
		return u
	}
	return s.d.BaseURL
}

// cancelOpen closes the open intents of earlier attempts of a booking. A
// provider failure is logged and the intent left pending: reconciliation
// settles it and a double payment is refunded as an overpayment.
func (s *Service) cancelOpen(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, creds ext.Credentials, pp ext.PaymentProvider, bookingID string) {
	open, err := store.ListOpenPaymentIntents(ctx, tx, es, bookingID)
	if err != nil {
		s.d.Log.WarnContext(ctx, "gateway: list open intents", "establishment_id", es.ID(), "booking_id", bookingID, "err", err)
		return
	}
	canceller, _ := pp.(PaymentCanceller)
	for i := range open {
		pi := &open[i]
		if pi.Provider != pp.Code() || canceller == nil {
			continue
		}
		if err := canceller.CancelPayment(ctx, pi.ExternalID, creds); err != nil {
			s.d.Log.InfoContext(ctx, "gateway: previous payment attempt left open", "establishment_id", es.ID(), "intent_id", pi.ID, "err", err)
			continue
		}
		pi.Status, pi.UpdatedAt = store.IntentCancelled, s.now()
		if err := store.UpdatePaymentIntent(ctx, tx, es, pi, "status"); err != nil {
			s.d.Log.WarnContext(ctx, "gateway: mark attempt cancelled", "intent_id", pi.ID, "err", err)
		}
	}
}

// LatestForBooking returns the most recent payment intent of a booking
// (store.ErrNotFound when none): status polling of the public page.
func (s *Service) LatestForBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*store.PaymentIntent, error) {
	pi, err := store.LatestPaymentIntentForBooking(ctx, db, es, bookingID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("gateway: %w", err)
	}
	return pi, nil
}
