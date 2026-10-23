package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	stripego "github.com/stripe/stripe-go/v87"

	"github.com/coste360/arenile/pkg/ext"
)

// Stripe event types handled by ParseWebhook.
const (
	evCheckoutCompleted     = "checkout.session.completed"
	evCheckoutAsyncSuccess  = "checkout.session.async_payment_succeeded"
	evCheckoutAsyncFailed   = "checkout.session.async_payment_failed"
	evCheckoutExpired       = "checkout.session.expired"
	evPaymentIntentSuccess  = "payment_intent.succeeded"
	evPaymentIntentFailed   = "payment_intent.payment_failed"
	evChargeRefunded        = "charge.refunded"
	evRefundFailed          = "refund.failed"
	signatureHeader         = "Stripe-Signature"
	failureAsyncPaymentFail = "async_payment_failed"
)

// ParseWebhook implements ext.PaymentProvider.
//
// Authentication: the Stripe-Signature header (t=..., one or more v1=...)
// is verified by the official SDK (HMAC-SHA256 of "<t>.<raw body>" with
// the establishment's webhook secret, constant-time comparison, every v1
// value tried so secrets can be rotated); the timestamp is then checked
// against the injected clock within SignatureTolerance in both directions,
// which rejects replays of captured deliveries. Any failure is
// ext.ErrInvalidSignature, without telling which check failed.
//
// Mapping (ProviderRef = the object Arenile created: Checkout Session,
// PaymentIntent or Refund):
//
//	checkout.session.completed (paid)          -> PaymentSucceeded
//	checkout.session.completed (unpaid, async) -> PaymentPending
//	checkout.session.async_payment_succeeded   -> PaymentSucceeded
//	checkout.session.async_payment_failed      -> PaymentFailed
//	checkout.session.expired                   -> PaymentFailed, outcome expired
//	payment_intent.succeeded                   -> PaymentSucceeded (ProviderRef pi_)
//	payment_intent.payment_failed              -> PaymentFailed, outcome attempt_failed
//	charge.refunded                            -> PaymentRefunded, outcome refund_total
//	                                              (ProviderRef pi_, Amount cumulative)
//	refund.failed                              -> PaymentFailed, outcome refund_failed
//	anything else                              -> ext.ErrIgnoredEvent
func (d *Driver) ParseWebhook(ctx context.Context, r *http.Request, creds ext.Credentials) (*ext.PaymentEvent, error) {
	secret := strings.TrimSpace(creds[CredWebhookSecret])
	if secret == "" {
		return nil, fmt.Errorf("%w: %w", ext.ErrInvalidSignature, ErrMissingCredentials)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxWebhookBody+1))
	if err != nil || len(body) > MaxWebhookBody {
		return nil, fmt.Errorf("%w: unreadable body", ext.ErrInvalidSignature)
	}
	header := r.Header.Get(signatureHeader)
	if err := d.verify(body, header, secret); err != nil {
		return nil, err
	}
	ev, err := stripego.ConstructEventWithoutVerification(body)
	if err != nil || ev.ID == "" || ev.Data == nil {
		// Authentic but not an event we can read (thin event, garbage).
		return nil, ext.ErrIgnoredEvent
	}
	return mapEvent(ev)
}

// verify checks the signature (SDK) and the timestamp (injected clock).
func (d *Driver) verify(body []byte, header, secret string) error {
	if err := stripego.ValidatePayload(body, header, secret, stripego.WithIgnoreTolerance()); err != nil {
		return ext.ErrInvalidSignature
	}
	ts, ok := signatureTimestamp(header)
	if !ok {
		return ext.ErrInvalidSignature
	}
	now := d.o.Clock.Now()
	if ts.Before(now.Add(-SignatureTolerance)) || ts.After(now.Add(SignatureTolerance)) {
		return ext.ErrInvalidSignature
	}
	return nil
}

// signatureTimestamp extracts t= from a Stripe-Signature header.
func signatureTimestamp(header string) (time.Time, bool) {
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && k == "t" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return time.Time{}, false
			}
			return time.Unix(n, 0), true
		}
	}
	return time.Time{}, false
}

// mapEvent normalises a verified Stripe event.
func mapEvent(ev stripego.Event) (*ext.PaymentEvent, error) {
	out := &ext.PaymentEvent{
		ProviderEventID: ev.ID,
		OccurredAt:      time.Unix(ev.Created, 0).UTC(),
		Metadata:        map[string]string{MetaStripeType: string(ev.Type)},
	}
	switch string(ev.Type) {
	case evCheckoutCompleted, evCheckoutAsyncSuccess, evCheckoutAsyncFailed, evCheckoutExpired:
		var cs stripego.CheckoutSession
		if err := json.Unmarshal(ev.Data.Raw, &cs); err != nil || cs.ID == "" {
			return nil, ext.ErrIgnoredEvent
		}
		fromSession(out, &cs)
		switch string(ev.Type) {
		case evCheckoutCompleted:
			out.Type = ext.PaymentPending
			if sessionPaid(&cs) {
				out.Type = ext.PaymentSucceeded
			}
		case evCheckoutAsyncSuccess:
			out.Type = ext.PaymentSucceeded
		case evCheckoutAsyncFailed:
			out.Type, out.FailureReason = ext.PaymentFailed, failureAsyncPaymentFail
		case evCheckoutExpired:
			out.Type, out.FailureReason = ext.PaymentFailed, OutcomeExpired
			out.Metadata[MetaOutcome] = OutcomeExpired
		}
	case evPaymentIntentSuccess, evPaymentIntentFailed:
		var pi stripego.PaymentIntent
		if err := json.Unmarshal(ev.Data.Raw, &pi); err != nil || pi.ID == "" {
			return nil, ext.ErrIgnoredEvent
		}
		fromPaymentIntent(out, &pi)
		if string(ev.Type) == evPaymentIntentSuccess {
			out.Type = ext.PaymentSucceeded
		} else {
			out.Type = ext.PaymentFailed
			out.Metadata[MetaOutcome] = OutcomeAttemptFailed
			out.FailureReason = "payment_failed"
			if pi.LastPaymentError != nil && pi.LastPaymentError.Code != "" {
				out.FailureReason = string(pi.LastPaymentError.Code)
			}
		}
	case evChargeRefunded:
		var ch stripego.Charge
		if err := json.Unmarshal(ev.Data.Raw, &ch); err != nil || ch.PaymentIntent == nil || ch.PaymentIntent.ID == "" {
			return nil, ext.ErrIgnoredEvent
		}
		out.Type = ext.PaymentRefunded
		out.ProviderRef = ch.PaymentIntent.ID
		out.Amount = ch.AmountRefunded
		out.Currency = strings.ToUpper(string(ch.Currency))
		copyIDs(out, ch.Metadata)
		out.Metadata[MetaOutcome] = OutcomeRefundTotal
		out.Metadata[MetaPaymentIntent] = ch.PaymentIntent.ID
		out.Metadata[MetaAmountCaptured] = strconv.FormatInt(ch.AmountCaptured, 10)
	case evRefundFailed:
		var rf stripego.Refund
		if err := json.Unmarshal(ev.Data.Raw, &rf); err != nil || rf.ID == "" {
			return nil, ext.ErrIgnoredEvent
		}
		fromRefund(out, &rf)
		out.Type = ext.PaymentFailed
		out.Metadata[MetaOutcome] = OutcomeRefundFailed
		out.FailureReason = "refund_failed"
		if rf.FailureReason != "" {
			out.FailureReason = string(rf.FailureReason)
		}
	default:
		return nil, ext.ErrIgnoredEvent
	}
	return out, nil
}

func sessionPaid(cs *stripego.CheckoutSession) bool {
	return cs.PaymentStatus == stripego.CheckoutSessionPaymentStatusPaid ||
		cs.PaymentStatus == stripego.CheckoutSessionPaymentStatusNoPaymentRequired
}

func fromSession(out *ext.PaymentEvent, cs *stripego.CheckoutSession) {
	out.ProviderRef = cs.ID
	out.Amount = cs.AmountTotal
	out.Currency = strings.ToUpper(string(cs.Currency))
	copyIDs(out, cs.Metadata)
	if out.BookingID == "" {
		out.BookingID = cs.ClientReferenceID
	}
	if cs.PaymentIntent != nil && cs.PaymentIntent.ID != "" {
		out.Metadata[MetaPaymentIntent] = cs.PaymentIntent.ID
	}
}

func fromPaymentIntent(out *ext.PaymentEvent, pi *stripego.PaymentIntent) {
	out.ProviderRef = pi.ID
	out.Amount = pi.AmountReceived
	if out.Amount == 0 {
		out.Amount = pi.Amount
	}
	out.Currency = strings.ToUpper(string(pi.Currency))
	copyIDs(out, pi.Metadata)
	out.Metadata[MetaPaymentIntent] = pi.ID
}

func fromRefund(out *ext.PaymentEvent, rf *stripego.Refund) {
	out.ProviderRef = rf.ID
	out.Amount = rf.Amount
	out.Currency = strings.ToUpper(string(rf.Currency))
	copyIDs(out, rf.Metadata)
	if rf.PaymentIntent != nil && rf.PaymentIntent.ID != "" {
		out.Metadata[MetaPaymentIntent] = rf.PaymentIntent.ID
	}
}

// copyIDs copies the non-personal Arenile metadata of a Stripe object.
func copyIDs(out *ext.PaymentEvent, md map[string]string) {
	for k, v := range md {
		switch k {
		case "establishment_id":
			out.EstablishmentID = v
		case "booking_id":
			out.BookingID = v
		}
		if _, taken := out.Metadata[k]; !taken {
			out.Metadata[k] = v
		}
	}
}
