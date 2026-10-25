//go:build integration

package setup

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// demoFakes records the use cases the demo calls. Customers are written to
// the database (the demo decides idempotency on them); the other ports only
// record, so this test checks the orchestration independently of the M3
// services (their own tests and internal/acceptance cover the real ones).
type demoFakes struct {
	mu        sync.Mutex
	customers []store.Customer
	creates   []booking.CreateInput
	releases  map[string][]calendar.Date
	payments  []DemoPayment
	opened    []string
	closed    map[string]store.MethodTotals
	cancelled []string
	checkedIn []string
	notImpl   bool
}

func (f *demoFakes) Resolve(ctx context.Context, tx bun.IDB, _ *auth.Principal, es store.EstablishmentScope, _ string, in *customer.Input) (*store.Customer, error) {
	if f.notImpl {
		return nil, configerr.ErrNotImplemented
	}
	now := t0
	email, phone := in.Email, in.Phone
	c := &store.Customer{
		ID: store.NewID(), EstablishmentID: es.ID(), FirstName: in.FirstName, LastName: in.LastName,
		Email: &email, Phone: &phone, Locale: in.Locale, Notes: in.Notes, Tags: in.Tags,
		SearchText: strings.ToLower(in.FirstName + " " + in.LastName), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.NewInsert().Model(c).Exec(ctx); err != nil {
		return nil, err
	}
	f.customers = append(f.customers, *c)
	return c, nil
}

func (f *demoFakes) CreateInTx(_ context.Context, _ bun.IDB, _ *auth.Principal, _ store.EstablishmentScope, in booking.CreateInput) (*booking.Detail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, in)
	status := in.Status
	return &booking.Detail{Booking: store.Booking{
		ID: store.NewID(), Status: string(status), StartsOn: in.StartsOn, EndsOn: in.EndsOn, TotalAmount: money.Cents(3000 + 100*len(f.creates)),
	}}, nil
}

func (f *demoFakes) ops(p *auth.Principal) DemoOperations {
	return DemoOperations{
		Actor: p, Customers: f, Bookings: f,
		ReleaseDays: func(_ context.Context, _ *auth.Principal, _, passID string, dates []calendar.Date) error {
			f.releases[passID] = dates
			return nil
		},
		OpenCash: func(context.Context, *auth.Principal, string, money.Cents) (string, error) {
			id := store.NewID()
			f.opened = append(f.opened, id)
			return id, nil
		},
		RecordPayment: func(_ context.Context, _ *auth.Principal, _ string, pay DemoPayment) error {
			f.payments = append(f.payments, pay)
			return nil
		},
		CloseCash: func(_ context.Context, _ *auth.Principal, _, id string, counted store.MethodTotals) error {
			f.closed[id] = counted
			return nil
		},
		CancelBooking: func(_ context.Context, _ *auth.Principal, _, id, _ string) error {
			f.cancelled = append(f.cancelled, id)
			return nil
		},
		CheckIn: func(_ context.Context, _ *auth.Principal, _, id string) error {
			f.checkedIn = append(f.checkedIn, id)
			return nil
		},
	}
}

func newDemoFakes() *demoFakes {
	return &demoFakes{releases: map[string][]calendar.Date{}, closed: map[string]store.MethodTotals{}}
}

// TestSeedDemoOperations_VolumesVATAndIdempotency runs the M3 demo on the
// configured demo establishment: VAT 22%, fake customers, season passes on
// rows A-B, bookings without overlaps, releases, two cash sessions (one
// closed) and a second run that changes nothing.
func TestSeedDemoOperations_VolumesVATAndIdempotency(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db, "demo-m3")
		if _, err := f.svc.SeedDemo(ctx, f.est.ID); err != nil {
			t.Fatal(err)
		}
		fk := newDemoFakes()
		res, err := f.svc.SeedDemoOperations(ctx, f.est.ID, fk.ops(f.owner()))
		if err != nil {
			t.Fatal(err)
		}
		if !res.Applied || len(res.Skipped) != 0 {
			t.Fatalf("result %+v", res)
		}

		// VAT 22% on the establishment default and on every pack and extra.
		es, err := store.SystemEstablishmentScope(ctx, db, f.est.ID)
		if err != nil {
			t.Fatal(err)
		}
		est, _ := store.GetEstablishment(ctx, db, es)
		settings, _ := establishment.ParseSettings(est.Settings, est.Currency)
		if settings.Pricing.DefaultVATRateBP != DemoVATRateBP {
			t.Fatalf("default vat %d", settings.Pricing.DefaultVATRateBP)
		}
		packs, _ := store.ListAllPacks(ctx, db, es)
		extras, _ := store.ListAllExtras(ctx, db, es)
		for _, p := range packs {
			if p.VATRateBP == nil || *p.VATRateBP != DemoVATRateBP {
				t.Fatalf("pack %s vat %v", p.Code, p.VATRateBP)
			}
		}
		for _, x := range extras {
			if x.VATRateBP == nil || *x.VATRateBP != DemoVATRateBP {
				t.Fatalf("extra %s vat %v", x.Code, x.VATRateBP)
			}
		}

		// Fake people only.
		if res.Customers != demoCustomers || len(fk.customers) != demoCustomers {
			t.Fatalf("customers %d", res.Customers)
		}
		seenEmail := map[string]bool{}
		for _, c := range fk.customers {
			if !strings.HasSuffix(*c.Email, "@example.com") || !strings.HasPrefix(*c.Phone, "+39000000") || seenEmail[*c.Email] {
				t.Fatalf("customer contact %q %q", *c.Email, *c.Phone)
			}
			seenEmail[*c.Email] = true
		}

		// Passes: season, whole season, distinct spots; bookings: no spot
		// used twice on the same day (drafts hold nothing).
		var passes, held, drafts, confirmed int
		used := map[string]map[calendar.Date]bool{}
		year, _, _ := demoSeason(calendar.In(t0, mustLoc(t)))
		season, _ := store.GetSeasonByYear(ctx, db, es, year)
		for _, in := range fk.creates {
			if in.IsSeasonPass {
				passes++
				if in.AllocationKind != inventory.KindSeason || !in.StartsOn.Equal(season.OpensOn) || !in.EndsOn.Equal(season.ClosesOn) || in.SeasonID != season.ID {
					t.Fatalf("pass input %+v", in)
				}
			}
			switch in.Status {
			case booking.StatusHeld:
				held++
				if in.HoldTTL <= 0 {
					t.Fatal("held demo booking without ttl")
				}
			case booking.StatusDraft:
				drafts++
				continue
			case booking.StatusConfirmed:
				confirmed++
			}
			if !in.IsSeasonPass && (in.StartsOn.Before(season.OpensOn) || in.EndsOn.After(season.ClosesOn)) {
				t.Fatalf("booking outside the season: %s..%s", in.StartsOn, in.EndsOn)
			}
			if in.IsSeasonPass {
				continue
			}
			spot := in.Lines[0].SpotID
			if used[spot] == nil {
				used[spot] = map[calendar.Date]bool{}
			}
			for d := in.StartsOn; !d.After(in.EndsOn); d = d.AddDays(1) {
				if used[spot][d] && !isReleasedDay(fk, d) {
					t.Fatalf("spot %s booked twice on %s", spot, d)
				}
				used[spot][d] = true
			}
		}
		if passes != demoSeasonPasses || res.SeasonPasses != demoSeasonPasses {
			t.Fatalf("passes %d", passes)
		}
		if res.Bookings < 60 || held == 0 || drafts == 0 || confirmed < 50 {
			t.Fatalf("bookings %d held %d drafts %d confirmed %d", res.Bookings, held, drafts, confirmed)
		}
		if len(fk.releases) != demoReleasedPasses || res.Releases != 3*demoReleasedPasses || res.Resales != 2 {
			t.Fatalf("releases %+v resales %d", fk.releases, res.Resales)
		}

		// Cash: two sessions, the first closed with counted totals; every
		// payment belongs to one of them.
		if len(fk.opened) != 2 || len(fk.closed) != 1 || fk.closed[fk.opened[0]] == nil {
			t.Fatalf("cash sessions opened %v closed %v", fk.opened, fk.closed)
		}
		var cashInClosed money.Cents
		for _, p := range fk.payments {
			if p.CashSessionID != fk.opened[0] && p.CashSessionID != fk.opened[1] {
				t.Fatalf("payment outside sessions %+v", p)
			}
			if p.Amount <= 0 || (p.Method != store.PaymentMethodCash && p.Method != store.PaymentMethodCard) {
				t.Fatalf("payment %+v", p)
			}
			if p.CashSessionID == fk.opened[0] && p.Method == store.PaymentMethodCash {
				cashInClosed += p.Amount
			}
		}
		if got := fk.closed[fk.opened[0]][store.PaymentMethodCash]; got != 10000+cashInClosed-200 {
			t.Fatalf("counted cash %d, expected float + cash payments - 200 = %d", got, 10000+cashInClosed-200)
		}
		if res.Payments != len(fk.payments) || res.Payments == 0 || res.Cancelled == 0 {
			t.Fatalf("payments %d cancelled %d", res.Payments, res.Cancelled)
		}

		// Second run: no new customer, booking or payment.
		fk2 := newDemoFakes()
		again, err := f.svc.SeedDemoOperations(ctx, f.est.ID, fk2.ops(f.owner()))
		if err != nil {
			t.Fatal(err)
		}
		if again.Applied || len(fk2.creates) != 0 || len(fk2.payments) != 0 || again.VATUpdated != 0 {
			t.Fatalf("second run changed data: %+v", again)
		}
	})
}

// TestSeedDemoOperations_SkipsMissingUseCases keeps the CLI working while
// some M3 use cases are not wired or not implemented.
func TestSeedDemoOperations_SkipsMissingUseCases(t *testing.T) {
	ctx := context.Background()
	db := storetest.SQLite(t)
	f := newFixture(t, db, "demo-skip")
	if _, err := f.svc.SeedDemo(ctx, f.est.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SeedDemoOperations(ctx, f.est.ID, DemoOperations{}); !errors.Is(err, configerr.ErrValidation) {
		t.Fatalf("missing ports: %v", err)
	}

	fk := newDemoFakes()
	fk.notImpl = true
	res, err := f.svc.SeedDemoOperations(ctx, f.est.ID, fk.ops(f.owner()))
	if err != nil || res.Applied || len(res.Skipped) != 1 {
		t.Fatalf("customers not implemented: %+v %v", res, err)
	}

	fk = newDemoFakes()
	ops := fk.ops(f.owner())
	ops.ReleaseDays, ops.OpenCash, ops.RecordPayment, ops.CloseCash, ops.CancelBooking, ops.CheckIn = nil, nil, nil, nil, nil, nil
	res, err = f.svc.SeedDemoOperations(ctx, f.est.ID, ops)
	if err != nil || !res.Applied || res.Bookings == 0 || len(res.Skipped) != 3 {
		t.Fatalf("partial ports: %+v %v", res, err)
	}
}

func isReleasedDay(fk *demoFakes, d calendar.Date) bool {
	for _, dates := range fk.releases {
		for _, x := range dates {
			if x.Equal(d) {
				return true
			}
		}
	}
	return false
}

func mustLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
