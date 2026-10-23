//go:build integration

package checkout_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// confirmed books spot on day through the public flow (manual driver) and
// issues a magic link for it.
func confirmed(e *checkouttest.Env, spot *store.Spot, day string) (*checkout.Session, string) {
	e.T.Helper()
	s := e.Create(e.Seat(spot, day))
	e.Details(s, "anna@example.org")
	if _, err := e.Svc.Pay(e.Ctx, slug, s.BookingID, s.Token, checkout.PayInput{}); err != nil {
		e.T.Fatalf("pay: %v", err)
	}
	link, err := e.Notify.IssueMagicLink(e.Ctx, e.DB, e.ES, s.BookingID, nil)
	if err != nil {
		e.T.Fatal(err)
	}
	return s, link.Token
}

// paidOnline records an online (provider) payment of amount on a booking.
func paidOnline(e *checkouttest.Env, bookingID string, amount money.Cents) {
	e.T.Helper()
	now := e.F.Clock.Now()
	ref := "pi_" + bookingID
	e.F.Insert(&store.Payment{
		ID: store.NewID(), EstablishmentID: e.F.Est.ID, BookingID: &bookingID, Provider: "stripe", Method: "card", Direction: "in",
		Amount: amount, Status: "succeeded", ProviderRef: &ref, VATBreakdown: []store.VATShare{}, BusinessDate: calendar.In(now, time.UTC),
		Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	})
	e.F.PayBooking(bookingID, amount)
}

// TestManage_MagicLinkLifecycle: a link opens exactly its booking with
// the actions of the policy snapshot; wrong, foreign, expired or revoked
// links are plain not found; QR and .ics carry the booking (the .ics also
// the manage link).
func TestManage_MagicLinkLifecycle(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		s, token := confirmed(e, e.F.Spots[0], "2027-07-20")
		m, err := e.Svc.Manage(ctx, slug, token)
		if err != nil {
			t.Fatal(err)
		}
		if m.Booking.Booking.ID != s.BookingID || !m.Allowed.Modify || !m.Allowed.Cancel || m.Allowed.PayBalance || m.Allowed.ReleaseDays ||
			m.QRPayload == "" || m.CancelPreview == nil || m.Allowed.ModifyUntil.String() != "2027-07-18" || m.Quote == nil || m.Labels.Packs[e.F.Base.ID] == nil {
			t.Fatalf("managed %+v", m.Allowed)
		}
		ics, err := e.Svc.ManageDocument(ctx, slug, token, checkout.DocICS)
		if err != nil || !strings.Contains(string(ics.Body), "/b/lido-delfino/m/"+token) {
			t.Fatalf("ics %v", err)
		}
		for name, tok := range map[string]string{"malformed": "x", "unknown": strings.Repeat("Z", 43)} {
			if _, err := e.Svc.Manage(ctx, slug, tok); !errors.Is(err, checkout.ErrLinkNotFound) {
				t.Fatalf("%s link: %v", name, err)
			}
		}
		if _, err := e.Svc.Manage(ctx, "lido-altro", token); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("link on another establishment: %v", err)
		}
		// Revoked links.
		if _, err := db.NewUpdate().Model((*store.MagicLinkToken)(nil)).Set("revoked_at = ?", e.F.Clock.Now()).Where("booking_id = ?", s.BookingID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Svc.Manage(ctx, slug, token); !errors.Is(err, checkout.ErrLinkNotFound) {
			t.Fatalf("revoked link: %v", err)
		}
		// Expiry: end of the booking + 30 days.
		link, _ := e.Notify.IssueMagicLink(ctx, db, e.ES, s.BookingID, nil)
		if _, err := e.Svc.Manage(ctx, slug, link.Token); err != nil {
			t.Fatal(err)
		}
		e.F.Clock.Set(time.Date(2027, 8, 21, 0, 0, 1, 0, time.UTC))
		if _, err := e.Svc.Manage(ctx, slug, link.Token); !errors.Is(err, checkout.ErrLinkNotFound) {
			t.Fatalf("expired link: %v", err)
		}
	})
}

// TestManage_CancelRefundsThroughProvider: the preview shows the policy
// refund; the cancellation releases the spot and refunds the online
// payment through the gateway; a late cancellation pays a credit.
func TestManage_CancelRefundsThroughProvider(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		s, token := confirmed(e, e.F.Spots[1], "2027-07-20")
		paidOnline(e, s.BookingID, 2000)
		prev, err := e.Svc.ManageCancel(ctx, slug, token, nil, checkout.CancelInput{DryRun: true})
		if err != nil || prev.Result.Applied || prev.Result.Refund.Amount != 2000 || prev.Result.Refund.As != booking.SettleRefund || prev.RefundStatus != checkout.RefundProviderPending {
			t.Fatalf("preview %v %+v", err, prev)
		}
		stale := int64(1)
		if _, err := e.Svc.ManageCancel(ctx, slug, token, &stale, checkout.CancelInput{}); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale If-Match: %v", err)
		}
		out, err := e.Svc.ManageCancel(ctx, slug, token, nil, checkout.CancelInput{Reason: "cambio programma"})
		if err != nil || !out.Result.Applied || out.RefundStatus != checkout.RefundProviderPending || out.Managed.Booking.Booking.Status != "cancelled" {
			t.Fatalf("cancel %v %+v", err, out)
		}
		if len(e.Pay.Refunds) != 1 || e.Pay.Refunds[0].Amount != 2000 || e.Pay.Refunds[0].IdempotencyKey == "" {
			t.Fatalf("provider refunds %+v", e.Pay.Refunds)
		}
		if e.Count((*store.SpotAllocation)(nil), "booking_id = ?", s.BookingID) != 0 {
			t.Fatal("cancelled booking kept its spot")
		}
		if _, err := e.Svc.ManageCancel(ctx, slug, token, nil, checkout.CancelInput{}); !errors.Is(err, booking.ErrInvalidTransition) {
			t.Fatalf("second cancel: %v", err)
		}

		// 5 days before: 50% as credit (default policy).
		s2, token2 := confirmed(e, e.F.Spots[2], "2027-06-06")
		paidOnline(e, s2.BookingID, 2000)
		out, err = e.Svc.ManageCancel(ctx, slug, token2, nil, checkout.CancelInput{})
		if err != nil || out.RefundStatus != checkout.RefundCredited || out.Result.Refund.Amount != 1000 || out.Result.Recorded == nil {
			t.Fatalf("credit cancel %v %+v", err, out)
		}
		if len(e.Pay.Refunds) != 1 {
			t.Fatal("a credit must not refund through the provider")
		}
	})
}

// TestManage_ModifyWithinPolicy: a modification previews the new price
// and applies with re-allocation; outside the window it is a policy
// violation (dry run reports it, apply refuses it).
func TestManage_ModifyWithinPolicy(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		s, token := confirmed(e, e.F.Spots[0], "2027-07-20")
		two := checkout.Selection{
			StartsOn: calendar.MustParse("2027-07-20"), EndsOn: calendar.MustParse("2027-07-21"), Slots: calendar.SlotsFull, Formula: "multi_day",
			Lines: []booking.LineInput{{PackID: e.F.Base.ID, SpotID: e.F.Spots[3].ID}},
		}
		prev, err := e.Svc.ManageModify(ctx, slug, token, nil, checkout.ModifyInput{Selection: two, DryRun: true})
		if err != nil || prev.Result.Applied || prev.Result.NewTotal != 4000 || prev.Result.PriceDifference() != 2000 {
			t.Fatalf("preview %v %+v", err, prev)
		}
		if e.Booking(s.BookingID).TotalAmount != 2000 {
			t.Fatal("dry run wrote")
		}
		out, err := e.Svc.ManageModify(ctx, slug, token, nil, checkout.ModifyInput{Selection: two})
		if err != nil || !out.Result.Applied || out.Managed == nil || out.Managed.Booking.Booking.TotalAmount != 4000 {
			t.Fatalf("apply %v %+v", err, out)
		}
		if n := e.Count((*store.SpotAllocation)(nil), "booking_id = ? AND spot_id = ?", s.BookingID, e.F.Spots[3].ID); n != 4 {
			t.Fatalf("re-allocated rows %d", n)
		}
		// Two days before the start the default policy closes modifications.
		e.F.Clock.Set(time.Date(2027, 7, 19, 9, 0, 0, 0, time.UTC))
		prev, err = e.Svc.ManageModify(ctx, slug, token, nil, checkout.ModifyInput{Selection: e.Seat(e.F.Spots[3], "2027-07-20"), DryRun: true})
		if err != nil || len(prev.Violations) != 1 || prev.Violations[0].Code != booking.CodeModificationNotAllowed {
			t.Fatalf("late preview %v %+v", err, prev)
		}
		var pve *policy.ViolationError
		if _, err := e.Svc.ManageModify(ctx, slug, token, nil, checkout.ModifyInput{Selection: e.Seat(e.F.Spots[3], "2027-07-20")}); !errors.As(err, &pve) {
			t.Fatalf("late apply: %v", err)
		}
	})
}

// TestManage_RequestLinkNoEnumeration: always nil for well formed
// requests (match or not), work done in the background, limits per IP and
// per email + code.
func TestManage_RequestLinkNoEnumeration(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		var ve *configerr.ValidationError
		if err := e.Svc.RequestLink(ctx, slug, "not-an-email", "LD-2027-000001", "10.0.0.1"); !errors.As(err, &ve) {
			t.Fatalf("malformed email: %v", err)
		}
		for i, code := range []string{"LD-2027-000001", "LD-2027-999999", "ld-2027-000002"} {
			if err := e.Svc.RequestLink(ctx, slug, "Someone@Example.org", code, "10.0.0.1"); err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
		}
		e.Svc.Wait()
		// The requests run in the background: their order is not defined.
		got := slices.Sorted(slices.Values(e.Notify.Requests))
		want := []string{"someone@example.org|LD-2027-000001", "someone@example.org|LD-2027-000002", "someone@example.org|LD-2027-999999"}
		if !slices.Equal(got, want) {
			t.Fatalf("requests %v", got)
		}
		var rl *auth.RateLimitError
		for i := 0; ; i++ {
			err := e.Svc.RequestLink(ctx, slug, "x@example.org", "LD-2027-000003", "10.0.0.1")
			if errors.As(err, &rl) {
				if i != checkout.DefaultLinkRequestsPerIP-3 {
					t.Fatalf("limited after %d requests", i)
				}
				break
			}
			if err != nil || i > 10 {
				t.Fatalf("request %d: %v", i, err)
			}
		}
		// The per booking limit binds across IPs.
		for i := range checkout.DefaultLinkRequestsPerBooking {
			if err := e.Svc.RequestLink(ctx, slug, "y@example.org", "LD-2027-000004", "10.0.1."+string(rune('1'+i))); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.Svc.RequestLink(ctx, slug, "y@example.org", "LD-2027-000004", "10.0.2.1"); !errors.As(err, &rl) {
			t.Fatalf("per booking limit: %v", err)
		}
		e.Svc.Wait()
	})
}

// TestManage_SeasonPassRelease: a pass holder previews and releases a day
// from the manage area (policy customer_can_release_season_days); the
// preview writes nothing, the release frees the spot for resale.
func TestManage_SeasonPassRelease(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		in := e.F.Input(e.F.Spots[1], "2027-07-01", "2027-07-10")
		in.IsSeasonPass, in.AllocationKind = true, "season"
		d, err := e.Book.Create(ctx, e.F.Owner, e.F.Est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		link, err := e.Notify.IssueMagicLink(ctx, db, e.ES, d.Booking.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		m, err := e.Svc.Manage(ctx, slug, link.Token)
		if err != nil || !m.Allowed.ReleaseDays || m.Allowed.Modify {
			t.Fatalf("pass actions %v %+v", err, m.Allowed)
		}
		day := checkout.ReleaseDay{Date: calendar.MustParse("2027-07-05"), Slots: calendar.SlotsFull}
		prev, err := e.Svc.ManageRelease(ctx, slug, link.Token, checkout.ReleaseInput{Days: []checkout.ReleaseDay{day}, DryRun: true})
		if err != nil || prev.Applied || len(prev.Released) != 2 {
			t.Fatalf("preview %v %+v", err, prev)
		}
		if n := e.Count((*store.SeasonRelease)(nil), ""); n != 0 {
			t.Fatalf("preview wrote %d releases", n)
		}
		out, err := e.Svc.ManageRelease(ctx, slug, link.Token, checkout.ReleaseInput{Days: []checkout.ReleaseDay{day}})
		if err != nil || !out.Applied || len(out.Released) != 2 || out.Managed == nil || len(out.Managed.Releases) != 2 {
			t.Fatalf("release %v %+v", err, out)
		}
		if n := e.Count((*store.SpotAllocation)(nil), "booking_id = ? AND date = ?", d.Booking.ID, calendar.MustParse("2027-07-05")); n != 0 {
			t.Fatalf("released day still allocated: %d", n)
		}
		if _, err := e.Svc.ManageRelease(ctx, slug, link.Token, checkout.ReleaseInput{Days: []checkout.ReleaseDay{day}}); err == nil {
			t.Fatal("double release accepted")
		}
	})
}
