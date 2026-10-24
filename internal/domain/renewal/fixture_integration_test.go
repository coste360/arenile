//go:build integration

package renewal_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/domain/rollover"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// fx is the renewal cycle on the real booking, inventory, pricing,
// customer and checkout services (checkouttest): a 2027 season with
// season passes, rolled over to 2028, plus the renewal service with an
// in-memory notifier. Now is 2027-10-01 (after the season).
type fx struct {
	*checkouttest.Env
	t       *testing.T
	ctx     context.Context
	ro      *rollover.Service
	rn      *renewal.Service
	notes   *notes
	target  *store.Season
	passes  []*booking.Detail
	endedAt time.Time
}

// deadline is the pre-emption deadline of the test campaigns.
var deadline = time.Date(2028, 3, 31, 20, 0, 0, 0, time.UTC)

func newFx(t *testing.T, db *store.DB, passes int) *fx {
	t.Helper()
	e := checkouttest.New(t, db)
	f := e.F
	x := &fx{Env: e, t: t, ctx: e.Ctx, notes: &notes{}}
	// The base pack is sold as a season pass too.
	f.Base.AllowedFormulas = append(f.Base.AllowedFormulas, "season")
	if _, err := db.NewUpdate().Model(f.Base).Column("allowed_formulas").WherePK().Exec(x.ctx); err != nil {
		t.Fatal(err)
	}
	for i := range passes {
		d, err := e.Book.Create(x.ctx, f.Owner, f.Est.ID, booking.CreateInput{
			Customer: &customer.Input{
				FirstName: fmt.Sprintf("Cliente%d", i), LastName: "Abbonato", Email: fmt.Sprintf("abbonato%d@example.org", i), Locale: "it",
			},
			StartsOn: calendar.MustParse("2027-06-01"), EndsOn: f.Season.ClosesOn, Slots: calendar.SlotsFull, Formula: "season",
			Lines:    []booking.LineInput{{PackID: f.Base.ID, SpotID: f.Spots[i].ID}},
			SeasonID: f.Season.ID, IsSeasonPass: true, AllocationKind: inventory.KindSeason,
		})
		if err != nil {
			t.Fatalf("season pass %d: %v", i, err)
		}
		x.passes = append(x.passes, d)
	}
	f.Clock.Set(time.Date(2027, 10, 1, 8, 0, 0, 0, time.UTC))
	x.endedAt = f.Clock.Now()
	x.ro = rollover.NewService(rollover.Deps{DB: db, Clock: f.Clock, Copier: f.Pricing})
	res, created, err := x.ro.Rollover(x.ctx, f.Owner, f.Est.ID, f.Season.ID, rollover.Input{
		TargetYear: 2028, IncludeRatePeriods: true, IncludeClosureDays: true, IncludePriceRules: true, IncludeFixedPrices: true, IncludePolicies: true,
	})
	if err != nil || !created {
		t.Fatalf("rollover: %v created=%v", err, created)
	}
	x.target = res.Season
	x.rn = renewal.NewService(renewal.Deps{
		DB: db, Clock: f.Clock, Bookings: e.Book, Checkout: e.Svc, Notify: x.notes, Credits: e.Credits, CreditApplier: e.Credits,
	})
	return x
}

// campaign creates the campaign of every pass with the test deadline.
func (x *fx) campaign() *renewal.CreateResult {
	x.t.Helper()
	res, err := x.rn.CreateCampaign(x.ctx, x.F.Owner, x.F.Est.ID, renewal.CreateInput{
		SourceSeasonID: x.F.Season.ID, TargetSeasonID: x.target.ID, DeadlineAt: deadline,
	})
	if err != nil {
		x.t.Fatalf("campaign: %v", err)
	}
	return res
}

// enableOnline configures the (fake) online provider.
func (x *fx) enableOnline() {
	x.t.Helper()
	sk, wh := "sk_live_51abcdefghijklmnopqrstuv", "whsec_abcdefghijklmn"
	if _, err := x.Online.UpdatePayments(x.ctx, x.F.Owner, x.F.Est.ID, nil, online.PaymentSettingsInput{
		Provider: "stripe", Mode: "live", Stripe: &online.StripeInput{SecretKey: &sk, WebhookSecret: &wh},
	}); err != nil {
		x.t.Fatal(err)
	}
}

// link issues a magic link of the booking (as notify does for the email).
func (x *fx) link(bookingID string) string {
	x.t.Helper()
	var tok string
	err := store.RunInTx(x.ctx, x.DB, func(ctx context.Context, tx bun.Tx) error {
		l, err := x.Notify.IssueMagicLink(ctx, tx, x.ES, bookingID, nil)
		if err == nil {
			tok = l.Token
		}
		return err
	})
	if err != nil {
		x.t.Fatal(err)
	}
	return tok
}

// sale is a desk booking of spot on day for a new customer (forced: the
// default policy limits the lead time and the target season is months
// away; the allocation constraint still decides).
func (x *fx) sale(spot *store.Spot, day string) error {
	in := x.F.Input(spot, day, day)
	in.Force = true
	_, err := x.Book.Create(x.ctx, x.F.Owner, x.F.Est.ID, in)
	return err
}

// grant gives amount of goodwill credit to a customer.
func (x *fx) grant(customerID string, amount money.Cents) {
	x.t.Helper()
	if err := store.RunInTx(x.ctx, x.DB, func(ctx context.Context, tx bun.Tx) error {
		return x.Credits.GrantCredit(ctx, tx, x.ES, &store.CustomerCredit{CustomerID: customerID, Amount: amount, Source: store.CreditSourceGoodwill})
	}); err != nil {
		x.t.Fatal(err)
	}
}

// balance is the stored credit balance of a customer.
func (x *fx) balance(customerID string) money.Cents {
	x.t.Helper()
	bal, err := store.GetCustomerCreditBalance(x.ctx, x.DB, x.ES, customerID)
	if err != nil {
		x.t.Fatal(err)
	}
	return bal
}

func (x *fx) proposal(id string) *store.RenewalProposal {
	x.t.Helper()
	p, err := store.GetRenewalProposal(x.ctx, x.DB, x.ES, id)
	if err != nil {
		x.t.Fatal(err)
	}
	return p
}

// notes records the queued emails.
type notes struct {
	mu   sync.Mutex
	sent []notify.Enqueue
}

func (n *notes) Enqueue(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, m notify.Enqueue) (*store.Message, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, m)
	return &store.Message{ID: store.NewID(), Template: m.Template, DedupeKey: m.DedupeKey}, nil
}
