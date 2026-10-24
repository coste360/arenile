package booking

import (
	"context"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/store"
)

// Booking transitions of the season pass renewals (spec 10.4, M5B; caller
// internal/domain/renewal). A renewal proposal owns a held season pass
// booking of the target season (renewal_of_booking_id = the source pass)
// whose hold lasts until the pre-emption deadline, so the spot is protected
// by the allocation constraints (ADR 0003, ADR 0017). Every method runs in
// the caller's transaction (the renewal service updates the proposal in the
// same one) through mutateInTx: row lock, conditional update, audit and
// outbox (INV-18).

// Audit actions of the renewal transitions that are not status changes.
const actionHoldExtended = "booking.hold_extended"

// ExtendHold moves the hold of a held or pending_payment booking to until
// (pre-emption deadline extended or shortened by the campaign; must be in
// the future): hold_expires_at and the hold allocations move together,
// audit booking.hold_extended and booking.modified. A hold already elapsed
// is availability.ErrHoldExpired.
func (s *Service) ExtendHold(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, until time.Time) error {
	_, err := s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		st := Status(b.Status)
		if st != StatusHeld && st != StatusPendingPayment {
			return nil, &TransitionError{From: st, To: StatusHeld}
		}
		if b.HoldExpiresAt == nil || !b.HoldExpiresAt.After(s.now()) {
			return nil, availability.ErrHoldExpired
		}
		until = until.UTC()
		if err := s.d.Inventory.ExtendHold(ctx, tx, es, b.ID, until); err != nil {
			return nil, err
		}
		prev := *b.HoldExpiresAt
		b.HoldExpiresAt = &until
		return &outcome{action: actionHoldExtended, event: eventModified, diff: map[string]any{
			"hold_expires_at": until.Format(time.RFC3339), "previous_hold_expires_at": prev.Format(time.RFC3339), "renewal": true,
		}}, nil
	})
	return err
}

// ConfirmRenewal confirms the held or pending_payment booking of a renewal
// accepted at the desk (manual driver, spec 13.2, INV-19): the hold
// allocations become season allocations. Online acceptances are confirmed
// by the payment webhook (ConfirmFromPayment), never here. The approval
// rule applies as to any confirmation (R2, INV-78): a renewal whose pack
// or policy requires approval waits in awaiting_approval for the manager.
func (s *Service) ConfirmRenewal(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID, note string) (*Detail, error) {
	return s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		st := Status(b.Status)
		if st == StatusExpired {
			return nil, availability.ErrHoldExpired
		}
		if st != StatusHeld && st != StatusPendingPayment {
			return nil, &TransitionError{From: st, To: StatusConfirmed}
		}
		if b.HoldExpiresAt == nil || !b.HoldExpiresAt.After(s.now()) {
			return nil, availability.ErrHoldExpired
		}
		needed, err := s.needsApproval(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		var o *outcome
		if DecideApproval(ApprovalCase{Op: ApprovalOnConfirm, From: st, Needed: needed}) == ApprovalAwait {
			o, err = s.awaitApprovalLocked(ctx, tx, es, b, "renewal")
		} else {
			o, err = s.confirmLocked(ctx, tx, es, b, note, false)
		}
		if err != nil {
			return nil, err
		}
		o.diff["renewal"] = true
		return o, nil
	})
}

// EndRenewal releases the booking of a renewal proposal that will not be
// renewed: a held booking that the customer declined becomes cancelled; any
// other open booking (deadline passed, campaign closed, forced release, or
// a declined booking already pending_payment) expires, with the refund of
// what was paid online scheduled in the same transaction (INV-51). The
// spot is released for general sale (allocation.released through the
// booking events). A booking already cancelled or expired is left alone
// (idempotent); confirmed or other statuses are an invalid transition.
func (s *Service) EndRenewal(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, declined bool, reason string) (*Detail, error) {
	b, err := store.LockBooking(ctx, tx, es, bookingID)
	if err != nil {
		return nil, notFound(err)
	}
	switch Status(b.Status) {
	case StatusCancelled, StatusExpired:
		return s.loadDetail(ctx, tx, es, b)
	}
	return s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		st := Status(b.Status)
		if st != StatusHeld && st != StatusPendingPayment {
			return nil, &TransitionError{From: st, To: StatusExpired}
		}
		reason = strings.TrimSpace(reason)
		if declined && st == StatusHeld {
			alloc, err := s.releaseAll(ctx, tx, es, b)
			if err != nil {
				return nil, err
			}
			now := s.now()
			b.Status = string(StatusCancelled)
			b.CancelledAt = &now
			b.HoldExpiresAt = nil
			if reason != "" {
				b.CancelReason = &reason
			}
			o := &outcome{action: eventCancelled, alloc: alloc, diff: map[string]any{
				"renewal": "declined", "refund_amount": int64(0), "settlement": string(SettleNone),
			}}
			// Credit applied before the decline goes back as credit (INV-71).
			if err := s.returnCreditOnEnd(ctx, tx, es, b, o); err != nil {
				return nil, err
			}
			return o, nil
		}
		why := "renewal_expired"
		if declined {
			why = "renewal_declined"
		}
		return s.expireLocked(ctx, tx, es, b, map[string]any{"reason": why})
	})
}

// AcceptRenewalTerms records that the customer accepted the terms version
// of the establishment while accepting a renewal from the magic link
// (spec 23): terms_version and terms_accepted_at of the booking, audit
// booking.terms_accepted (no outbox event: nothing observable changed).
func (s *Service) AcceptRenewalTerms(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID, version string) error {
	_, err := s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		st := Status(b.Status)
		if st != StatusHeld && st != StatusPendingPayment {
			return nil, &TransitionError{From: st, To: StatusPendingPayment}
		}
		v := strings.TrimSpace(version)
		var vp *string
		if v != "" {
			vp = &v
		}
		at := s.now()
		if err := store.SetBookingOnlineTerms(ctx, tx, es, b.ID, vp, &at, b.Locale); err != nil {
			return nil, err
		}
		b.TermsVersion, b.TermsAcceptedAt = vp, &at
		return &outcome{action: actionTermsAccepted, event: "-", diff: map[string]any{"terms_version": v, "renewal": true}}, nil
	})
	return err
}

// actionTermsAccepted is the audit action of AcceptRenewalTerms.
const actionTermsAccepted = "booking.terms_accepted"
