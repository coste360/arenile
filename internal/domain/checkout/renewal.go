package checkout

import (
	"context"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Magic link operations used by the season pass renewals (spec 10.4, 12.4;
// M5B, caller internal/domain/renewal). The renewal proposal emails carry
// the ordinary manage link of the held booking of the proposal (notify
// issues it), so the link is authenticated exactly like the manage area:
// unknown, expired or revoked tokens are a plain 404.

// ManageRef is a resolved magic link: the establishment, the booking it
// grants access to and the link id (the customer actor of the audit log).
type ManageRef struct {
	Scope         store.EstablishmentScope
	Establishment *store.Establishment
	BookingID     string
	LinkID        string
	// Online reports whether the establishment takes online payments.
	Online bool
	// TermsVersion is the current terms version ("" when none is published).
	TermsVersion string
}

// ResolveManage authenticates a magic link token of the establishment of
// slug (ErrLinkNotFound otherwise).
func (s *Service) ResolveManage(ctx context.Context, slug, token string) (*ManageRef, error) {
	pe, l, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	ref := &ManageRef{
		Scope: pe.Scope, Establishment: pe.Establishment, BookingID: d.Booking.ID, LinkID: l.ID, Online: pe.Online,
	}
	if pe.Terms != nil {
		ref.TermsVersion = pe.Terms.Version
	} else if t, err := s.d.Settings.CurrentTerms(ctx, s.d.DB, pe.Scope); err == nil && t != nil {
		ref.TermsVersion = t.Version
	}
	return ref, nil
}

// PayManagedAmount starts the online payment of amount (the deposit or the
// whole balance of a renewal) for the held or pending_payment booking of
// a magic link: the booking moves to pending_payment through the booking
// service (its hold is never shortened, so the pre-emption deadline keeps
// protecting the spot), then the provider is asked for amount outside the
// transaction. The amount is checked again under the booking lock. The
// confirmation comes only from the provider webhook (INV-19); the browser
// returns to the manage area without the token.
func (s *Service) PayManagedAmount(ctx context.Context, slug, token string, amount money.Cents, _ string) (*PayOutcome, error) {
	pe, l, d, err := s.link(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	switch st := booking.Status(d.Booking.Status); st {
	case booking.StatusHeld, booking.StatusPendingPayment:
	case booking.StatusExpired:
		return nil, availability.ErrHoldExpired
	default:
		return nil, &booking.TransitionError{From: st, To: booking.StatusPendingPayment}
	}
	if amount <= 0 {
		return nil, ErrNothingToPay
	}
	if !pe.Online {
		return nil, ErrOnlinePaymentUnavailable
	}
	var due money.Cents
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		if d, err = s.d.Bookings.StartPayment(ctx, tx, actor(ctx, l.ID), pe.Scope, d.Booking.ID, s.now().Add(MaxPaymentHold)); err != nil {
			return err
		}
		due = d.Booking.TotalAmount - d.Booking.PaidAmount
		if amount > due {
			return configerr.Invalid("amount", "L'importo supera il saldo dovuto.")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	purpose := gateway.PurposeFull
	if amount < due {
		purpose = gateway.PurposeDeposit
	}
	locale := d.Customer.Locale
	if d.Booking.Locale != nil {
		locale = *d.Booking.Locale
	}
	s.d.Log.InfoContext(ctx, "renewal payment requested", slog.String("booking_id", d.Booking.ID), slog.String("link_id", l.ID))
	return s.providerPayment(ctx, pe, d, purpose, amount, s.manageReturnURL(pe.Establishment.Slug, d.Booking.ID), locale)
}
