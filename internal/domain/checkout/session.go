package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Checkout session use cases (spec 10.1 steps 3-7, 12.2, 12.3 step 4).

// supportedLocales of the public page (spec 20).
var supportedLocales = map[string]bool{"it": true, "en": true, "de": true, "fr": true}

func parseBreakdown(raw []byte) (pricing.PriceBreakdown, error) {
	var bd pricing.PriceBreakdown
	if len(raw) == 0 {
		return bd, nil
	}
	if err := json.Unmarshal(raw, &bd); err != nil {
		return bd, fmt.Errorf("checkout: price snapshot: %w", err)
	}
	return bd, nil
}

// Create creates the held booking and the checkout session in one
// transaction; the token is returned once (only its hash is stored).
func (s *Service) Create(ctx context.Context, slug string, in CreateInput) (*Session, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	es, est := pe.Scope, pe.Establishment
	if err := allow(s.createLimit, es.ID()); err != nil {
		return nil, err
	}
	sel := in.Selection
	if sel.Formula == "" && !sel.StartsOn.IsZero() && !sel.EndsOn.IsZero() {
		sel.Formula = DefaultFormula(sel.StartsOn, sel.EndsOn, sel.Slots)
	}
	cat, err := s.loadCatalog(ctx, s.d.DB, es, selectionSpotIDs(sel))
	if err != nil {
		return nil, err
	}
	if err := validateSelection(sel, cat, pe.Flow, s.today(est)); err != nil {
		return nil, err
	}
	promo := strings.TrimSpace(sel.PromoCode)
	if !pe.Flow.PromoCodesEnabled {
		promo = ""
	}
	locale := strings.ToLower(strings.TrimSpace(in.Locale))
	if !supportedLocales[locale] {
		locale = est.DefaultLocale
	}
	token, hash, err := NewToken()
	if err != nil {
		return nil, err
	}
	sessionID := store.NewID()
	var (
		d  *booking.Detail
		cs *store.CheckoutSession
	)
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		d, err = s.d.Bookings.CreateOnline(ctx, tx, actor(ctx, sessionID), es, booking.CreateInput{
			Channel: store.ChannelOnline, StartsOn: sel.StartsOn, EndsOn: sel.EndsOn, Slots: sel.Slots, Formula: sel.Formula,
			Lines: sel.Lines, Extras: sel.Extras, PromoCode: promo, HoldTTL: s.d.HoldTTL,
		})
		if err != nil {
			return err
		}
		now := s.now()
		cs = &store.CheckoutSession{
			ID: sessionID, BookingID: d.Booking.ID, TokenHash: hash, State: store.CheckoutOpen,
			ExpiresAt: *d.Booking.HoldExpiresAt, Locale: locale, CreatedAt: now, UpdatedAt: now,
		}
		if o := strings.TrimSpace(in.EmbedOrigin); o != "" && len(o) <= 255 {
			cs.EmbedOrigin = &o
		}
		return store.InsertCheckoutSession(ctx, tx, es, cs)
	})
	if err != nil {
		return nil, err
	}
	q, err := s.quoteOf(ctx, s.d.DB, pe, d)
	if err != nil {
		return nil, err
	}
	return &Session{BookingID: d.Booking.ID, Token: token, Status: d.Booking.Status, HoldExpiresAt: cs.ExpiresAt, Quote: q}, nil
}

// State returns the state of the session and its booking. It never
// confirms (INV-19) and never writes.
func (s *Service) State(ctx context.Context, slug, bookingID, token string) (*State, error) {
	pe, cs, err := s.session(ctx, slug, bookingID, token)
	if err != nil {
		return nil, err
	}
	return s.state(ctx, s.d.DB, pe, cs)
}

func (s *Service) state(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, cs *store.CheckoutSession) (*State, error) {
	es := pe.Scope
	d, err := s.d.Bookings.DetailIn(ctx, db, es, cs.BookingID)
	if err != nil {
		return nil, err
	}
	out := &State{Booking: d, Session: *cs}
	var pi *store.PaymentIntent
	if s.d.Payments != nil {
		got, err := s.d.Payments.LatestForBooking(ctx, db, es, cs.BookingID)
		switch {
		case err == nil:
			pi = got
		case errors.Is(err, store.ErrNotFound), errors.Is(err, configerr.ErrNotImplemented):
		default:
			return nil, err
		}
	}
	out.Payment = paymentState(pi)
	placeholder := customer.IsPlaceholder(&d.Customer)
	fs := deriveState(cs.State, &d.Booking, detailsComplete(&d.Booking, placeholder, pe.Terms), out.Payment, s.now())
	out.Session.State, out.NextAction = fs.Session, fs.Next
	if out.Quote, out.Labels, err = s.summarize(ctx, db, pe, d); err != nil {
		return nil, err
	}
	if fs.Next == NextDone {
		out.Confirmation = s.confirmation(d, placeholder)
	}
	return out, nil
}

// confirmation builds the confirmation block of a confirmed booking: code,
// QR payload (and inline SVG when the renderer is available), masked email
// of the customer receiving the manage link.
func (s *Service) confirmation(d *booking.Detail, placeholder bool) *Confirmation {
	c := &Confirmation{Code: d.Booking.Code, QRPayload: notify.QRPayload(d.Booking.Code)}
	if body, _, err := s.d.Notifier.QR(c.QRPayload, notify.QRSVG); err == nil {
		c.QRSVG = string(body)
	}
	if !placeholder && d.Customer.Email != nil && *d.Customer.Email != "" {
		c.ManageLinkSent = true
		c.EmailMasked = notify.MaskEmail(*d.Customer.Email)
	}
	return c
}

// UpdateCustomer saves customer details, terms acceptance, consents,
// extras and promo code of the held booking (step 4).
func (s *Service) UpdateCustomer(ctx context.Context, slug, bookingID, token string, in CustomerInput) (*State, error) {
	pe, cs, err := s.session(ctx, slug, bookingID, token)
	if err != nil {
		return nil, err
	}
	es := pe.Scope
	if err := validateCustomer(in, pe.Flow); err != nil {
		return nil, err
	}
	terms, err := s.d.Settings.CurrentTerms(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	accepted := strings.TrimSpace(in.AcceptTermsVersion)
	if terms != nil && accepted != terms.Version {
		return nil, ErrTermsOutdated
	}
	locale := strings.ToLower(strings.TrimSpace(in.Locale))
	if locale == "" {
		locale = cs.Locale
	}
	if !supportedLocales[locale] {
		return nil, configerr.Invalid("locale", "Lingua non supportata (it, en, de, fr).")
	}
	ci := customer.Input{FirstName: in.FirstName, LastName: in.LastName, Email: in.Email, Phone: in.Phone, Locale: locale}
	if in.MarketingConsent {
		version := accepted
		if version == "" {
			version = "online"
		}
		ci.MarketingConsent = &store.MarketingConsent{GivenAt: s.now(), Source: customer.ConsentSourceOnline, TextVersion: version}
	}
	promo := in.PromoCode
	if !pe.Flow.PromoCodesEnabled {
		promo = nil
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if _, err := s.d.Bookings.AttachCustomer(ctx, tx, actor(ctx, cs.ID), es, cs.BookingID, booking.OnlineCustomerInput{
			Customer: ci, TermsVersion: accepted, Locale: locale, NotesCustomer: in.Notes,
			Extras: in.Extras, ExtrasSet: in.ExtrasSet, PromoCode: promo,
		}); err != nil {
			return err
		}
		cs.Locale, cs.UpdatedAt = locale, s.now()
		return store.UpdateCheckoutSession(ctx, tx, es, cs)
	})
	if err != nil {
		return nil, err
	}
	pe.Terms = terms
	return s.state(ctx, s.d.DB, pe, cs)
}

// validateCustomer checks step 4 beyond the customer rules: first and last
// name and email are required on the public page, the phone when the
// establishment asks for it (E.164 normalisation in the customer service).
func validateCustomer(in CustomerInput, flow online.Config) error {
	ve := &configerr.ValidationError{}
	if strings.TrimSpace(in.FirstName) == "" {
		ve.Add("first_name", "Il nome è obbligatorio.")
	}
	if strings.TrimSpace(in.LastName) == "" {
		ve.Add("last_name", "Il cognome è obbligatorio.")
	}
	if e := customer.NormalizeEmail(in.Email); e == "" || !customer.ValidEmail(e) {
		ve.Add("email", "Indirizzo email non valido.")
	}
	switch phone := strings.TrimSpace(in.Phone); {
	case phone == "" && flow.RequirePhone:
		ve.Add("phone", "Il telefono è obbligatorio.")
	case phone != "":
		if _, ok := customer.NormalizePhone(phone); !ok {
			ve.Add("phone", "Numero di telefono non valido: usa il formato internazionale, es. +39 333 1234567.")
		}
	}
	if len([]rune(in.Notes)) > 1000 {
		ve.Add("notes", "Al massimo 1000 caratteri.")
	}
	return ve.OrNil()
}

// planOf is the payment plan of a stored booking.
func (s *Service) planOf(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, d *booking.Detail) (string, money.Cents, error) {
	q, err := s.quoteOf(ctx, db, pe, d)
	if err != nil {
		return "", 0, err
	}
	return q.Plan, q.DueNow, nil
}

// Pay starts the provider payment or confirms without online payment
// (manual driver: payment mode none, pay-at-desk provider, approval).
// The browser return URL only resumes the page: confirmation comes from
// the webhook (INV-19).
func (s *Service) Pay(ctx context.Context, slug, bookingID, token string, in PayInput) (*PayOutcome, error) {
	pe, cs, err := s.session(ctx, slug, bookingID, token)
	if err != nil {
		return nil, err
	}
	es := pe.Scope
	if in.Settlement == "" {
		in.Settlement = SettleOnline
	}
	if in.Settlement != SettleOnline && in.Settlement != SettleAtDesk {
		return nil, configerr.Invalid("settlement", "Usa online o at_desk.")
	}
	d, err := s.d.Bookings.DetailIn(ctx, s.d.DB, es, cs.BookingID)
	if err != nil {
		return nil, err
	}
	st := booking.Status(d.Booking.Status)
	switch st {
	case booking.StatusHeld, booking.StatusPendingPayment:
	case booking.StatusExpired:
		return nil, availability.ErrHoldExpired
	default:
		return nil, &booking.TransitionError{From: st, To: booking.StatusPendingPayment}
	}
	terms, err := s.d.Settings.CurrentTerms(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	if customer.IsPlaceholder(&d.Customer) {
		return nil, ErrDetailsMissing
	}
	if !detailsComplete(&d.Booking, false, terms) {
		return nil, ErrTermsOutdated
	}
	// A first look at the plan decides the branch; startPayment computes
	// the amount again under the booking lock.
	plan, _, err := s.planOf(ctx, s.d.DB, pe, d)
	if err != nil {
		return nil, err
	}
	switch {
	case plan == PlanAtDesk || plan == PlanApproval:
		if st != booking.StatusHeld {
			return nil, &booking.TransitionError{From: st, To: booking.StatusConfirmed}
		}
		return s.confirmWithoutPayment(ctx, pe, cs)
	case in.Settlement == SettleAtDesk:
		return nil, ErrSettlementNotAllowed
	}
	returnURL := s.pageURL(pe.Establishment.Slug) + "?checkout=" + cs.BookingID
	if in.ReturnTo != "" {
		u, err := embedReturnURL(in.ReturnTo, pe.Establishment.AllowedEmbedOrigins, cs.BookingID)
		if err != nil {
			return nil, err
		}
		returnURL = u
	}
	return s.startPayment(ctx, pe, cs, returnURL)
}

// EmbedReturnParam is the query parameter of the host page that embed.js
// turns into ?checkout=<booking id> of the widget after a provider payment.
const EmbedReturnParam = "arenile_checkout"

// embedReturnURL validates the host page of an embedded widget against the
// allowed embed origins (no open redirect) and returns it with
// arenile_checkout=<bookingID>, without fragment.
func embedReturnURL(raw string, allowed []string, bookingID string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !u.IsAbs() || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", configerr.Invalid("return_to", "Pagina di ritorno non valida.")
	}
	origin, err := establishment.NormalizeEmbedOrigin(u.Scheme + "://" + u.Host)
	if err != nil || !slices.Contains(allowed, origin) {
		return "", configerr.Invalid("return_to", "La pagina di ritorno non è tra i siti autorizzati a ospitare il widget.")
	}
	u.Fragment, u.RawFragment = "", ""
	q := u.Query()
	q.Set(EmbedReturnParam, bookingID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Service) confirmWithoutPayment(ctx context.Context, pe *online.PublicEstablishment, cs *store.CheckoutSession) (*PayOutcome, error) {
	var d *booking.Detail
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		if d, err = s.d.Bookings.ConfirmWithoutPayment(ctx, tx, actor(ctx, cs.ID), pe.Scope, cs.BookingID); err != nil {
			return err
		}
		cs.State, cs.UpdatedAt = store.CheckoutCompleted, s.now()
		return store.UpdateCheckoutSession(ctx, tx, pe.Scope, cs)
	})
	if err != nil {
		return nil, err
	}
	return &PayOutcome{Mode: PayNone, Status: d.Booking.Status, HoldExpiresAt: d.Booking.HoldExpiresAt}, nil
}

// startPayment moves the booking to pending_payment and asks the provider
// for the amount due. The plan and the amount are computed again inside
// the transaction, on the locked booking (INV-22, INV-51): the total read
// by Pay may have changed in between (extras or promo code of a later
// PATCH, a staff edit), and the provider must be asked for the current
// amount, never a stale one that would leave the booking below its
// confirmation threshold with money on it. A booking that no longer needs
// an online payment is a conflict: the page reloads its state.
func (s *Service) startPayment(ctx context.Context, pe *online.PublicEstablishment, cs *store.CheckoutSession, returnURL string) (*PayOutcome, error) {
	es := pe.Scope
	if !pe.Online {
		return nil, ErrOnlinePaymentUnavailable
	}
	until := s.now().Add(MaxPaymentHold)
	if dl := s.holdDeadline(cs); until.After(dl) {
		until = dl
	}
	var (
		d    *booking.Detail
		plan string
		due  money.Cents
	)
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		if d, err = s.d.Bookings.StartPayment(ctx, tx, actor(ctx, cs.ID), es, cs.BookingID, until); err != nil {
			return err
		}
		if plan, due, err = s.planOf(ctx, tx, pe, d); err != nil {
			return err
		}
		if (plan != PlanOnlineFull && plan != PlanOnlineDeposit) || due <= 0 {
			return ErrBookingChanged
		}
		cs.State, cs.PaymentAttempts, cs.UpdatedAt = store.CheckoutPaying, cs.PaymentAttempts+1, s.now()
		cs.ExpiresAt = *d.Booking.HoldExpiresAt
		return store.UpdateCheckoutSession(ctx, tx, es, cs)
	})
	if err != nil {
		return nil, err
	}
	purpose := gateway.PurposeFull
	if plan == PlanOnlineDeposit {
		purpose = gateway.PurposeDeposit
	}
	return s.providerPayment(ctx, pe, d, purpose, due, returnURL, cs.Locale)
}

// providerPayment asks the payment gateway (outside any transaction: the
// provider call is network I/O) for the payment of amount.
func (s *Service) providerPayment(ctx context.Context, pe *online.PublicEstablishment, d *booking.Detail, purpose gateway.Purpose, amount money.Cents, returnURL, locale string) (*PayOutcome, error) {
	est := pe.Establishment
	sep := "?"
	if strings.Contains(returnURL, "?") {
		sep = "&"
	}
	req := gateway.StartRequest{
		BookingID: d.Booking.ID, BookingCode: d.Booking.Code, Purpose: purpose, Amount: amount,
		Currency: est.Currency, Description: est.Name + " · " + d.Booking.Code, Locale: locale,
		ReturnURL: returnURL, CancelURL: returnURL + sep + "cancelled=1",
	}
	if d.Customer.Email != nil && !customer.IsPlaceholder(&d.Customer) {
		req.CustomerEmail = *d.Customer.Email
	}
	res, err := s.d.Payments.Start(ctx, s.d.DB, pe.Scope, req)
	if err != nil {
		s.d.Log.WarnContext(ctx, "provider payment not started", slog.String("booking_id", d.Booking.ID), slog.String("error", err.Error()))
		return nil, err
	}
	out := &PayOutcome{
		Mode: PayRedirect, Status: string(booking.StatusPendingPayment), RedirectURL: res.RedirectURL, ClientSecret: res.ClientSecret,
		PublishableKey: res.PublishableKey, Amount: res.Amount, HoldExpiresAt: d.Booking.HoldExpiresAt,
	}
	if d.Booking.Status != "" {
		out.Status = d.Booking.Status
	}
	switch res.Mode {
	case ext.PaymentModeEmbedded:
		out.Mode = PayEmbedded
	case ext.PaymentModeNone:
		out.Mode = PayNone
	}
	return out, nil
}

// Abandon releases the hold of the session's booking (booking expired,
// session abandoned). Idempotent.
func (s *Service) Abandon(ctx context.Context, slug, bookingID, token string) error {
	pe, cs, err := s.session(ctx, slug, bookingID, token)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := s.d.Bookings.Abandon(ctx, tx, actor(ctx, cs.ID), pe.Scope, cs.BookingID); err != nil {
			return err
		}
		cs.State, cs.UpdatedAt = store.CheckoutAbandoned, s.now()
		return store.UpdateCheckoutSession(ctx, tx, pe.Scope, cs)
	})
}

// Document returns the QR (DocQRSVG / DocQRPNG) or the .ics (DocICS) of a
// confirmed booking of the session.
func (s *Service) Document(ctx context.Context, slug, bookingID, token, kind string) (*Document, error) {
	pe, cs, err := s.session(ctx, slug, bookingID, token)
	if err != nil {
		return nil, err
	}
	d, err := s.d.Bookings.DetailIn(ctx, s.d.DB, pe.Scope, cs.BookingID)
	if err != nil {
		return nil, err
	}
	return s.document(ctx, pe, d, kind, "")
}

func (s *Service) document(ctx context.Context, pe *online.PublicEstablishment, d *booking.Detail, kind, manageURL string) (*Document, error) {
	switch booking.Status(d.Booking.Status) {
	case booking.StatusConfirmed, booking.StatusCheckedIn, booking.StatusCompleted:
	default:
		return nil, ErrDocumentNotReady
	}
	name := "prenotazione-" + d.Booking.Code
	switch kind {
	case DocQRSVG, DocQRPNG:
		format := notify.QRSVG
		if kind == DocQRPNG {
			format = notify.QRPNG
		}
		body, ct, err := s.d.Notifier.QR(notify.QRPayload(d.Booking.Code), format)
		if err != nil {
			return nil, err
		}
		return &Document{ContentType: ct, Filename: name + "." + format, Body: body}, nil
	case DocICS:
		body, err := s.d.Notifier.ICS(ctx, s.d.DB, pe.Scope, d.Booking.ID, manageURL)
		if err != nil {
			return nil, err
		}
		return &Document{ContentType: "text/calendar; charset=utf-8", Filename: name + ".ics", Body: body}, nil
	}
	return nil, configerr.Invalid("format", "Formato non supportato.")
}

// rulesOf decodes the policy snapshot of a booking.
func rulesOf(b *store.Booking) policy.Rules {
	snap, err := policy.ParseSnapshot(b.PolicySnapshot)
	if err != nil {
		return policy.Rules{}
	}
	return snap.Rules
}
