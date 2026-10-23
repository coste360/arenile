package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository of the public checkout (M4, spec 10.1, 12.2): checkout
// sessions (token hash -> held booking) and the online columns of bookings
// that the generic booking update does not rewrite (accepted terms, locale).
// Owned by internal/domain/checkout and the public-channel methods of
// internal/domain/booking.

// InsertCheckoutSession inserts a session (ErrConflict on a token hash
// collision, which a 256-bit random token makes practically impossible).
func InsertCheckoutSession(ctx context.Context, db bun.IDB, es EstablishmentScope, s *CheckoutSession) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	s.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(s).Exec(ctx)
	return wrapWrite("insert checkout session", err)
}

// GetCheckoutSessionByToken returns the session of es whose token hash is
// tokenHash and whose booking is bookingID (ErrNotFound otherwise: wrong
// token, other booking or other establishment are indistinguishable).
func GetCheckoutSessionByToken(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID, tokenHash string) (*CheckoutSession, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(CheckoutSession)
	err := db.NewSelect().Model(s).
		Where("cks.token_hash = ?", tokenHash).
		Where("cks.establishment_id = ?", es.ID()).
		Where("cks.booking_id = ?", bookingID).
		Scan(ctx)
	return s, wrapRead("get checkout session", err)
}

// UpdateCheckoutSession writes the state, expiry and payment attempts of s.
func UpdateCheckoutSession(ctx context.Context, db bun.IDB, es EstablishmentScope, s *CheckoutSession) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewUpdate().Model(s).Column("state", "expires_at", "payment_attempts", "locale", "updated_at").
		Where("id = ?", s.ID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("update checkout session", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return nil
}

// CountCheckoutSessions counts the sessions of es created at or after since
// (tests and diagnostics).
func CountCheckoutSessions(ctx context.Context, db bun.IDB, es EstablishmentScope, since time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	n, err := db.NewSelect().Model((*CheckoutSession)(nil)).
		Where("establishment_id = ?", es.ID()).Where("created_at >= ?", since.UTC()).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count checkout sessions: %w", err)
	}
	return int(n), nil
}

// SetBookingOnlineTerms records the terms version accepted by the customer,
// the instant of the acceptance and the communication locale of a booking
// of es (spec 12.3 step 4, 23), in the caller's transaction (the booking
// row is locked and versioned by the booking service).
func SetBookingOnlineTerms(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string, version *string, acceptedAt *time.Time, locale *string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	var at *time.Time
	if acceptedAt != nil {
		t := acceptedAt.UTC()
		at = &t
	}
	res, err := db.NewUpdate().Model((*Booking)(nil)).
		Set("terms_version = ?", version).Set("terms_accepted_at = ?", at).Set("locale = ?", locale).
		Where("id = ?", bookingID).Where("establishment_id = ?", es.ID()).
		Exec(ctx)
	if err != nil {
		return wrapWrite("set booking terms", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return nil
}
