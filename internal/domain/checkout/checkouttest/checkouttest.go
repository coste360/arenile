// Package checkouttest builds the public flow on a real database for
// integration tests (internal/domain/checkout, internal/http/public): the
// bookingtest lido with booking enabled online, the real booking,
// inventory, customer, pricing, pass, payment and online services wired as
// internal/app does, and in-memory adapters for the ports owned by other
// M4 packages (payment gateway provider calls, notify magic links / QR /
// .ics). The zone-mode zone gets its system pool and auto-assignment.
package checkouttest

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// env is the public flow on the real booking, inventory, customer,
// pricing, pass, payment and online services (wired as internal/app does)
// with in-memory adapters for the two ports owned by other M4 packages:
// the payment gateway (provider calls) and notify (magic links, QR, .ics).
type Env struct {
	T      testing.TB
	F      *bookingtest.Fixture
	DB     *store.DB
	Ctx    context.Context
	ES     store.EstablishmentScope
	Svc    *checkout.Service
	Online *online.Service
	Book   *booking.Service
	Inv    *inventory.Service
	Pay    *Payments
	Notify *Notifier
	Pool   *store.Pool
	// M5B: the real customer, payment and credit services (the credit
	// ledger is the Credits port of the payments, as internal/app wires it).
	Customers *customer.Service
	Payments  *payment.Service
	Credits   *credit.Service
}

const Slug = "lido-delfino"

type Options struct {
	// zoneSpots adds spots to the zone-mode zone (the fixture has 2).
	ZoneSpots int
	// quota sets a fixed online quota on the zone-mode zone.
	Quota int
}

func New(t testing.TB, db *store.DB, opts ...Options) *Env {
	t.Helper()
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	f := bookingtest.New(t, db)
	c := f.Clock
	ctx := f.Ctx
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	reg := app.NewRegistry(c, ext.Extensions{})
	inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cust := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	ledger := credit.NewService(credit.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cust.UseCreditLedger(ledger)
	cs := cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	fs := fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: reg})
	pays := payment.NewService(payment.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
		Cash: cs, Fiscal: fs, Credits: cust, Providers: reg,
	})
	bk := booking.NewService(booking.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: f.Pricing, Customers: cust, Payments: pays,
		Releases: pass.NewResale(pass.ResaleDeps{Clock: c, Credits: cust}),
	})
	pays.SetConfirmer(bk)
	ledger.SetPayments(pays)
	passes := pass.NewService(pass.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: bk, Inventory: inv, Credits: cust})
	keys, err := crypto.NewKeyRing(make([]byte, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	on := online.NewService(online.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Keys: keys, Providers: testProviders{}, BaseURL: "https://prenota.example.org"})
	e := &Env{
		T: t, F: f, DB: db, Ctx: ctx, Online: on, Book: bk, Inv: inv, Pay: &Payments{}, Notify: &Notifier{db: db, clock: c},
		Customers: cust, Payments: pays, Credits: ledger,
	}
	e.ES, err = store.SystemEstablishmentScope(ctx, db, f.Est.ID)
	if err != nil {
		t.Fatal(err)
	}
	e.Svc = checkout.NewService(checkout.Deps{
		DB: db, Clock: c, Settings: on, Inventory: inv, Pricer: f.Pricing, Bookings: bk, Payments: e.Pay, Notifier: e.Notify,
		Passes: passes, HoldTTL: 10 * time.Minute, BaseURL: "https://prenota.example.org", CreatesPerEstablishment: -1,
	})
	if _, err := on.Update(ctx, f.Owner, f.Est.ID, nil, online.Input{BookingEnabled: true, Flow: online.Config{PromoCodesEnabled: true}}); err != nil {
		t.Fatal(err)
	}
	// The zone-mode zone: more spots, auto-assignment, its system pool.
	now := c.Now()
	for i := 0; i < o.ZoneSpots; i++ {
		sp := &store.Spot{
			ID: store.NewID(), EstablishmentID: f.Est.ID, MapID: f.FreeSpots[0].MapID, ZoneID: f.FreeZone.ID, Label: fmt.Sprintf("B%d", 3+i),
			RowLabel: "2", Number: 3 + i, Kind: "umbrella", X: float64(100 + 10*i), Y: 40, Shape: store.Shape{Type: "circle", W: 20, H: 20},
			Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
		}
		f.Insert(sp)
		f.FreeSpots = append(f.FreeSpots, sp)
	}
	zone := f.FreeZone
	zone.AutoAssign = true
	if o.Quota > 0 {
		zone.OnlineQuota = &store.OnlineQuota{Type: "count", Value: o.Quota}
	}
	if _, err := db.NewUpdate().Model(zone).Column("auto_assign", "online_quota").WherePK().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	zoneID := zone.ID
	e.Pool = &store.Pool{
		ID: store.NewID(), EstablishmentID: f.Est.ID, ZoneID: &zoneID, ComponentCode: mapping.SystemPoolComponentCode,
		Name: store.I18nText{"it": "B"}, Capacity: len(f.FreeSpots), CapacityOverrides: []store.CapacityOverride{},
		SystemManaged: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.Insert(e.Pool)
	return e
}

// selection is a one-day seat booking of the base pack.
func (e *Env) Seat(spot *store.Spot, day string) checkout.Selection {
	return checkout.Selection{
		StartsOn: calendar.MustParse(day), EndsOn: calendar.MustParse(day), Slots: calendar.SlotsFull, Formula: "day",
		Lines: []booking.LineInput{{PackID: e.F.Base.ID, SpotID: spot.ID}},
	}
}

// zone is a one-day zone-mode booking of the base pack.
func (e *Env) Zone(day string) checkout.Selection {
	return checkout.Selection{
		StartsOn: calendar.MustParse(day), EndsOn: calendar.MustParse(day), Slots: calendar.SlotsFull, Formula: "day",
		Lines: []booking.LineInput{{PackID: e.F.Base.ID, ZoneID: e.F.FreeZone.ID}},
	}
}

func (e *Env) Create(sel checkout.Selection) *checkout.Session {
	e.T.Helper()
	s, err := e.Svc.Create(e.Ctx, Slug, checkout.CreateInput{Selection: sel, Locale: "it"})
	if err != nil {
		e.T.Fatalf("create: %v", err)
	}
	return s
}

func (e *Env) Details(s *checkout.Session, email string) *checkout.State {
	e.T.Helper()
	st, err := e.Svc.UpdateCustomer(e.Ctx, Slug, s.BookingID, s.Token, checkout.CustomerInput{
		FirstName: "Giulia", LastName: "Bianchi", Email: email, Phone: "333 1234567", Locale: "it", MarketingConsent: true,
	})
	if err != nil {
		e.T.Fatalf("details: %v", err)
	}
	return st
}

func (e *Env) Booking(id string) *store.Booking {
	e.T.Helper()
	b, err := store.GetBooking(e.Ctx, e.DB, e.ES, id)
	if err != nil {
		e.T.Fatal(err)
	}
	return b
}

func (e *Env) Count(model any, where string, args ...any) int {
	return e.F.Count(model, where, args...)
}

// ---- in-memory adapters -------------------------------------------------------------

// testProviders registers the manual driver and an online driver.
type testProviders struct{}

func (testProviders) PaymentProviderCodes() []string { return []string{"manual", "stripe"} }
func (testProviders) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	switch code {
	case "stripe", "manual":
		return testDriver{code: code}, true
	}
	return nil, false
}

type testDriver struct{ code string }

func (d testDriver) Code() string { return d.code }
func (d testDriver) Capabilities() ext.PaymentCapabilities {
	return ext.PaymentCapabilities{Online: d.code != "manual", Refunds: true}
}

func (testDriver) CreatePayment(context.Context, ext.CreatePaymentRequest) (*ext.CreatePaymentResult, error) {
	return nil, ext.ErrNotSupported
}

func (testDriver) ParseWebhook(context.Context, *http.Request, ext.Credentials) (*ext.PaymentEvent, error) {
	return nil, ext.ErrNotSupported
}

func (testDriver) Refund(context.Context, ext.RefundRequest) (*ext.RefundResult, error) {
	return nil, ext.ErrNotSupported
}

// fakePayments records the provider payments asked by checkout.
type Payments struct {
	mu      sync.Mutex
	Starts  []gateway.StartRequest
	Refunds []gateway.RefundRequest
	Fail    error
}

func (p *Payments) Start(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, req gateway.StartRequest) (*gateway.StartResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Fail != nil {
		return nil, p.Fail
	}
	p.Starts = append(p.Starts, req)
	return &gateway.StartResult{Provider: "stripe", Mode: ext.PaymentModeRedirect, RedirectURL: "https://checkout.stripe.test/c/" + req.BookingID, Amount: req.Amount}, nil
}

func (p *Payments) LatestForBooking(context.Context, bun.IDB, store.EstablishmentScope, string) (*store.PaymentIntent, error) {
	return nil, store.ErrNotFound
}

func (p *Payments) RefundViaProvider(_ context.Context, _ store.EstablishmentScope, req gateway.RefundRequest) (*store.PaymentIntent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Refunds = append(p.Refunds, req)
	return &store.PaymentIntent{ID: store.NewID(), Status: store.IntentPending, Amount: req.Amount}, nil
}

// fakeNotifier keeps magic links in magic_link_tokens (hash only, like
// notify) and records the "new link" requests.
type Notifier struct {
	db       *store.DB
	clock    interface{ Now() time.Time }
	mu       sync.Mutex
	Requests []string
}

func (n *Notifier) IssueMagicLink(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, _ *string) (*notify.MagicLink, error) {
	b, err := store.GetBooking(ctx, tx, es, bookingID)
	if err != nil {
		return nil, err
	}
	tok, hash, err := checkout.NewToken()
	if err != nil {
		return nil, err
	}
	now := n.clock.Now().UTC()
	row := &store.MagicLinkToken{
		ID: store.NewID(), EstablishmentID: es.ID(), BookingID: bookingID, TokenHash: hash,
		ExpiresAt: b.EndsOn.Time().Add(24*time.Hour + notify.MagicLinkGrace), CreatedAt: now,
	}
	if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, err
	}
	return &notify.MagicLink{ID: row.ID, Token: tok, ExpiresAt: row.ExpiresAt}, nil
}

func (n *Notifier) ResolveMagicLink(ctx context.Context, db bun.IDB, es store.EstablishmentScope, token string) (*store.MagicLinkToken, error) {
	row := new(store.MagicLinkToken)
	err := db.NewSelect().Model(row).Where("token_hash = ?", checkout.HashToken(token)).Where("establishment_id = ?", es.ID()).
		Where("revoked_at IS NULL").Where("expires_at > ?", n.clock.Now().UTC()).Scan(ctx)
	if err != nil {
		return nil, notify.ErrLinkNotFound
	}
	return row, nil
}

func (n *Notifier) RequestLink(_ context.Context, _ store.EstablishmentScope, email, code string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Requests = append(n.Requests, email+"|"+code)
	return nil
}

func (n *Notifier) QR(payload, format string) ([]byte, string, error) {
	if format == notify.QRPNG {
		return []byte("\x89PNG"), "image/png", nil
	}
	return []byte("<svg>" + payload + "</svg>"), "image/svg+xml", nil
}

func (n *Notifier) ICS(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, bookingID, manageURL string) ([]byte, error) {
	return []byte("BEGIN:VCALENDAR\r\nX-BOOKING:" + bookingID + "\r\nX-MANAGE:" + manageURL + "\r\nEND:VCALENDAR\r\n"), nil
}

var (
	_ checkout.Payments = (*Payments)(nil)
	_ checkout.Notifier = (*Notifier)(nil)
)
