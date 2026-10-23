package checkout

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Accepting a waitlist offer (M5B, spec 10.5): the offer hold becomes a
// held booking of the entry's customer with a checkout session, exactly
// like createPublicBooking; the customer continues on the public page
// (details, payment) and the booking is confirmed only by the payment
// webhook or the manual driver (INV-19).

// holdBookings is the booking capability used here (implemented by
// *booking.Service, internal/domain/booking/waitlist.go); asserted at run
// time so that the Bookings port and its test doubles stay unchanged.
type holdBookings interface {
	CreateFromHold(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, in booking.HoldBookingInput) (*booking.Detail, error)
}

// HoldSession are the inputs of CreateFromHold.
type HoldSession struct {
	HoldID     string
	CustomerID string
	PackID     string
	Locale     string
	// ActorID identifies the waitlist entry in the audit log.
	ActorID string
	// Within runs in the same transaction once the booking exists (the
	// waitlist marks its offer accepted and its entry converted there).
	Within func(ctx context.Context, tx bun.Tx, d *booking.Detail) error
}

// CreateFromHold converts an open hold into a held booking of an existing
// customer and opens its checkout session, in one transaction with
// in.Within. The session token is returned once (only its hash is stored).
func (s *Service) CreateFromHold(ctx context.Context, es store.EstablishmentScope, in HoldSession) (*Session, error) {
	hb, ok := s.d.Bookings.(holdBookings)
	if !ok {
		return nil, configerr.ErrNotImplemented
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("checkout: establishment: %w", err)
	}
	pe, err := s.establishment(ctx, est.Slug, false)
	if err != nil {
		return nil, err
	}
	locale := strings.ToLower(strings.TrimSpace(in.Locale))
	if !supportedLocales[locale] {
		locale = est.DefaultLocale
	}
	token, hash, err := NewToken()
	if err != nil {
		return nil, err
	}
	var (
		d  *booking.Detail
		cs *store.CheckoutSession
	)
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		h, err := store.GetHold(ctx, tx, es, in.HoldID)
		if err != nil {
			return fmt.Errorf("checkout: hold: %w", err)
		}
		d, err = hb.CreateFromHold(ctx, tx, actor(ctx, in.ActorID), es, booking.HoldBookingInput{
			HoldID: in.HoldID, CustomerID: in.CustomerID, PackID: in.PackID,
			Formula: DefaultFormula(h.StartsOn, h.EndsOn, h.Slots), HoldTTL: s.d.HoldTTL,
		})
		if err != nil {
			return err
		}
		now := s.now()
		cs = &store.CheckoutSession{
			ID: store.NewID(), BookingID: d.Booking.ID, TokenHash: hash, State: store.CheckoutOpen,
			ExpiresAt: *d.Booking.HoldExpiresAt, Locale: locale, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertCheckoutSession(ctx, tx, es, cs); err != nil {
			return err
		}
		if in.Within != nil {
			return in.Within(ctx, tx, d)
		}
		return nil
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
