package stripe

import (
	"context"
	"fmt"
	"strings"
	"time"

	stripego "github.com/stripe/stripe-go/v87"

	"github.com/coste360/arenile/pkg/ext"
)

// CreatePayment implements ext.PaymentProvider.
//
// Redirect (default): a Checkout Session in payment mode with one line item
// (amount in cents, currency, description), customer_email, success and
// cancel URLs, client_reference_id = booking id, metadata on the session
// and on its PaymentIntent, locale and expires_at from the request hints
// (clamped to Stripe's 31 min..24 h window) and the request idempotency key.
//
// Embedded (PreferredMode embedded): a PaymentIntent with automatic payment
// methods; the client confirms it with the Payment Element and the
// ClientSecret.
func (d *Driver) CreatePayment(ctx context.Context, req ext.CreatePaymentRequest) (*ext.CreatePaymentResult, error) {
	c, err := d.client(req.Credentials)
	if err != nil {
		return nil, err
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("stripe: amount must be positive")
	}
	currency := strings.ToLower(req.Currency)
	if currency == "" {
		currency = "eur"
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = "Prenotazione " + req.BookingCode
	}
	meta := forwardMetadata(req)

	if req.PreferredMode == ext.PaymentModeEmbedded {
		p := &stripego.PaymentIntentCreateParams{
			Amount:                  stripego.Int64(req.Amount),
			Currency:                stripego.String(currency),
			Description:             stripego.String(desc),
			Metadata:                meta,
			AutomaticPaymentMethods: &stripego.PaymentIntentCreateAutomaticPaymentMethodsParams{Enabled: stripego.Bool(true)},
		}
		if req.CustomerEmail != "" {
			p.ReceiptEmail = stripego.String(req.CustomerEmail)
		}
		if req.IdempotencyKey != "" {
			p.SetIdempotencyKey(req.IdempotencyKey)
		}
		pi, err := c.V1PaymentIntents.Create(ctx, p)
		if err != nil {
			return nil, mapErr("create payment intent", err)
		}
		return &ext.CreatePaymentResult{ProviderRef: pi.ID, Mode: ext.PaymentModeEmbedded, ClientSecret: pi.ClientSecret}, nil
	}

	now := d.o.Clock.Now()
	expires := sessionExpiry(now, req.Metadata[HintExpiresAt])
	p := &stripego.CheckoutSessionCreateParams{
		Mode: stripego.String(string(stripego.CheckoutSessionModePayment)),
		LineItems: []*stripego.CheckoutSessionCreateLineItemParams{{
			Quantity: stripego.Int64(1),
			PriceData: &stripego.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:    stripego.String(currency),
				UnitAmount:  stripego.Int64(req.Amount),
				ProductData: &stripego.CheckoutSessionCreateLineItemPriceDataProductDataParams{Name: stripego.String(desc)},
			},
		}},
		SuccessURL:        stripego.String(req.ReturnURL),
		CancelURL:         stripego.String(req.CancelURL),
		ClientReferenceID: stripego.String(req.BookingID),
		Locale:            stripego.String(stripeLocale(req.Metadata[HintLocale])),
		ExpiresAt:         stripego.Int64(expires.Unix()),
		Metadata:          meta,
		PaymentIntentData: &stripego.CheckoutSessionCreatePaymentIntentDataParams{Metadata: meta, Description: stripego.String(desc)},
	}
	if req.CustomerEmail != "" {
		p.CustomerEmail = stripego.String(req.CustomerEmail)
	}
	if req.IdempotencyKey != "" {
		p.SetIdempotencyKey(req.IdempotencyKey)
	}
	cs, err := c.V1CheckoutSessions.Create(ctx, p)
	if err != nil {
		return nil, mapErr("create checkout session", err)
	}
	res := &ext.CreatePaymentResult{ProviderRef: cs.ID, Mode: ext.PaymentModeRedirect, RedirectURL: cs.URL}
	if cs.ExpiresAt > 0 {
		t := time.Unix(cs.ExpiresAt, 0).UTC()
		res.ExpiresAt = &t
	}
	return res, nil
}

// CancelPayment closes a provider payment that is no longer wanted (a new
// attempt replaces it): a Checkout Session is expired, a PaymentIntent
// cancelled. A payment that is no longer open (already paid, expired or
// cancelled) is refused with an *APIError: the caller keeps its record as
// is and lets the webhook or the reconciliation report the real outcome.
// Implements gateway.PaymentCanceller.
func (d *Driver) CancelPayment(ctx context.Context, providerRef string, creds ext.Credentials) error {
	c, err := d.client(creds)
	if err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(providerRef, "cs_"):
		_, err = c.V1CheckoutSessions.Expire(ctx, providerRef, nil)
	case strings.HasPrefix(providerRef, "pi_"):
		_, err = c.V1PaymentIntents.Cancel(ctx, providerRef, nil)
	default:
		return fmt.Errorf("stripe: cancel: unknown reference kind")
	}
	return mapErr("cancel payment", err)
}

// forwardMetadata returns the metadata sent to Stripe: the request
// metadata without the driver hints, plus the ids of spec 13.2.
func forwardMetadata(req ext.CreatePaymentRequest) map[string]string {
	meta := make(map[string]string, len(req.Metadata)+3)
	for k, v := range req.Metadata {
		if k == HintLocale || k == HintExpiresAt {
			continue
		}
		meta[k] = v
	}
	meta["establishment_id"] = req.EstablishmentID
	meta["booking_id"] = req.BookingID
	if req.BookingCode != "" {
		meta["booking_code"] = req.BookingCode
	}
	return meta
}

// sessionExpiry aligns the session with the booking hold (hint RFC 3339)
// within the window accepted by Stripe.
func sessionExpiry(now time.Time, hint string) time.Time {
	lo, hi := now.Add(minSessionLifetime), now.Add(maxSessionLifetime)
	t, err := time.Parse(time.RFC3339, hint)
	if err != nil || t.Before(lo) {
		return lo
	}
	if t.After(hi) {
		return hi
	}
	return t
}
