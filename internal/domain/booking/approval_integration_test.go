//go:build integration

package booking_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestApprovalRule_DeskCannotBypass covers rules R1-R4 of approval.go
// (ADR 0019 E) end to end on the real schema.
func TestApprovalRule_DeskCannotBypass(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)

		// R1: a forced desk creation of a pack needing approval still waits.
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.Lines[0].PackID, in.Force = f.VIP.ID, true
		forced, err := f.Svc.Create(f.Ctx, f.Cashier, f.Est.ID, in)
		if err != nil || forced.Booking.Status != string(booking.StatusAwaitingApproval) {
			t.Fatalf("forced vip: %v %+v", err, forced)
		}
		// R3: the desk can neither approve nor confirm it.
		if _, err := f.Svc.Approve(f.Ctx, f.Cashier, f.Est.ID, forced.Booking.ID, booking.ActionInput{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier approve: %v", err)
		}
		if _, err := f.Svc.Confirm(f.Ctx, f.Cashier, f.Est.ID, forced.Booking.ID, booking.ActionInput{}); !errors.Is(err, booking.ErrApprovalRequired) {
			t.Fatalf("cashier confirm of awaiting: %v", err)
		}
		// R3: the manager confirming it is approving it (audited).
		got, err := f.Svc.Confirm(f.Ctx, f.Manager, f.Est.ID, forced.Booking.ID, booking.ActionInput{})
		if err != nil || got.Booking.Status != string(booking.StatusConfirmed) {
			t.Fatalf("manager confirm: %v %+v", err, got)
		}
		if f.Count((*store.AuditEntry)(nil), "entity_id = ? AND action = ? AND CAST(diff AS TEXT) LIKE ?", forced.Booking.ID, event.BookingConfirmed, "%approved%") != 1 {
			t.Fatal("approval by confirm not audited")
		}

		// R2: a held desk booking needing approval goes to awaiting_approval on Confirm.
		held := f.Input(f.Spots[1], "2027-07-10", "2027-07-11")
		held.Lines[0].PackID, held.Status = f.VIP.ID, booking.StatusHeld
		h, err := f.Svc.Create(f.Ctx, f.Cashier, f.Est.ID, held)
		if err != nil || h.Booking.Status != string(booking.StatusHeld) {
			t.Fatalf("held vip: %v %+v", err, h)
		}
		w, err := f.Svc.Confirm(f.Ctx, f.Cashier, f.Est.ID, h.Booking.ID, booking.ActionInput{})
		if err != nil || w.Booking.Status != string(booking.StatusAwaitingApproval) || w.Booking.HoldExpiresAt == nil {
			t.Fatalf("held vip confirm: %v %+v", err, w)
		}
		for _, a := range w.SpotAllocations {
			if a.Kind != store.AllocationHold {
				t.Fatalf("awaiting allocation %+v", a)
			}
		}

		// R2: a draft needing approval is allocated with holds and waits.
		draft := f.Input(f.Spots[2], "2027-07-10", "2027-07-11")
		draft.Lines[0].PackID, draft.Status = f.VIP.ID, booking.StatusDraft
		dr, err := f.Svc.Create(f.Ctx, f.Cashier, f.Est.ID, draft)
		if err != nil {
			t.Fatal(err)
		}
		dw, err := f.Svc.Confirm(f.Ctx, f.Cashier, f.Est.ID, dr.Booking.ID, booking.ActionInput{})
		if err != nil || dw.Booking.Status != string(booking.StatusAwaitingApproval) || len(dw.SpotAllocations) == 0 {
			t.Fatalf("draft vip confirm: %v %+v", err, dw)
		}

		// R2: a payment collected at the desk does not confirm it either.
		paid := f.Input(f.Spots[3], "2027-07-10", "2027-07-11")
		paid.Lines[0].PackID, paid.Status = f.VIP.ID, booking.StatusHeld
		p, err := f.Svc.Create(f.Ctx, f.Cashier, f.Est.ID, paid)
		if err != nil {
			t.Fatal(err)
		}
		es, err := store.SystemEstablishmentScope(f.Ctx, f.DB, f.Est.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Svc.ConfirmFromPayment(f.Ctx, f.DB, f.Cashier, es, p.Booking.ID, &store.Payment{ID: store.NewID(), Provider: "manual"}); err != nil {
			t.Fatal(err)
		}
		if b, _ := store.GetBooking(f.Ctx, f.DB, es, p.Booking.ID); b.Status != string(booking.StatusAwaitingApproval) {
			t.Fatalf("paid vip status %s", b.Status)
		}

		// R4: a forced desk change of a confirmed booking to a pack needing
		// approval is refused; the manager may do it (audited as approved).
		base := f.MustCreate(f.Input(f.Spots[0], "2027-08-10", "2027-08-11"))
		patch := booking.PatchInput{Lines: []booking.LineInput{{PackID: f.VIP.ID, SpotID: f.Spots[0].ID}}, Force: true}
		if _, err := f.Svc.Update(f.Ctx, f.Cashier, f.Est.ID, base.Booking.ID, nil, patch); !errors.Is(err, booking.ErrApprovalRequired) {
			t.Fatalf("cashier forced patch: %v", err)
		}
		res, err := f.Svc.Update(f.Ctx, f.Manager, f.Est.ID, base.Booking.ID, nil, patch)
		if err != nil || res.Detail.Booking.Status != string(booking.StatusConfirmed) {
			t.Fatalf("manager patch: %v %+v", err, res)
		}
		if f.Count((*store.AuditEntry)(nil), "entity_id = ? AND action = ? AND CAST(diff AS TEXT) LIKE ?", base.Booking.ID, event.BookingModified, "%approved%") != 1 {
			t.Fatal("approved modification not audited")
		}
		// The booking is now approved: the desk may change it further.
		notes := "ok"
		if _, err := f.Svc.Update(f.Ctx, f.Cashier, f.Est.ID, base.Booking.ID, nil, booking.PatchInput{NotesInternal: &notes}); err != nil {
			t.Fatalf("cashier patch of approved booking: %v", err)
		}
	})
}
