//go:build integration

package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// TestAcceptance_SeasonCycle is the M5 acceptance (spec 28: "ciclo completo
// di fine stagione -> nuova stagione (copia configurazione, proposta
// rinnovi, conferme) eseguito senza interventi manuali sul database"),
// driven only through the admin and public APIs, the signed Stripe webhook
// and the background jobs, with the fake clock:
//
//  1. six season passes are sold during the 2027 season;
//  2. after the season the rollover is previewed, applied (201) and applied
//     again (200, already_applied): the 2028 season has the price list;
//  3. the renewal campaign is previewed and created: six held season
//     passes of 2028 on the same spots, priced by the new list; the spots
//     cannot be sold to anyone else; the proposals are emailed in bulk;
//  4. outcomes: one accepted at the desk, one accepted online from the
//     magic link and confirmed by the Stripe webhook, one declined online,
//     one declined at the desk, two without an answer that expire at the
//     deadline (jobs) and go back on sale;
//  5. the new season is consistent: two confirmed renewals with season
//     allocations, every other spot free, no open proposal or held
//     booking left, campaign counts and outbox events agree.
func TestAcceptance_SeasonCycle(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ctx := context.Background()
		c := newClient(t, e.srv.URL)
		c.login(ownerEmail, ownerPassword)
		pub := newClient(t, e.srv.URL)

		// Online payments with the fake Stripe.
		r := c.must(c.get("getPaymentSettings", e.path("/payment-settings")), "payment settings")
		sk, wh, test := stripetest.SecretKey, stripetest.WebhookSecret, api.Test
		c.must(c.do("updatePaymentSettings", http.MethodPut, e.path("/payment-settings"), api.PaymentSettingsInput{
			Provider: "stripe", Mode: test, Stripe: &api.StripeSettingsInput{SecretKey: &sk, WebhookSecret: &wh},
		}, map[string]string{"If-Match": r.ETag}), "stripe in test mode")

		// 1. Season passes of 2027.
		const n = 6
		passIDs := make([]string, n)
		for i := range n {
			first, last := fmt.Sprintf("Abbonato%02d", i), "Rinnovo"
			email := openapi_types.Email(fmt.Sprintf("abbonato%02d@example.com", i))
			cr := c.must(c.post("createCustomer", e.path("/customers"), api.CustomerInput{FirstName: &first, LastName: last, Email: &email}), "customer")
			cust := decode[api.Customer](t, cr).Id
			pr := c.must(c.post("createSeasonPass", e.path("/season-passes"), api.SeasonPassCreate{
				CustomerId: cust, PackId: e.basePack.ID, SeasonId: e.season.ID, SpotId: e.passSpots[i].ID,
			}), "season pass")
			passIDs[i] = decode[api.SeasonPassDetail](t, pr).Pass.Id
		}

		// End of the season.
		e.clock.Set(time.Date(2027, 10, 5, 7, 0, 0, 0, time.UTC))
		e.runJobs()
		c.login(ownerEmail, ownerPassword) // months later: a new session

		// 2. Rollover: preview, apply, apply again.
		dry := true
		rp := c.must(c.post("rolloverSeason", e.path("/seasons/"+e.season.ID+"/rollover"), api.SeasonRolloverRequest{TargetYear: 2028, DryRun: &dry}), "rollover preview")
		preview := decode[api.SeasonRolloverResult](t, rp)
		if rp.Status != http.StatusOK || !preview.DryRun || preview.Season != nil || preview.Counts.PriceRules == 0 || preview.SeasonPasses == nil || *preview.SeasonPasses != n {
			t.Fatalf("rollover preview: %s", rp)
		}
		dry = false
		ra := c.must(c.post("rolloverSeason", e.path("/seasons/"+e.season.ID+"/rollover"), api.SeasonRolloverRequest{TargetYear: 2028, DryRun: &dry}), "rollover")
		applied := decode[api.SeasonRolloverResult](t, ra)
		if ra.Status != http.StatusCreated || applied.Season == nil || applied.Counts != preview.Counts || applied.RolloverId == nil {
			t.Fatalf("rollover apply: %s", ra)
		}
		rr := c.must(c.post("rolloverSeason", e.path("/seasons/"+e.season.ID+"/rollover"), api.SeasonRolloverRequest{TargetYear: 2028, DryRun: &dry}), "rollover again")
		if again := decode[api.SeasonRolloverResult](t, rr); rr.Status != http.StatusOK || !again.AlreadyApplied || *again.RolloverId != *applied.RolloverId {
			t.Fatalf("rollover again: %s", rr)
		}
		if l := decode[api.SeasonRolloverList](t, c.must(c.get("listSeasonRollovers", e.path("/season-rollovers")), "rollovers")); len(l.Items) != 1 {
			t.Fatalf("rollovers: %+v", l)
		}
		target := applied.Season

		// 3. Renewal campaign.
		deadline := time.Date(2028, 3, 31, 20, 0, 0, 0, time.UTC)
		cp := c.must(c.post("createRenewalCampaign", e.path("/renewal-campaigns"), api.RenewalCampaignCreate{
			SourceSeasonId: e.season.ID, TargetSeasonId: target.Id, DeadlineAt: deadline, DryRun: &[]bool{true}[0],
		}), "campaign preview")
		if p := decode[api.RenewalCampaignResult](t, cp); cp.Status != http.StatusOK || len(p.Proposals) != n || len(p.Skipped) != 0 {
			t.Fatalf("campaign preview: %s", cp)
		}
		cc := c.must(c.post("createRenewalCampaign", e.path("/renewal-campaigns"), api.RenewalCampaignCreate{
			SourceSeasonId: e.season.ID, TargetSeasonId: target.Id, DeadlineAt: deadline,
		}), "campaign")
		created := decode[api.RenewalCampaignResult](t, cc)
		if cc.Status != http.StatusCreated || created.Campaign == nil || len(created.Proposals) != n {
			t.Fatalf("campaign: %s", cc)
		}
		campaignID := created.Campaign.Id
		pl := decode[api.RenewalProposalList](t, c.must(c.get("listRenewalProposals", e.path("/renewal-campaigns/"+campaignID+"/proposals")), "proposals"))
		if len(pl.Items) != n {
			t.Fatalf("proposals: %d", len(pl.Items))
		}
		props := map[string]api.RenewalProposal{} // by source pass
		for _, p := range pl.Items {
			if p.Status != api.RenewalProposalStatusProposed || p.BookingStatus == nil || *p.BookingStatus != api.BookingStatusHeld ||
				!p.DeadlineAt.Equal(deadline) || p.StartsOn != target.OpensOn || p.TotalAmount <= 0 {
				t.Fatalf("proposal: %+v", p)
			}
			props[p.SourcePassId] = p
		}
		day := calendar.MustParse("2028-07-15")
		saleOf := func(spotID string) result {
			force := true
			return c.post("createBooking", e.path("/bookings"), api.BookingCreate{
				Customer: &api.CustomerInput{LastName: "Passante"}, StartsOn: day, EndsOn: day, Slots: api.SlotsFULL, Formula: api.Formula("day"),
				Lines: []api.BookingLineInput{{PackId: e.basePack.ID, SpotId: &spotID}}, Force: &force,
			})
		}
		for i := range n {
			if r := saleOf(e.passSpots[i].ID); r.Status != http.StatusConflict || r.Problem.Type != "spot_unavailable" {
				t.Fatalf("sale of proposed spot %d: %s", i, r)
			}
		}
		bs := c.must(c.post("bulkRenewalProposals", e.path("/renewal-campaigns/"+campaignID+"/proposals/bulk"), api.RenewalBulkRequest{
			Action: api.RenewalBulkActionSend, Status: ptrTo(api.RenewalProposalStatusProposed),
		}), "bulk send")
		if b := decode[api.RenewalBulkResult](t, bs); b.Succeeded != n || len(b.Failed) != 0 {
			t.Fatalf("bulk send: %s", bs)
		}
		e.runJobs()
		var queued int64
		queued, err := e.db.NewSelect().Model((*store.Message)(nil)).Where("establishment_id = ?", e.estID).Where("template = ?", "renewal_proposal").Count(ctx)
		if err != nil || queued != int64(n) {
			t.Fatalf("renewal emails: %d %v", queued, err)
		}

		// 4. Outcomes.
		p0, p1, p2, p3 := props[passIDs[0]], props[passIDs[1]], props[passIDs[2]], props[passIDs[3]]
		// Desk acceptance, confirmed now (manual driver).
		ar := c.must(c.do("acceptRenewalProposal", http.MethodPost, e.path("/renewal-proposals/"+p0.Id+"/accept"), api.RenewalStaffAcceptRequest{}, ifMatch(p0.Version+1)), "desk accept")
		if a := decode[api.RenewalProposal](t, ar); a.Status != api.RenewalProposalStatusConverted {
			t.Fatalf("desk accept: %s", ar)
		}
		// Online acceptance from the magic link, confirmed by the webhook.
		token := manageToken(t, c, e, p1.BookingId)
		gr := pub.must(pub.get("getManagedRenewal", pubPathOf("/manage/"+token+"/renewal")), "public renewal")
		view := decode[api.PublicRenewalProposal](t, gr)
		if !view.Allowed.Accept || !view.Allowed.PayFull || view.SeasonYear != 2028 {
			t.Fatalf("public renewal: %s", gr)
		}
		mb := decode[api.ManageBooking](t, pub.must(pub.get("getManagedBooking", pubPathOf("/manage/"+token)), "manage view"))
		if mb.Renewal == nil || mb.Renewal.Status != api.RenewalProposalStatusProposed {
			t.Fatalf("manage view without the renewal: %+v", mb.Renewal)
		}
		terms := ""
		if view.TermsVersion != nil {
			terms = *view.TermsVersion
		}
		acc := pub.must(pub.post("acceptManagedRenewal", pubPathOf("/manage/"+token+"/renewal/accept"), api.PublicRenewalAcceptRequest{
			Payment: api.PublicRenewalAcceptRequestPaymentFull, AcceptTermsVersion: terms,
		}), "online accept")
		out := decode[api.PublicRenewalAcceptResult](t, acc)
		if out.Pay.Mode != api.PayResultModeRedirect || out.Pay.RedirectUrl == nil || strings.Contains(*out.Pay.RedirectUrl, token) {
			t.Fatalf("online accept: %s", acc)
		}
		ref := (*out.Pay.RedirectUrl)[strings.LastIndex(*out.Pay.RedirectUrl, "/")+1:]
		e.runJobs()
		if got := decode[api.RenewalProposal](t, c.must(c.get("getRenewalProposal", e.path("/renewal-proposals/"+p1.Id)), "p1")); got.Status != api.RenewalProposalStatusAccepted {
			t.Fatalf("renewal converted before the webhook: %s", got.Status)
		}
		if wr := stripeWebhook(t, e, ref, "evt_renewal_"+ref); wr.Status != http.StatusOK {
			t.Fatalf("webhook: %s", wr)
		}
		e.runJobs()
		// Declined online and at the desk.
		token2 := manageToken(t, c, e, p2.BookingId)
		pub.must(pub.post("declineManagedRenewal", pubPathOf("/manage/"+token2+"/renewal/decline"), api.PublicRenewalDeclineRequest{}), "online decline")
		c.must(c.do("declineRenewalProposal", http.MethodPost, e.path("/renewal-proposals/"+p3.Id+"/decline"), api.RenewalDeclineRequest{}, nil), "desk decline")
		for _, i := range []int{2, 3} {
			if r := saleOf(e.passSpots[i].ID); !r.ok() {
				t.Fatalf("sale after decline %d: %s", i, r)
			}
		}
		// No answer: the deadline passes, the jobs release the spots.
		e.clock.Set(deadline.Add(2 * time.Minute))
		e.runJobs()
		c.login(ownerEmail, ownerPassword)
		for _, i := range []int{4, 5} {
			if r := saleOf(e.passSpots[i].ID); !r.ok() {
				t.Fatalf("sale after the deadline %d: %s", i, r)
			}
		}

		// 5. The new season is consistent.
		cg := decode[api.RenewalCampaign](t, c.must(c.get("getRenewalCampaign", e.path("/renewal-campaigns/"+campaignID)), "campaign"))
		if cg.Counts != (api.RenewalStatusCounts{Converted: 2, Declined: 2, Expired: 2, Sent: n}) {
			t.Fatalf("campaign counts: %+v", cg.Counts)
		}
		want := map[string]string{
			passIDs[0]: "converted", passIDs[1]: "converted", passIDs[2]: "declined", passIDs[3]: "declined",
			passIDs[4]: "expired", passIDs[5]: "expired",
		}
		for _, p := range decode[api.RenewalProposalList](t, c.must(c.get("listRenewalProposals", e.path("/renewal-campaigns/"+campaignID+"/proposals")), "proposals")).Items {
			if string(p.Status) != want[p.SourcePassId] {
				t.Errorf("proposal of %s: %s, want %s", p.SourcePassId, p.Status, want[p.SourcePassId])
			}
		}
		var renewed []store.Booking
		if err := e.db.NewSelect().Model(&renewed).Where("establishment_id = ?", e.estID).Where("season_id = ?", target.Id).
			Where("is_season_pass = ?", true).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		confirmed := 0
		for _, b := range renewed {
			switch b.Status {
			case "confirmed":
				confirmed++
				if b.RenewalOfBookingID == nil || (want[*b.RenewalOfBookingID] != "converted") {
					t.Errorf("confirmed 2028 pass %s renews %v", b.Code, b.RenewalOfBookingID)
				}
				var kinds []string
				if err := e.db.NewSelect().Model((*store.SpotAllocation)(nil)).Column("kind").Where("booking_id = ?", b.ID).Distinct().Scan(ctx, &kinds); err != nil ||
					len(kinds) != 1 || kinds[0] != "season" {
					t.Errorf("allocations of %s: %v %v", b.Code, kinds, err)
				}
			case "held", "pending_payment":
				t.Errorf("open 2028 pass left: %s %s", b.Code, b.Status)
			}
		}
		if confirmed != 2 {
			t.Fatalf("confirmed renewals: %d", confirmed)
		}
		for typ, wantN := range map[string]int{
			"season.rolled_over": 1, "season.renewal_offered": n, "season.renewal_accepted": 2,
			"season.renewal_declined": 2, "season.renewal_expired": 2,
		} {
			got, err := e.db.NewSelect().Model((*store.OutboxEvent)(nil)).Where("establishment_id = ?", e.estID).Where("type = ?", typ).Count(ctx)
			if err != nil || got != int64(wantN) {
				t.Errorf("outbox %s: %d, want %d (%v)", typ, got, wantN, err)
			}
		}
		c.failIfNotImplemented()
		pub.failIfNotImplemented()
	})
}

func ptrTo[T any](v T) *T { return &v }

// pubPathOf is a public API path of the demo establishment.
func pubPathOf(suffix string) string { return "/api/public/v1/" + demoSlug + suffix }

// manageToken regenerates the magic link of a booking from the panel (as
// the desk shares it) and returns its token.
func manageToken(t *testing.T, c *apiClient, e *env, bookingID string) string {
	t.Helper()
	no := false
	r := c.must(c.post("regenerateMagicLink", e.path("/bookings/"+bookingID+"/magic-link"), api.MagicLinkRequest{SendEmail: &no}), "magic link")
	res := decode[api.MagicLinkResult](t, r)
	if res.ManageUrl == nil {
		t.Fatalf("magic link without url: %s", r)
	}
	return (*res.ManageUrl)[strings.LastIndex(*res.ManageUrl, "/")+1:]
}

// stripeWebhook completes a Checkout Session at the fake Stripe and
// delivers the signed checkout.session.completed event.
func stripeWebhook(t *testing.T, e *env, ref, eventID string) result {
	t.Helper()
	e.stripe.Pay(ref)
	now := e.clock.Now()
	payload := e.stripe.SessionEvent(eventID, "checkout.session.completed", ref, now)
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/webhooks/payments/stripe/"+e.estID, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", stripetest.Sign(payload, now, stripetest.WebhookSecret))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result{Status: -1, Body: []byte(err.Error())}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return result{Status: resp.StatusCode, Body: raw}
}
