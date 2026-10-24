//go:build integration

package gateway_test

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app/builtin"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// settings is the test double of *online.Service (gateway.ProviderSettings):
// Stripe in test mode with the fake's credentials for every establishment.
type settings struct {
	manual   atomic.Bool
	embedded bool
	// twoModes adds a live configuration (liveWebhookSecret) next to the
	// test one: real bookings pay through it, test bookings through the
	// test row (spec 15.4).
	twoModes bool
}

// liveWebhookSecret signs the deliveries of the live configuration.
const liveWebhookSecret = "whsec_arenile_live_secret" //nolint:gosec // fake test credential

func (s *settings) cfg(es store.EstablishmentScope) *online.ProviderConfig {
	return &online.ProviderConfig{
		EstablishmentID: es.ID(), Code: stripe.Code, Mode: store.ProviderModeTest, Online: true, Embedded: s.embedded,
		PublishableKey: stripetest.PublishableKey,
		Credentials:    ext.Credentials{stripe.CredSecretKey: stripetest.SecretKey, stripe.CredWebhookSecret: stripetest.WebhookSecret},
	}
}

func (s *settings) live(es store.EstablishmentScope) *online.ProviderConfig {
	c := s.cfg(es)
	c.Mode = store.ProviderModeLive
	c.Credentials = ext.Credentials{stripe.CredSecretKey: stripetest.SecretKey, stripe.CredWebhookSecret: liveWebhookSecret}
	return c
}

func (s *settings) ActiveProvider(_ context.Context, _ bun.IDB, es store.EstablishmentScope, test bool) (*online.ProviderConfig, error) {
	if s.manual.Load() {
		return &online.ProviderConfig{EstablishmentID: es.ID(), Code: payment.ProviderManual, Mode: store.ProviderModeLive}, nil
	}
	if s.twoModes && !test {
		return s.live(es), nil
	}
	return s.cfg(es), nil
}

func (s *settings) ProviderConfig(_ context.Context, _ bun.IDB, es store.EstablishmentScope, provider, mode string) (*online.ProviderConfig, error) {
	if provider != stripe.Code {
		return nil, store.ErrNotFound
	}
	if mode == store.ProviderModeLive {
		if !s.twoModes {
			return nil, store.ErrNotFound
		}
		return s.live(es), nil
	}
	return s.cfg(es), nil
}

// registry is the test double of *app.Registry (payment and fiscal
// drivers): manual, Stripe against the fake, fiscal "none".
type registry struct {
	c      clock.Clock
	stripe ext.PaymentProvider
}

func (r registry) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	switch code {
	case builtin.ManualCode:
		return builtin.Manual{}, true
	case stripe.Code:
		return r.stripe, true
	}
	return nil, false
}

func (r registry) FiscalDriver(code string) (ext.FiscalDriver, bool) {
	if code == builtin.NoneCode {
		return builtin.NoneFiscal{Clock: r.c}, true
	}
	return nil, false
}

// ledgerHook wraps the real ledger to inject a failure after the writes
// (atomicity tests).
type ledgerHook struct {
	gateway.Ledger
	failAfterPayment atomic.Bool
}

var errInjected = errors.New("injected failure")

func (l *ledgerHook) RecordProviderPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in payment.ProviderPayment) (*payment.Result, error) {
	res, err := l.Ledger.RecordProviderPayment(ctx, tx, es, in)
	if err == nil && l.failAfterPayment.Load() {
		return nil, errInjected
	}
	return res, err
}

// env is a lido on the real booking, inventory, customer, payment, cash
// and fiscal services (wired as internal/app does), the Stripe driver
// against the fake Stripe API, and the gateway.
type env struct {
	t        *testing.T
	f        *bookingtest.Fixture
	ctx      context.Context
	db       *store.DB
	es       store.EstablishmentScope
	srv      *stripetest.Server
	bookings *booking.Service
	pay      *payment.Service
	gw       *gateway.Service
	settings *settings
	ledger   *ledgerHook
	driver   *stripe.Driver
	evtSeq   atomic.Int64
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	f := bookingtest.New(t, db)
	c := f.Clock
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	srv := stripetest.New(t)
	srv.Now = c.Now
	zero := int64(0)
	drv := stripe.New(stripe.Options{BaseURL: srv.URL, Clock: c, MaxNetworkRetries: &zero})
	reg := registry{c: c, stripe: drv}
	inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cust := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cs := cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	fs := fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: reg})
	pay := payment.NewService(payment.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
		Cash: cs, Fiscal: fs, Credits: cust, Providers: reg,
	})
	svc := booking.NewService(booking.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: f.Pricing, Customers: cust, Payments: pay,
		Releases: pass.NewResale(pass.ResaleDeps{Clock: c, Credits: cust}), HoldTTL: 15 * time.Minute,
	})
	pay.SetConfirmer(svc)
	st := &settings{}
	lh := &ledgerHook{Ledger: pay}
	gw := gateway.NewService(gateway.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Providers: reg, Settings: st, Ledger: lh,
		BaseURL: "https://lido.example",
	})
	// Wired as internal/app does: a payment after an elapsed hold expires
	// the booking first; an expired hold with money on it schedules refunds.
	gw.SetHoldExpirer(svc)
	svc.SetExpiredRefunder(gw)
	es, err := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, f: f, ctx: f.Ctx, db: db, es: es, srv: srv, bookings: svc, pay: pay, gw: gw, settings: st, ledger: lh, driver: drv}
}

// held creates a held booking of spot i (one day) and returns it.
func (e *env) held(i int, day string) *store.Booking {
	e.t.Helper()
	in := e.f.Input(e.f.Spots[i], day, day)
	in.Status = booking.StatusHeld
	d, err := e.bookings.Create(e.ctx, e.f.Cashier, e.f.Est.ID, in)
	if err != nil {
		e.t.Fatalf("create held booking: %v", err)
	}
	return &d.Booking
}

// start starts a full online payment of b.
func (e *env) start(b *store.Booking) *gateway.StartResult {
	e.t.Helper()
	var res *gateway.StartResult
	err := store.RunInTx(e.ctx, e.db, func(ctx context.Context, tx bun.Tx) error {
		var err error
		res, err = e.gw.Start(ctx, tx, e.es, gateway.StartRequest{
			BookingID: b.ID, BookingCode: b.Code, Purpose: gateway.PurposeFull, Amount: b.TotalAmount, Currency: "EUR",
			Description: "Prenotazione " + b.Code, CustomerEmail: "mario@example.org", Locale: "it",
			ReturnURL: "https://lido.example/b/lido-delfino?checkout=x", CancelURL: "https://lido.example/b/lido-delfino",
		})
		return err
	})
	if err != nil {
		e.t.Fatalf("start payment: %v", err)
	}
	return res
}

// intent returns the payment intent of a start result.
func (e *env) intent(id string) *store.PaymentIntent {
	e.t.Helper()
	pi, err := store.GetPaymentIntent(e.ctx, e.db, e.es, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return pi
}

// sessionOf returns the provider session id of an intent.
func (e *env) sessionOf(res *gateway.StartResult) string { return e.intent(res.IntentID).ExternalID }

// deliver signs payload at the current clock and runs it through the
// gateway for establishment estID.
func (e *env) deliverTo(estID string, payload []byte) (*gateway.WebhookOutcome, error) {
	return e.deliverSigned(estID, payload, stripetest.WebhookSecret)
}

// deliverSigned signs payload with secret (the live or the test webhook
// secret) and runs it through the gateway for establishment estID.
func (e *env) deliverSigned(estID string, payload []byte, secret string) (*gateway.WebhookOutcome, error) {
	r := stripetest.WebhookRequest("/webhooks/payments/stripe/"+estID, payload, e.f.Clock.Now(), secret)
	return e.gw.HandleWebhook(e.ctx, stripe.Code, estID, r)
}

// setTestMode switches the test mode of the lido (spec 15.4) through the store.
func (e *env) setTestMode(on bool) {
	e.t.Helper()
	row, err := store.EnsureTestModeSettings(e.ctx, e.db, e.es, e.f.Clock.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	row.Enabled, row.UpdatedAt = on, e.f.Clock.Now()
	if ok, err := store.UpdateTestModeSettingsVersioned(e.ctx, e.db, e.es, row, row.Version); err != nil || !ok {
		e.t.Fatalf("switch test mode: %v %v", ok, err)
	}
}

func (e *env) deliver(payload []byte) (*gateway.WebhookOutcome, error) {
	return e.deliverTo(e.f.Est.ID, payload)
}

func (e *env) mustDeliver(payload []byte) *gateway.WebhookOutcome {
	e.t.Helper()
	out, err := e.deliver(payload)
	if err != nil {
		e.t.Fatalf("deliver: %v", err)
	}
	return out
}

func (e *env) evt() string { return "evt_test_" + strconv.FormatInt(e.evtSeq.Add(1), 10) }

// paidEvent pays the session at the fake and returns its completed event.
func (e *env) paidEvent(session string) []byte {
	e.srv.Pay(session)
	return e.srv.SessionEvent(e.evt(), "checkout.session.completed", session, e.f.Clock.Now())
}

func (e *env) reload(id string) *store.Booking {
	e.t.Helper()
	b, err := store.GetBooking(e.ctx, e.db, e.es, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *env) count(table, where string, args ...any) int {
	e.t.Helper()
	n, err := e.db.NewSelect().TableExpr(table).Where(where, args...).Count(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	return int(n)
}

// ledger checks INV-22 and INV-24 for a booking and returns paid_amount.
func (e *env) ledgerOf(bookingID string) money.Cents {
	e.t.Helper()
	b := e.reload(bookingID)
	totals, err := store.BookingPaymentTotals(e.ctx, e.db, e.es, bookingID)
	if err != nil {
		e.t.Fatal(err)
	}
	if b.PaidAmount != totals.Net() {
		e.t.Fatalf("INV-22: paid_amount %d != ledger %d", b.PaidAmount, totals.Net())
	}
	if totals.Refunded > totals.In {
		e.t.Fatalf("INV-24: refunded %d > paid %d", totals.Refunded, totals.In)
	}
	return b.PaidAmount
}

// allocKinds returns the kinds of the spot allocations of a booking.
func (e *env) allocKinds(bookingID string) map[string]int {
	e.t.Helper()
	var rows []store.SpotAllocation
	if err := e.db.NewSelect().Model(&rows).Where("booking_id = ?", bookingID).Scan(e.ctx); err != nil {
		e.t.Fatal(err)
	}
	out := map[string]int{}
	for _, r := range rows {
		out[r.Kind]++
	}
	return out
}
