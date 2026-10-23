//go:build integration

package renewal_test

import (
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestRenewal_DeclinedAfterCreditReturnsCredit (INV-71, spec 10.4 without
// online payments): a customer who accepts a renewal from the magic link
// applying credit below the deposit, where the establishment takes no
// online payments, gets "pay at the desk" (pay mode none) and the proposal
// stays accepted; the credit comes back as credit, in the same
// transaction, both when the customer then declines and when the deadline
// passes without the desk payment (renewals.sync).
func TestRenewal_DeclinedAfterCreditReturnsCredit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 2)
		ctx, f := x.ctx, x.F
		props := x.campaign().Proposals
		accept := func(i int) *renewal.PublicAcceptResult {
			t.Helper()
			cust := x.passes[i].Booking.CustomerID
			x.grant(cust, 500)
			tok := x.link(props[i].BookingID)
			pub, err := x.rn.GetByToken(ctx, checkouttestSlug, tok)
			if err != nil || !pub.CanAccept || !pub.CanUseCredit || pub.CanPayFull || pub.CreditAvailable != 500 {
				t.Fatalf("public view %d: %v %+v", i, err, pub)
			}
			out, err := x.rn.AcceptByToken(ctx, checkouttestSlug, tok, renewal.PublicAcceptInput{Payment: renewal.PaymentFull, UseCredit: true})
			if err != nil || out.CreditApplied != 500 || out.Pay.Mode != checkout.PayNone || out.Proposal.Proposal.Status != store.RenewalAccepted {
				t.Fatalf("accept without provider %d: %v %+v", i, err, out)
			}
			if b := x.Booking(props[i].BookingID); b.Status != "held" || b.PaidAmount != 500 || !b.HoldExpiresAt.Equal(deadline) {
				t.Fatalf("booking %d after credit: %s paid %d hold %v", i, b.Status, b.PaidAmount, b.HoldExpiresAt)
			}
			if bal := x.balance(cust); bal != 0 {
				t.Fatalf("balance %d after redemption: %d", i, bal)
			}
			return out
		}

		// 0: accepted with credit, then declined: the credit is back.
		accept(0)
		tok0 := x.link(props[0].BookingID)
		dec, err := x.rn.DeclineByToken(ctx, checkouttestSlug, tok0, "")
		if err != nil || dec.Proposal.Status != store.RenewalDeclined {
			t.Fatalf("decline after credit: %v %+v", err, dec)
		}
		if b := x.Booking(props[0].BookingID); b.Status != "cancelled" || b.PaidAmount != 0 {
			t.Fatalf("declined booking: %s paid %d", b.Status, b.PaidAmount)
		}
		if bal := x.balance(x.passes[0].Booking.CustomerID); bal != 500 {
			t.Fatalf("credit back after decline: %d", bal)
		}
		if n := x.Count((*store.CustomerCredit)(nil), "customer_id = ? AND source = ? AND amount = 500",
			x.passes[0].Booking.CustomerID, store.CreditSourceCancellation); n != 1 {
			t.Fatalf("cancellation credit rows %d", n)
		}

		// 1: accepted with credit, never paid at the desk: the sync job
		// leaves it alone until the deadline, then expires it and the
		// credit is back.
		accept(1)
		if n, err := x.rn.Sync(ctx); err != nil || n != 0 {
			t.Fatalf("sync before the deadline: %d %v", n, err)
		}
		f.Clock.Set(deadline.Add(time.Minute))
		if n, err := x.rn.Sync(ctx); err != nil || n != 1 {
			t.Fatalf("sync at the deadline: %d %v", n, err)
		}
		if p := x.proposal(props[1].ID); p.Status != store.RenewalExpired {
			t.Fatalf("proposal after the deadline: %s", p.Status)
		}
		if b := x.Booking(props[1].BookingID); b.Status != "expired" || b.PaidAmount != 0 {
			t.Fatalf("expired booking: %s paid %d", b.Status, b.PaidAmount)
		}
		if bal := x.balance(x.passes[1].Booking.CustomerID); bal != 500 {
			t.Fatalf("credit back after expiry: %d", bal)
		}
		if err := x.sale(f.Spots[1], "2028-07-15"); err != nil {
			t.Fatalf("sale after expiry: %v", err)
		}
	})
}

// TestRenewal_RevokedPassReleasesPreemption (INV-68): a source pass
// cancelled after the campaign was created cannot be renewed any more: the
// desk and the magic link are refused, the booking.cancelled event (or the
// next sync run) expires the proposal and releases the spot, and a repeated
// desk accept of an already accepted proposal is idempotent.
func TestRenewal_RevokedPassReleasesPreemption(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 3)
		ctx, f := x.ctx, x.F
		props := x.campaign().Proposals
		revoke := func(i int) {
			t.Helper()
			if _, err := x.Book.Cancel(ctx, f.Owner, f.Est.ID, x.passes[i].Booking.ID, booking.CancelInput{Reason: "revoked"}); err != nil {
				t.Fatalf("cancel pass %d: %v", i, err)
			}
		}

		// 0: revoked, then the job runs: refused everywhere, spot released.
		revoke(0)
		if _, err := x.rn.Accept(ctx, f.Owner, f.Est.ID, props[0].ID, nil, renewal.AcceptInput{Confirm: true}); !errors.Is(err, renewal.ErrSourceCancelled) {
			t.Fatalf("desk accept of a revoked pass: %v", err)
		}
		tok := x.link(props[0].BookingID)
		if _, err := x.rn.AcceptByToken(ctx, checkouttestSlug, tok, renewal.PublicAcceptInput{Payment: renewal.PaymentFull}); !errors.Is(err, renewal.ErrSourceCancelled) {
			t.Fatalf("link accept of a revoked pass: %v", err)
		}
		var ce *availability.ConflictError
		if err := x.sale(f.Spots[0], "2028-07-15"); !errors.As(err, &ce) {
			t.Fatalf("spot sold before the sync: %v", err)
		}
		if n, err := x.rn.Sync(ctx); err != nil || n != 1 {
			t.Fatalf("sync: %d %v", n, err)
		}
		if p := x.proposal(props[0].ID); p.Status != store.RenewalExpired {
			t.Fatalf("proposal of the revoked pass: %s", p.Status)
		}
		if b := x.Booking(props[0].BookingID); b.Status != "expired" {
			t.Fatalf("booking of the revoked pass: %s", b.Status)
		}
		if err := x.sale(f.Spots[0], "2028-07-15"); err != nil {
			t.Fatalf("sale after the release: %v", err)
		}
		if n := x.Count((*store.AuditEntry)(nil), "entity_type = 'renewal_proposal' AND CAST(diff AS TEXT) LIKE ?", "%source_pass_cancelled%"); n != 1 {
			t.Fatalf("audit of the release: %d", n)
		}

		// 1: revoked, the booking.cancelled event releases at once (sink).
		revoke(1)
		var cancelled store.OutboxEvent
		if err := db.NewSelect().Model(&cancelled).Where("type = ?", event.BookingCancelled).
			Where("CAST(payload AS TEXT) LIKE ?", "%"+x.passes[1].Booking.ID+"%").OrderExpr("occurred_at DESC").Limit(1).Scan(ctx); err != nil {
			t.Fatalf("booking.cancelled of the pass: %v", err)
		}
		for range 2 {
			if err := x.rn.Handle(ctx, event.FromRow(cancelled)); err != nil {
				t.Fatalf("renewal sink: %v", err)
			}
		}
		if p := x.proposal(props[1].ID); p.Status != store.RenewalExpired {
			t.Fatalf("proposal after the sink: %s", p.Status)
		}
		if err := x.sale(f.Spots[1], "2028-07-15"); err != nil {
			t.Fatalf("sale after the sink: %v", err)
		}

		// 2: a second desk accept (no confirm) of an accepted proposal is
		// idempotent: same status, no error.
		first, err := x.rn.Accept(ctx, f.Owner, f.Est.ID, props[2].ID, nil, renewal.AcceptInput{})
		if err != nil || first.Status != store.RenewalAccepted || first.BookingStatus != "pending_payment" {
			t.Fatalf("desk accept: %v %+v", err, first)
		}
		again, err := x.rn.Accept(ctx, f.Owner, f.Est.ID, props[2].ID, nil, renewal.AcceptInput{})
		if err != nil || again.Status != store.RenewalAccepted || again.Version != first.Version {
			t.Fatalf("repeated desk accept: %v %+v", err, again)
		}
		if n := x.Count((*store.OutboxEvent)(nil), "type = ?", event.SeasonRenewalAccepted); n != 1 {
			t.Fatalf("accepted events %d", n)
		}
	})
}
