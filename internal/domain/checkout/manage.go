package checkout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Manage area of the customer through the magic link (spec 12.4). The
// link is resolved by notify (hash lookup, expiry end of booking + 30
// days, revocation); an unknown, expired or revoked token is a plain 404.

// ErrLinkNotFound: unknown, expired or revoked magic link (404, never
// distinguishing the cases).
var ErrLinkNotFound = fmt.Errorf("%w: link", configerr.ErrNotFound)

// link authenticates a magic link token in the establishment of slug.
func (s *Service) link(ctx context.Context, slug, token string) (*online.PublicEstablishment, *store.MagicLinkToken, *booking.Detail, error) {
	if !ValidToken(token) {
		return nil, nil, nil, ErrLinkNotFound
	}
	pe, err := s.establishment(ctx, slug, false)
	if err != nil {
		if errors.Is(err, configerr.ErrNotFound) {
			return nil, nil, nil, ErrLinkNotFound
		}
		return nil, nil, nil, err
	}
	l, err := s.d.Notifier.ResolveMagicLink(ctx, s.d.DB, pe.Scope, token)
	if err != nil {
		if errors.Is(err, configerr.ErrNotFound) || errors.Is(err, store.ErrNotFound) {
			return nil, nil, nil, ErrLinkNotFound
		}
		return nil, nil, nil, err
	}
	if l.EstablishmentID != pe.Scope.ID() || !l.ExpiresAt.After(s.now()) || l.RevokedAt != nil {
		return nil, nil, nil, ErrLinkNotFound
	}
	d, err := s.d.Bookings.DetailIn(ctx, s.d.DB, pe.Scope, l.BookingID)
	if err != nil {
		if errors.Is(err, configerr.ErrNotFound) {
			return nil, nil, nil, ErrLinkNotFound
		}
		return nil, nil, nil, err
	}
	return pe, l, d, nil
}

// Manage returns the booking of a magic link with the actions the policy
// snapshot allows now and, when cancellable, the refund preview.
func (s *Service) Manage(ctx context.Context, slug, token string) (*Managed, error) {
	pe, l, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	return s.managed(ctx, pe, l, d)
}

func (s *Service) managed(ctx context.Context, pe *online.PublicEstablishment, l *store.MagicLinkToken, d *booking.Detail) (*Managed, error) {
	b := &d.Booking
	out := &Managed{Booking: d, Link: *l, Allowed: manageActions(b, rulesOf(b), s.today(pe.Establishment), onlinePayment(pe, b.Test) == nil), Refund: ManageRefundOf(d.RefundRequests)}
	var err error
	if out.Quote, out.Labels, err = s.summarize(ctx, s.d.DB, pe, d); err != nil {
		return nil, err
	}
	if out.Allowed.Cancel {
		res, err := s.d.Bookings.CancelAsCustomer(ctx, actor(ctx, l.ID), pe.Scope, b.ID, booking.CancelInput{DryRun: true})
		if err == nil {
			rf := res.Refund
			out.CancelPreview = &rf
		}
	}
	if b.IsSeasonPass {
		rels, err := store.ListSeasonReleases(ctx, s.d.DB, pe.Scope, b.ID)
		if err != nil {
			return nil, fmt.Errorf("checkout: releases: %w", err)
		}
		for _, r := range rels {
			if r.RevokedAt == nil {
				out.Releases = append(out.Releases, r)
			}
		}
	}
	switch booking.Status(b.Status) {
	case booking.StatusConfirmed, booking.StatusCheckedIn, booking.StatusCompleted:
		out.QRPayload = notify.QRPayload(b.Code)
		if body, _, err := s.d.Notifier.QR(out.QRPayload, notify.QRSVG); err == nil {
			out.QRSVG = string(body)
		}
	}
	return out, nil
}

// ManageDocument returns the QR or .ics of the booking of a magic link
// (the .ics carries the manage link itself).
func (s *Service) ManageDocument(ctx context.Context, slug, token, kind string) (*Document, error) {
	pe, _, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	return s.document(ctx, pe, d, kind, s.manageURL(pe.Establishment.Slug, token))
}

func (s *Service) manageURL(slug, token string) string { return s.pageURL(slug) + "/m/" + token }

// manageReturnURL is where the provider sends the customer back after a
// balance payment: the manage area WITHOUT the magic link token (the page
// keeps the token in its own storage and resumes from there), so the token
// never reaches the provider's session objects, dashboard or logs.
func (s *Service) manageReturnURL(slug, bookingID string) string {
	return s.pageURL(slug) + "/m?paid=1&booking=" + bookingID
}

// onlinePayment reports whether a booking with the given test flag can pay
// online now (nil) or why not. pe describes the bookings the page creates
// now (test flag = the test-mode switch); an existing booking keeps its own
// flag (spec 15.4): with test keys while test mode is off a real booking is
// refused (ErrProviderTestMode, INV-88) but a test booking created earlier
// still pays with the test keys (gateway.Start decides by the booking too).
func onlinePayment(pe *online.PublicEstablishment, test bool) error {
	switch {
	case pe.LivePaymentsBlocked && !test:
		return gateway.ErrProviderTestMode
	case pe.LivePaymentsBlocked:
		return nil
	case !pe.Online:
		return ErrOnlinePaymentUnavailable
	}
	return nil
}

// ManagePay starts the payment of the balance with the online provider.
func (s *Service) ManagePay(ctx context.Context, slug, token, idempotencyKey string) (*PayOutcome, error) {
	pe, l, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	b := &d.Booking
	st := booking.Status(b.Status)
	if st != booking.StatusConfirmed && st != booking.StatusCheckedIn {
		return nil, &booking.TransitionError{From: st, To: booking.StatusConfirmed}
	}
	balance := b.TotalAmount - b.PaidAmount
	if balance <= 0 {
		return nil, ErrNothingToPay
	}
	if err := onlinePayment(pe, b.Test); err != nil {
		return nil, err
	}
	locale := d.Customer.Locale
	if b.Locale != nil {
		locale = *b.Locale
	}
	s.d.Log.InfoContext(ctx, "balance payment requested", slog.String("booking_id", b.ID), slog.String("link_id", l.ID))
	return s.providerPayment(ctx, pe, d, gateway.PurposeBalance, balance, s.manageReturnURL(pe.Establishment.Slug, b.ID), locale)
}

// ManageModify previews or applies a modification within the policy:
// the selection is checked against the online catalog, the booking
// service re-prices and re-allocates atomically. A dry run reports
// violations and conflicts in the outcome instead of failing.
func (s *Service) ManageModify(ctx context.Context, slug, token string, expected *int64, in ModifyInput) (*ModifyOutcome, error) {
	pe, l, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	sel := in.Selection
	if sel.Formula == "" && !sel.StartsOn.IsZero() && !sel.EndsOn.IsZero() {
		sel.Formula = DefaultFormula(sel.StartsOn, sel.EndsOn, sel.Slots)
	}
	cat, err := s.loadCatalog(ctx, s.d.DB, pe.Scope, selectionSpotIDs(sel))
	if err != nil {
		return nil, err
	}
	if err := validateSelection(sel, cat, pe.Flow, s.today(pe.Establishment)); err != nil {
		return nil, err
	}
	loc := location(pe.Establishment)
	fee := policy.ModificationAllowed(d.Booking.StartsOn, s.now(), loc, rulesOf(&d.Booking)).Fee
	out := &ModifyOutcome{Fee: fee, Violations: []policy.Violation{}}
	slots, formula := sel.Slots, sel.Formula
	res, err := s.d.Bookings.ModifyAsCustomer(ctx, actor(ctx, l.ID), pe.Scope, d.Booking.ID, expected, booking.PatchInput{
		StartsOn: &sel.StartsOn, EndsOn: &sel.EndsOn, Slots: &slots, Formula: &formula,
		Lines: sel.Lines, Extras: sel.Extras, ExtrasSet: true, DryRun: in.DryRun,
	})
	var pve *policy.ViolationError
	switch {
	case in.DryRun && errors.As(err, &pve):
		out.Violations = append(out.Violations, pve.Violations...)
		out.Result = &booking.ChangeResult{Detail: d, PreviousTotal: d.Booking.TotalAmount, NewTotal: d.Booking.TotalAmount, Conflicts: []availability.Conflict{}}
	case err != nil:
		return nil, err
	default:
		out.Result = res
	}
	if out.Result.Applied {
		fresh, err := s.d.Bookings.DetailIn(ctx, s.d.DB, pe.Scope, d.Booking.ID)
		if err != nil {
			return nil, err
		}
		if out.Managed, err = s.managed(ctx, pe, l, fresh); err != nil {
			return nil, err
		}
	}
	if out.Result.Detail != nil {
		if out.Quote, err = s.quoteOf(ctx, s.d.DB, pe, out.Result.Detail); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ManageCancel previews or applies a cancellation with refund or credit.
// An applied money refund of an online payment is executed through the
// payment gateway (the provider confirms it later, webhook or
// reconciliation); payments taken at the desk leave a manual refund due.
func (s *Service) ManageCancel(ctx context.Context, slug, token string, expected *int64, in CancelInput) (*CancelOutcome, error) {
	pe, l, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	settle := booking.SettlePolicy
	switch in.Settlement {
	case "", string(booking.SettlePolicy), string(booking.SettleRefund):
	case string(booking.SettleCredit):
		settle = booking.SettleCredit
	default:
		return nil, configerr.Invalid("settlement", "Usa refund o credit.")
	}
	res, err := s.d.Bookings.CancelAsCustomer(ctx, actor(ctx, l.ID), pe.Scope, d.Booking.ID, booking.CancelInput{
		ExpectedVersion: expected, Reason: in.Reason, Settlement: settle, DryRun: in.DryRun,
	})
	if err != nil {
		return nil, err
	}
	out := &CancelOutcome{Result: res, RefundStatus: RefundNone}
	rf := res.Refund
	switch {
	case rf.Amount <= 0 || rf.As == booking.SettleNone:
	case rf.As == booking.SettleCredit:
		out.RefundStatus = RefundCredited
	case !res.Applied:
		out.RefundStatus = RefundManualDue
		if providerPaid(d.Payments) > 0 {
			out.RefundStatus = RefundProviderPending
		}
	default:
		// M5B: the credit-paid part was already given back as credit
		// (booking service, res.Recorded). ADR 0019: the rest was reserved
		// as guaranteed provider refunds in the cancellation transaction
		// (res.RefundRequestIDs); what no provider payment can take stays
		// a manual refund due.
		due := rf.Amount
		if res.Recorded != nil && res.Recorded.Method == store.PaymentMethodCredit {
			due -= res.Recorded.Amount
		}
		switch {
		case due <= 0:
			out.RefundStatus = RefundCredited
		case res.RefundDue > 0 || len(res.RefundRequestIDs) == 0:
			out.RefundStatus = RefundManualDue
		default:
			out.RefundStatus = s.sendRefunds(ctx, pe.Scope, res.RefundRequestIDs)
		}
	}
	if res.Applied {
		fresh, err := s.d.Bookings.DetailIn(ctx, s.d.DB, pe.Scope, d.Booking.ID)
		if err != nil {
			return nil, err
		}
		if out.Managed, err = s.managed(ctx, pe, l, fresh); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// providerPayment reports whether a ledger row was paid through an online
// provider (refundable by the gateway).
func isProviderPayment(p store.Payment) bool {
	return p.Direction == store.PaymentDirectionIn && p.Status == "succeeded" && p.Provider != "" && p.Provider != online.ProviderManual
}

func providerPaid(ps []store.Payment) money.Cents {
	var sum money.Cents
	for _, p := range ps {
		if isProviderPayment(p) {
			sum += p.Amount
		}
	}
	return sum
}

// sendRefunds sends the reserved refund requests at once (when the
// sender is wired) and reports provider_succeeded when the provider
// already confirmed all of them, provider_pending otherwise (the job
// keeps sending until the provider confirms).
func (s *Service) sendRefunds(ctx context.Context, es store.EstablishmentScope, ids []string) string {
	if s.d.Refunds == nil {
		return RefundProviderPending
	}
	s.d.Refunds.SendNow(context.WithoutCancel(ctx), es, ids)
	for _, id := range ids {
		rr, err := store.GetRefundRequest(ctx, s.d.DB, es, id)
		if err != nil || rr.Status != store.RefundRequestSucceeded {
			return RefundProviderPending
		}
	}
	return RefundProviderSucceeded
}

// ManageRelease previews or applies the release of season pass days (one
// slot choice per request).
func (s *Service) ManageRelease(ctx context.Context, slug, token string, in ReleaseInput) (*ReleaseOutcome, error) {
	pe, l, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	if !d.Booking.IsSeasonPass || s.d.Passes == nil {
		return nil, pass.ErrCustomerReleaseNotAllowed
	}
	if len(in.Days) == 0 || len(in.Days) > 120 {
		return nil, configerr.Invalid("days", "Indica da 1 a 120 giorni.")
	}
	slots := in.Days[0].Slots
	dates := make([]calendar.Date, 0, len(in.Days))
	for i, day := range in.Days {
		if day.Slots != slots {
			return nil, configerr.Invalid(fmt.Sprintf("days[%d].slots", i), "Usa la stessa fascia per tutti i giorni della richiesta.")
		}
		dates = append(dates, day.Date)
	}
	pin := pass.ReleaseInput{Dates: dates, Slots: slots}
	var rows []store.SeasonRelease
	if in.DryRun {
		rows, err = s.d.Passes.PreviewReleaseAsCustomer(ctx, actor(ctx, l.ID), pe.Scope, d.Booking.ID, pin)
	} else {
		rows, err = s.d.Passes.ReleaseAsCustomer(ctx, actor(ctx, l.ID), pe.Scope, d.Booking.ID, pin)
	}
	if err != nil {
		return nil, err
	}
	out := &ReleaseOutcome{Applied: !in.DryRun, Released: rows}
	if out.Released == nil {
		out.Released = []store.SeasonRelease{}
	}
	if out.Applied {
		fresh, err := s.d.Bookings.DetailIn(ctx, s.d.DB, pe.Scope, d.Booking.ID)
		if err != nil {
			return nil, err
		}
		if out.Managed, err = s.managed(ctx, pe, l, fresh); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RequestLink sends a new manage link when email and code match; it
// returns nil also when they do not (no enumeration). The work runs in the
// background so that the response time does not tell a match from a miss;
// the limits (per IP and per email+code) answer 429 before anything else.
// ip feeds the rate limiter (never logged with the email).
func (s *Service) RequestLink(ctx context.Context, slug, email, code, ip string) error {
	pe, err := s.establishment(ctx, slug, false)
	if err != nil {
		return err
	}
	email = customer.NormalizeEmail(email)
	code = strings.ToUpper(strings.TrimSpace(code))
	ve := &configerr.ValidationError{}
	if !customer.ValidEmail(email) {
		ve.Add("email", "Indirizzo email non valido.")
	}
	if code == "" || len(code) > 40 {
		ve.Add("code", "Codice prenotazione non valido.")
	}
	if err := ve.OrNil(); err != nil {
		return err
	}
	if err := allow(s.linkLimitIP, pe.Scope.ID()+"|"+ip); err != nil {
		return err
	}
	if err := allow(s.linkLimitBooking, pe.Scope.ID()+"|"+HashToken(email+"|"+code)); err != nil {
		return err
	}
	select {
	case s.linkSem <- struct{}{}:
	default:
		// Saturated: drop silently (the customer can retry; no enumeration).
		s.d.Log.WarnContext(ctx, "manage link request dropped: too many in flight", slog.String("establishment_id", pe.Scope.ID()))
		return nil
	}
	s.linkWG.Add(1)
	es := pe.Scope
	bg := context.WithoutCancel(ctx)
	go func() {
		defer s.linkWG.Done()
		defer func() { <-s.linkSem }()
		ctx, cancel := context.WithTimeout(bg, linkRequestTimeout)
		defer cancel()
		if err := s.d.Notifier.RequestLink(ctx, es, email, code); err != nil {
			s.d.Log.ErrorContext(ctx, "manage link request failed", slog.String("establishment_id", es.ID()), slog.String("error", err.Error()))
		}
	}()
	return nil
}
