package ticketing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Refund settlements of a void (api.TicketVoidRequest.refund).
const (
	VoidRefund = "refund" // money back with the ticket's method (default)
	VoidCredit = "credit" // customer credit (not for the walk-in customer)
	VoidNone   = "none"   // no refund
)

// maxVoidReason bounds the reason of a void.
const maxVoidReason = 500

// VoidInput is api.TicketVoidRequest.
type VoidInput struct {
	Reason          string
	Refund          string // VoidRefund (default), VoidCredit or VoidNone
	ExpectedVersion *int64
}

// Void voids a ticket in one transaction (auth.PermBookingsWrite, plus
// auth.PermRefundsIssue unless Refund is none): the booking is cancelled
// through the booking service (audit + outbox, allocations and zone
// capacity released, refund recorded with the manual driver: a cash refund
// needs an open cash session), then the ticket is marked voided and keeps
// its number. 409 ticket_voided when already voided, ticket_already_entered
// when the entry was used.
func (s *Service) Void(ctx context.Context, p *auth.Principal, establishmentID, id string, in VoidInput) (*View, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Refund == "" {
		in.Refund = VoidRefund
	}
	ve := &configerr.ValidationError{}
	if in.Reason == "" || len([]rune(in.Reason)) > maxVoidReason {
		ve.Add("reason", fmt.Sprintf("Indica il motivo (massimo %d caratteri).", maxVoidReason))
	}
	switch in.Refund {
	case VoidRefund, VoidCredit, VoidNone:
	default:
		ve.Add("refund", "Usa refund, credit o none.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	perms := []auth.Permission{auth.PermBookingsWrite}
	if in.Refund != VoidNone {
		perms = append(perms, auth.PermRefundsIssue)
	}
	es, err := s.authorize(ctx, p, establishmentID, perms...)
	if err != nil {
		return nil, err
	}
	var out *View
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		// Lock order (same as the assignment path, MoveThen then the ticket
		// hook): the booking row first, through Cancel, then the ticket row.
		// The ticket is read without a lock for the early checks and
		// re-checked once locked.
		t, err := store.GetTicket(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if err := configerr.CheckVersion(in.ExpectedVersion, t.Version); err != nil {
			return err
		}
		if err := voidable(t); err != nil {
			return err
		}
		if in.Refund == VoidCredit {
			if err := s.checkCreditable(ctx, tx, es, t); err != nil {
				return err
			}
		}
		amount := t.TotalAmount
		cin := booking.CancelInput{Reason: "Biglietto annullato: " + in.Reason, RefundAmount: &amount, FromTicket: true}
		switch in.Refund {
		case VoidRefund:
			cin.Settlement, cin.RefundMethod = booking.SettleRefund, t.PaymentMethod
		case VoidCredit:
			cin.Settlement = booking.SettleCredit
		default:
			cin.Settlement = booking.SettleNone
		}
		res, err := s.d.BookingsIn(tx).Cancel(ctx, p, establishmentID, t.BookingID, cin)
		if err != nil {
			return err
		}
		seen := t.Version
		if t, err = store.LockTicket(ctx, tx, es, id); err != nil {
			return notFound(err)
		}
		if err := voidable(t); err != nil {
			return err
		}
		if t.Version != seen {
			return &configerr.VersionConflictError{Current: t.Version}
		}
		now := s.now()
		version := t.Version
		t.Status, t.VoidedAt, t.VoidedByUserID, t.VoidReason = store.TicketVoided, &now, userIDOf(p), in.Reason
		t.NeedsAssignment, t.ProvisionalSpotID, t.UpdatedAt = false, nil, now
		if res.Recorded != nil {
			t.RefundPaymentID = &res.Recorded.ID
		}
		ok, err := store.UpdateTicketVersioned(ctx, tx, es, t, version)
		if err != nil {
			return err
		}
		if !ok {
			return &configerr.VersionConflictError{Current: version}
		}
		diff := map[string]any{"refund": in.Refund, "reason": in.Reason}
		if res.Recorded != nil {
			diff["refund_payment_id"] = res.Recorded.ID
			diff["refund_amount"] = int64(res.Recorded.Amount)
		}
		if err := s.record(ctx, tx, p, es, "ticket.voided", t, diff); err != nil {
			return err
		}
		out, err = view(ctx, tx, es, t)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// voidable is ErrVoided or ErrAlreadyEntered when t cannot be voided.
func voidable(t *store.Ticket) error {
	switch {
	case t.Status == store.TicketVoided:
		return ErrVoided
	case t.EnteredAt != nil:
		return ErrAlreadyEntered
	}
	return nil
}

// checkCreditable refuses a credit refund to the walk-in customer (the
// owner of the quick sales without a named customer).
func (s *Service) checkCreditable(ctx context.Context, db bun.IDB, es store.EstablishmentScope, t *store.Ticket) error {
	b, err := store.GetBooking(ctx, db, es, t.BookingID)
	if err != nil {
		return err
	}
	st, err := store.GetDailySaleSettings(ctx, db, es)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if st != nil && st.WalkInCustomerID != nil && *st.WalkInCustomerID == b.CustomerID {
		return configerr.Invalid("refund", "Il credito richiede un cliente con nome: usa il rimborso.")
	}
	return nil
}
