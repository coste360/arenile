//go:build integration

package renewal_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestRenewal_CampaignLifecycle: the campaign proposes every pass of the
// source season as a held season pass of the target season on the same
// spot (price of the new list, hold until the deadline), the held spot
// cannot be sold to anyone else, and each proposal ends in one of the
// outcomes (desk acceptance, online acceptance confirmed by the payment
// webhook, decline, expiry at the deadline), each releasing or keeping the
// spot as it should, with audit and outbox events.
func TestRenewal_CampaignLifecycle(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 4)
		ctx, f := x.ctx, x.F

		// Preview: nothing written.
		prev, err := x.rn.CreateCampaign(ctx, f.Owner, f.Est.ID, renewal.CreateInput{
			SourceSeasonID: f.Season.ID, TargetSeasonID: x.target.ID, DeadlineAt: deadline, DryRun: true,
		})
		if err != nil || !prev.DryRun || len(prev.Proposals) != 4 || len(prev.Skipped) != 0 {
			t.Fatalf("preview: %v %+v", err, prev)
		}
		if n := x.Count((*store.RenewalCampaign)(nil), ""); n != 0 {
			t.Fatalf("preview wrote %d campaigns", n)
		}
		days := x.target.OpensOn.DaysUntil(x.target.ClosesOn) + 1
		wantTotal := int64(days) * 2000
		if int64(prev.Proposals[0].TotalAmount) != wantTotal || prev.Proposals[0].StartsOn != x.target.OpensOn.String() {
			t.Fatalf("preview price %d (want %d) from %s", prev.Proposals[0].TotalAmount, wantTotal, prev.Proposals[0].StartsOn)
		}

		res := x.campaign()
		if res.Campaign == nil || len(res.Proposals) != 4 || res.Campaign.Counts.Proposed != 4 || res.Campaign.Name != "Rinnovi 2028" {
			t.Fatalf("campaign: %+v", res)
		}
		if _, err := x.rn.CreateCampaign(ctx, f.Owner, f.Est.ID, renewal.CreateInput{
			SourceSeasonID: f.Season.ID, TargetSeasonID: x.target.ID, DeadlineAt: deadline,
		}); !errors.Is(err, renewal.ErrCampaignExists) {
			t.Fatalf("second campaign: %v", err)
		}
		props := res.Proposals
		for i, p := range props {
			b := x.Booking(p.BookingID)
			if b.Status != "held" || !b.IsSeasonPass || b.RenewalOfBookingID == nil || *b.RenewalOfBookingID != x.passes[i].Booking.ID ||
				b.HoldExpiresAt == nil || !b.HoldExpiresAt.Equal(deadline) || b.SeasonID == nil || *b.SeasonID != x.target.ID {
				t.Fatalf("proposal booking %d: %+v", i, b)
			}
			if p.SpotID != f.Spots[i].ID || int64(p.TotalAmount) != wantTotal || p.PreviousAmount != x.passes[i].Booking.TotalAmount {
				t.Fatalf("proposal %d: %+v", i, p)
			}
		}
		// Pre-emption: before the deadline nobody else gets the spot.
		var ce *availability.ConflictError
		if err := x.sale(f.Spots[0], "2028-07-15"); !errors.As(err, &ce) {
			t.Fatalf("sale of a proposed spot: %v", err)
		}
		// A campaign the pass is already in skips it.
		if open, err := store.OpenRenewalSources(ctx, db, x.ES, []string{x.passes[0].Booking.ID}); err != nil || !open[x.passes[0].Booking.ID] {
			t.Fatalf("open sources: %v %v", err, open)
		}

		// Send: proposal, then reminder.
		if _, err := x.rn.Send(ctx, f.Owner, f.Est.ID, props[0].ID, ""); err != nil {
			t.Fatal(err)
		}
		p0, err := x.rn.Send(ctx, f.Owner, f.Est.ID, props[0].ID, "")
		if err != nil || p0.SendCount != 2 || p0.LastSentAt == nil {
			t.Fatalf("second send: %v %+v", err, p0)
		}
		if len(x.notes.sent) != 2 || x.notes.sent[0].Template != notify.TemplateRenewalProposal || x.notes.sent[1].Template != notify.TemplateRenewalReminder ||
			x.notes.sent[0].Recipient != "abbonato0@example.org" || x.notes.sent[0].BookingID != props[0].BookingID || x.notes.sent[0].DedupeKey == x.notes.sent[1].DedupeKey {
			t.Fatalf("emails: %+v", x.notes.sent)
		}

		// 0: accepted at the desk and confirmed (manual driver).
		version := p0.Version
		if _, err := x.rn.Accept(ctx, f.Owner, f.Est.ID, props[0].ID, &[]int64{version - 1}[0], renewal.AcceptInput{Confirm: true}); !errors.Is(err, configerr.ErrVersionConflict) {
			var vc *configerr.VersionConflictError
			if !errors.As(err, &vc) {
				t.Fatalf("stale If-Match: %v", err)
			}
		}
		acc, err := x.rn.Accept(ctx, f.Owner, f.Est.ID, props[0].ID, &version, renewal.AcceptInput{Confirm: true, Note: "pagherà in cassa"})
		if err != nil || acc.Status != store.RenewalConverted || acc.BookingStatus != "confirmed" || acc.ResponseChannel == nil || *acc.ResponseChannel != store.RenewalResponseDesk {
			t.Fatalf("desk accept: %v %+v", err, acc)
		}
		var kinds []string
		if err := db.NewSelect().Model((*store.SpotAllocation)(nil)).Column("kind").Where("booking_id = ?", props[0].BookingID).Distinct().Scan(ctx, &kinds); err != nil || len(kinds) != 1 || kinds[0] != "season" {
			t.Fatalf("allocations of the converted pass: %v %v", err, kinds)
		}

		// 1: accepted online, paid through the provider, confirmed by the webhook.
		x.enableOnline()
		tok := x.link(props[1].BookingID)
		pub, err := x.rn.GetByToken(ctx, checkouttestSlug, tok)
		if err != nil || !pub.CanAccept || !pub.CanPayFull || pub.SeasonYear != 2028 || pub.CanUseCredit {
			t.Fatalf("public view: %v %+v", err, pub)
		}
		if _, err := x.rn.AcceptByToken(ctx, checkouttestSlug, tok, renewal.PublicAcceptInput{Payment: renewal.PaymentDeposit}); err == nil {
			t.Fatal("deposit accepted without a deposit in the policy")
		}
		out, err := x.rn.AcceptByToken(ctx, checkouttestSlug, tok, renewal.PublicAcceptInput{Payment: renewal.PaymentFull})
		if err != nil || out.Pay.Mode != checkout.PayRedirect || int64(out.Pay.Amount) != wantTotal || out.Proposal.Proposal.Status != store.RenewalAccepted {
			t.Fatalf("online accept: %v %+v", err, out)
		}
		b1 := x.Booking(props[1].BookingID)
		if b1.Status != "pending_payment" || !b1.HoldExpiresAt.Equal(deadline) || b1.TermsVersion != nil {
			t.Fatalf("pending renewal: %s %v", b1.Status, b1.HoldExpiresAt)
		}
		if start := x.Pay.Starts[len(x.Pay.Starts)-1]; start.Purpose != "full" || strings.Contains(start.ReturnURL, tok) {
			t.Fatalf("provider request: %+v", start)
		}
		// The browser return never confirms: only the webhook does.
		if n, err := x.rn.Sync(ctx); err != nil || n != 0 {
			t.Fatalf("sync before the webhook: %d %v", n, err)
		}
		webhookConfirm(t, x, props[1].BookingID)
		// Paid but not yet converted (the sink has not run): accepting again
		// from the link is refused as no longer open, not as expired.
		if _, err := x.rn.AcceptByToken(ctx, checkouttestSlug, tok, renewal.PublicAcceptInput{Payment: renewal.PaymentFull}); !errors.Is(err, renewal.ErrNotOpen) {
			t.Fatalf("accept after the payment: %v", err)
		}
		// The booking.confirmed event converts the proposal at once (event
		// sink); a redelivery and the next job run change nothing more.
		var confirmed store.OutboxEvent
		if err := db.NewSelect().Model(&confirmed).Where("type = ?", event.BookingConfirmed).
			Where("CAST(payload AS TEXT) LIKE ?", "%"+props[1].BookingID+"%").OrderExpr("occurred_at DESC").Limit(1).Scan(ctx); err != nil {
			t.Fatalf("booking.confirmed of the renewal: %v", err)
		}
		for range 2 {
			if err := x.rn.Handle(ctx, event.FromRow(confirmed)); err != nil {
				t.Fatalf("renewal sink: %v", err)
			}
		}
		if p := x.proposal(props[1].ID); p.Status != store.RenewalConverted || *p.ResponseChannel != store.RenewalResponseOnline {
			t.Fatalf("online proposal: %+v", p)
		}
		if n, err := x.rn.Sync(ctx); err != nil || n != 0 {
			t.Fatalf("sync after the sink: %d %v", n, err)
		}

		// 2: declined by the customer: spot free again.
		tok2 := x.link(props[2].BookingID)
		dec, err := x.rn.DeclineByToken(ctx, checkouttestSlug, tok2, "cambio città")
		if err != nil || dec.Proposal.Status != store.RenewalDeclined || dec.CanAccept {
			t.Fatalf("decline: %v %+v", err, dec)
		}
		if b := x.Booking(props[2].BookingID); b.Status != "cancelled" {
			t.Fatalf("declined booking %s", b.Status)
		}
		if err := x.sale(f.Spots[2], "2028-07-15"); err != nil {
			t.Fatalf("sale after decline: %v", err)
		}
		if _, err := x.rn.AcceptByToken(ctx, checkouttestSlug, tok2, renewal.PublicAcceptInput{Payment: renewal.PaymentFull}); !errors.Is(err, renewal.ErrNotOpen) {
			t.Fatalf("accept after decline: %v", err)
		}

		// 3: no answer: expires at the deadline, the spot goes on sale.
		if err := x.sale(f.Spots[3], "2028-07-15"); !errors.As(err, &ce) {
			t.Fatalf("sale before the deadline: %v", err)
		}
		f.Clock.Set(deadline.Add(time.Minute))
		if _, err := x.rn.Accept(ctx, f.Owner, f.Est.ID, props[3].ID, nil, renewal.AcceptInput{Confirm: true}); !errors.Is(err, renewal.ErrExpired) {
			t.Fatalf("accept after the deadline: %v", err)
		}
		if n, err := x.rn.Sync(ctx); err != nil || n != 1 {
			t.Fatalf("sync at the deadline: %d %v", n, err)
		}
		if p := x.proposal(props[3].ID); p.Status != store.RenewalExpired || *p.ResponseChannel != store.RenewalResponseSystem {
			t.Fatalf("expired proposal: %+v", p)
		}
		if b := x.Booking(props[3].BookingID); b.Status != "expired" {
			t.Fatalf("expired booking %s", b.Status)
		}
		if err := x.sale(f.Spots[3], "2028-07-15"); err != nil {
			t.Fatalf("sale after the deadline: %v", err)
		}
		if n, err := x.rn.Sync(ctx); err != nil || n != 0 {
			t.Fatalf("sync is idempotent: %d %v", n, err)
		}

		c, err := x.rn.GetCampaign(ctx, f.Viewer, f.Est.ID, res.Campaign.ID)
		if err != nil || c.Counts != (renewal.Counts{Converted: 2, Declined: 1, Expired: 1, Sent: 1}) || int64(c.ConvertedAmount) != 2*wantTotal {
			t.Fatalf("campaign counts: %v %+v", err, c)
		}
		for typ, want := range map[string]int{
			event.SeasonRenewalOffered: 1, event.SeasonRenewalAccepted: 2, event.SeasonRenewalDeclined: 1, event.SeasonRenewalExpired: 1,
		} {
			if n := x.Count((*store.OutboxEvent)(nil), "type = ?", typ); n != want {
				t.Errorf("outbox %s = %d, want %d", typ, n, want)
			}
		}
		if n := x.Count((*store.AuditEntry)(nil), "entity_type = 'renewal_proposal'"); n < 4+2+2+1+1 {
			t.Errorf("audit entries of the proposals: %d", n)
		}
		// The audit log never carries the free-text reason (INV-30).
		if n := x.Count((*store.AuditEntry)(nil), "CAST(diff AS TEXT) LIKE ?", "%cambio città%"); n != 0 {
			t.Errorf("decline reason copied to the audit log %d times", n)
		}
		// Permissions: the viewer reads, never writes.
		if _, err := x.rn.Send(ctx, f.Viewer, f.Est.ID, props[0].ID, ""); err == nil {
			t.Fatal("viewer sent a proposal")
		}
		if _, err := x.rn.GetCampaign(ctx, f.Outsider, f.Est.ID, res.Campaign.ID); err == nil {
			t.Fatal("outsider read a campaign")
		}
	})
}

// TestRenewal_SkipsAndBulk: passes that cannot be proposed are reported
// (inactive spot, anonymised customer); bulk actions apply per proposal
// and report skipped ones; moving the deadline moves the holds; closing
// the campaign expires what is still open.
func TestRenewal_SkipsAndBulk(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 4)
		ctx, f := x.ctx, x.F
		now := f.Clock.Now()
		if _, err := db.NewUpdate().Model((*store.Spot)(nil)).Set("active = ?", false).Where("id = ?", f.Spots[3].ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := db.NewUpdate().Model((*store.Customer)(nil)).Set("anonymized_at = ?", now).
			Where("id = ?", x.passes[2].Booking.CustomerID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		res := x.campaign()
		reasons := map[string]string{}
		for _, s := range res.Skipped {
			reasons[s.SourcePassID] = s.Reason
		}
		if len(res.Proposals) != 2 || reasons[x.passes[3].Booking.ID] != renewal.SkipSpotInactive || reasons[x.passes[2].Booking.ID] != renewal.SkipCustomerAnonymized {
			t.Fatalf("skips: %+v proposals %d", res.Skipped, len(res.Proposals))
		}

		// Bulk send: both sent, a second run skips them (only_unsent).
		br, err := x.rn.Bulk(ctx, f.Owner, f.Est.ID, res.Campaign.ID, renewal.BulkInput{Action: renewal.ActionSend, Status: store.RenewalProposed, OnlyUnsent: true})
		if err != nil || br.Matched != 2 || br.Succeeded != 2 || len(br.Failed) != 0 {
			t.Fatalf("bulk send: %v %+v", err, br)
		}
		br, err = x.rn.Bulk(ctx, f.Owner, f.Est.ID, res.Campaign.ID, renewal.BulkInput{Action: renewal.ActionSend, Status: store.RenewalProposed, OnlyUnsent: true})
		if err != nil || br.Skipped != 2 || br.Succeeded != 0 {
			t.Fatalf("bulk send again: %v %+v", err, br)
		}
		// Release without force: nothing is due yet.
		br, err = x.rn.Bulk(ctx, f.Owner, f.Est.ID, res.Campaign.ID, renewal.BulkInput{Action: renewal.ActionRelease, Status: store.RenewalProposed})
		if err != nil || br.Skipped != 2 {
			t.Fatalf("bulk release: %v %+v", err, br)
		}

		// Move the deadline: proposals and holds follow.
		later := deadline.Add(7 * 24 * time.Hour)
		c, err := x.rn.UpdateCampaign(ctx, f.Owner, f.Est.ID, res.Campaign.ID, &res.Campaign.Version, renewal.UpdateInput{DeadlineAt: &later})
		if err != nil || !c.DeadlineAt.Equal(later) {
			t.Fatalf("move deadline: %v %+v", err, c)
		}
		for _, p := range res.Proposals {
			if got := x.proposal(p.ID); !got.DeadlineAt.Equal(later) {
				t.Fatalf("proposal deadline %v", got.DeadlineAt)
			}
			if b := x.Booking(p.BookingID); !b.HoldExpiresAt.Equal(later) {
				t.Fatalf("hold %v", b.HoldExpiresAt)
			}
		}
		var holdRows int64
		holdRows, err = db.NewSelect().Model((*store.SpotAllocation)(nil)).Where("booking_id = ?", res.Proposals[0].BookingID).
			Where("expires_at = ?", later).Count(ctx)
		if err != nil || holdRows == 0 {
			t.Fatalf("hold allocations not moved: %d %v", holdRows, err)
		}
		// A deadline after the opening of the new season is refused.
		bad := time.Date(2028, 6, 1, 0, 0, 0, 0, time.UTC)
		if _, err := x.rn.UpdateCampaign(ctx, f.Owner, f.Est.ID, res.Campaign.ID, nil, renewal.UpdateInput{DeadlineAt: &bad}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("deadline after opening: %v", err)
		}

		// Bulk accept without confirmation: waiting for payment.
		br, err = x.rn.Bulk(ctx, f.Owner, f.Est.ID, res.Campaign.ID, renewal.BulkInput{
			Action: renewal.ActionAccept, ProposalIDs: []string{res.Proposals[0].ID}, Confirm: false,
		})
		if err != nil || br.Succeeded != 1 {
			t.Fatalf("bulk accept: %v %+v", err, br)
		}
		if b := x.Booking(res.Proposals[0].BookingID); b.Status != "pending_payment" || !b.HoldExpiresAt.Equal(later) {
			t.Fatalf("accepted booking: %s %v", b.Status, b.HoldExpiresAt)
		}

		// Close: every open proposal expires and frees its spot.
		closed, err := x.rn.UpdateCampaign(ctx, f.Owner, f.Est.ID, res.Campaign.ID, nil, renewal.UpdateInput{Close: true})
		if err != nil || closed.Status != store.RenewalCampaignClosed || closed.ClosedAt == nil || closed.Counts.Expired != 2 {
			t.Fatalf("close: %v %+v", err, closed)
		}
		for i := range 2 {
			if err := x.sale(f.Spots[i], "2028-07-20"); err != nil {
				t.Fatalf("sale after close %d: %v", i, err)
			}
		}
		if _, err := x.rn.UpdateCampaign(ctx, f.Owner, f.Est.ID, res.Campaign.ID, nil, renewal.UpdateInput{DeadlineAt: &later}); !errors.Is(err, renewal.ErrNotOpen) {
			t.Fatalf("move deadline of a closed campaign: %v", err)
		}
		// A new campaign for the same seasons is refused even after closing.
		if _, err := x.rn.CreateCampaign(ctx, f.Owner, f.Est.ID, renewal.CreateInput{
			SourceSeasonID: f.Season.ID, TargetSeasonID: x.target.ID, DeadlineAt: deadline,
		}); !errors.Is(err, renewal.ErrCampaignExists) {
			t.Fatalf("campaign after close: %v", err)
		}
	})
}

// TestRenewal_CreateValidation covers the request checks.
func TestRenewal_CreateValidation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 1)
		ctx, f := x.ctx, x.F
		for name, in := range map[string]renewal.CreateInput{
			"past deadline":          {SourceSeasonID: f.Season.ID, TargetSeasonID: x.target.ID, DeadlineAt: x.endedAt.Add(-time.Hour)},
			"deadline after opening": {SourceSeasonID: f.Season.ID, TargetSeasonID: x.target.ID, DeadlineAt: time.Date(2028, 7, 1, 0, 0, 0, 0, time.UTC)},
			"same seasons":           {SourceSeasonID: f.Season.ID, TargetSeasonID: f.Season.ID, DeadlineAt: deadline},
			"backwards":              {SourceSeasonID: x.target.ID, TargetSeasonID: f.Season.ID, DeadlineAt: deadline},
			"foreign pass":           {SourceSeasonID: f.Season.ID, TargetSeasonID: x.target.ID, DeadlineAt: deadline, PassIDs: []string{store.NewID()}},
		} {
			if _, err := x.rn.CreateCampaign(ctx, f.Owner, f.Est.ID, in); !errors.Is(err, configerr.ErrValidation) {
				t.Errorf("%s: want validation error, got %v", name, err)
			}
		}
	})
}

// webhookConfirm confirms a pending booking as the payment webhook does
// (booking.ConfirmFromPayment inside the payment transaction).
func webhookConfirm(t *testing.T, x *fx, bookingID string) {
	t.Helper()
	err := store.RunInTx(x.ctx, x.DB, func(ctx context.Context, tx bun.Tx) error {
		return x.Book.ConfirmFromPayment(ctx, tx, nil, x.ES, bookingID, &store.Payment{ID: store.NewID(), Provider: "stripe"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if b := x.Booking(bookingID); b.Status != "confirmed" {
		t.Fatalf("webhook did not confirm: %s", b.Status)
	}
}

const checkouttestSlug = "lido-delfino"

// TestRenewal_SpotTakenSkipped: a spot sold in the new season before the
// campaign skips its pass (spot_unavailable) and the savepoint of that
// proposal leaves nothing behind (no booking, no allocation, no audit),
// while the other proposals of the same transaction are kept.
func TestRenewal_SpotTakenSkipped(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 2)
		f := x.F
		if err := x.sale(f.Spots[1], "2028-08-01"); err != nil {
			t.Fatal(err)
		}
		before := x.Count((*store.AuditEntry)(nil), "")
		res := x.campaign()
		if len(res.Proposals) != 1 || len(res.Skipped) != 1 || res.Skipped[0].Reason != renewal.SkipSpotUnavailable ||
			res.Skipped[0].SourcePassID != x.passes[1].Booking.ID {
			t.Fatalf("campaign: %+v", res)
		}
		if n := x.Count((*store.Booking)(nil), "renewal_of_booking_id = ?", x.passes[1].Booking.ID); n != 0 {
			t.Fatalf("orphan proposal booking: %d", n)
		}
		if n := x.Count((*store.RenewalProposal)(nil), ""); n != 1 {
			t.Fatalf("proposals: %d", n)
		}
		// Audit: booking created + hold extended + proposal created for the
		// kept pass, plus the campaign; nothing for the skipped one.
		if n := x.Count((*store.AuditEntry)(nil), "") - before; n != 4 {
			t.Fatalf("audit entries written: %d", n)
		}
	})
}
