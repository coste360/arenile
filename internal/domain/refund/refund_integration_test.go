//go:build integration

package refund_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/refund"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

const provider = "fakepay"

// tempErr is a temporary provider failure (gateway.isTemporary).
type tempErr struct{}

func (tempErr) Error() string   { return "fakepay: unavailable" }
func (tempErr) Temporary() bool { return true }

// fakeProvider refunds in memory and deduplicates by idempotency key like
// a real provider: the same key never moves money twice.
type fakeProvider struct {
	down  atomic.Bool
	calls atomic.Int64
	mu    sync.Mutex
	byKey map[string]string
	moved map[string]int64 // provider refund id -> amount
}

func newProvider() *fakeProvider {
	return &fakeProvider{byKey: map[string]string{}, moved: map[string]int64{}}
}

func (p *fakeProvider) Code() string { return provider }
func (p *fakeProvider) Capabilities() ext.PaymentCapabilities {
	return ext.PaymentCapabilities{Online: true, Refunds: true, PartialRefunds: true}
}

func (p *fakeProvider) CreatePayment(context.Context, ext.CreatePaymentRequest) (*ext.CreatePaymentResult, error) {
	return nil, ext.ErrNotSupported
}

func (p *fakeProvider) ParseWebhook(context.Context, *http.Request, ext.Credentials) (*ext.PaymentEvent, error) {
	return nil, ext.ErrNotSupported
}

func (p *fakeProvider) Refund(_ context.Context, req ext.RefundRequest) (*ext.RefundResult, error) {
	p.calls.Add(1)
	if p.down.Load() {
		return nil, tempErr{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	id, ok := p.byKey[req.IdempotencyKey]
	if !ok {
		id = "re_" + strconv.Itoa(len(p.byKey)+1)
		p.byKey[req.IdempotencyKey] = id
		p.moved[id] = req.Amount
	}
	return &ext.RefundResult{ProviderRef: id, Status: ext.RefundSucceeded}, nil
}

// refunds is the money the provider actually moved.
func (p *fakeProvider) refunds() (n int, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, a := range p.moved {
		n++
		total += a
	}
	return n, total
}

type registry struct{ p *fakeProvider }

func (r registry) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	if code == provider {
		return r.p, true
	}
	return nil, false
}

type settings struct{}

func (settings) cfg(es store.EstablishmentScope) *online.ProviderConfig {
	return &online.ProviderConfig{EstablishmentID: es.ID(), Code: provider, Mode: store.ProviderModeTest, Online: true, Credentials: ext.Credentials{}}
}

func (s settings) ActiveProvider(_ context.Context, _ bun.IDB, es store.EstablishmentScope) (*online.ProviderConfig, error) {
	return s.cfg(es), nil
}

func (s settings) ProviderConfig(_ context.Context, _ bun.IDB, es store.EstablishmentScope, code string) (*online.ProviderConfig, error) {
	if code != provider {
		return nil, store.ErrNotFound
	}
	return s.cfg(es), nil
}

// env is the public flow (checkouttest) with the real gateway on the fake
// provider and the real refund service plugged into the cancellations.
type env struct {
	*checkouttest.Env
	prov *fakeProvider
	gw   *gateway.Service
	rf   *refund.Service
}

func newEnv(t *testing.T, db *store.DB, maxAttempts int) *env {
	t.Helper()
	e := checkouttest.New(t, db)
	c := e.F.Clock
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	prov := newProvider()
	gw := gateway.NewService(gateway.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Providers: registry{p: prov}, Settings: settings{}, Ledger: e.Payments,
		BaseURL: "https://lido.example",
	})
	rf := refund.NewService(refund.Deps{DB: db, Clock: c, Audit: aw, Sender: gw, MaxAttempts: maxAttempts})
	e.Book.SetCancelRefunds(rf)
	return &env{Env: e, prov: prov, gw: gw, rf: rf}
}

// confirmed books spot on day through the public flow (manual driver).
func (e *env) confirmed(spot int, day string) string {
	e.T.Helper()
	s := e.Create(e.Seat(e.F.Spots[spot], day))
	e.Details(s, "anna@example.org")
	if _, err := e.Svc.Pay(e.Ctx, checkouttest.Slug, s.BookingID, s.Token, checkout.PayInput{}); err != nil {
		e.T.Fatalf("pay: %v", err)
	}
	return s.BookingID
}

// paidOnline records a provider payment of amount: ledger row and its
// succeeded payment intent.
func (e *env) paidOnline(bookingID string, amount money.Cents) *store.PaymentIntent {
	e.T.Helper()
	now := e.F.Clock.Now()
	ref := "pi_" + store.NewID()
	pm := &store.Payment{
		ID: store.NewID(), EstablishmentID: e.F.Est.ID, BookingID: &bookingID, Provider: provider, Method: "card", Direction: "in",
		Amount: amount, Status: "succeeded", ProviderRef: &ref, VATBreakdown: []store.VATShare{}, BusinessDate: calendar.In(now, time.UTC),
		Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	e.F.Insert(pm)
	pi := &store.PaymentIntent{
		ID: store.NewID(), EstablishmentID: e.F.Est.ID, BookingID: &bookingID, PaymentID: &pm.ID, Provider: provider, Mode: store.ProviderModeTest,
		Kind: store.IntentKindPayment, ExternalID: "cs_" + pm.ID, ExternalRef: &ref, Status: store.IntentSucceeded, Amount: amount,
		Currency: "EUR", Attempt: 1, RawEventIDs: []string{}, CreatedAt: now, UpdatedAt: now,
	}
	e.F.Insert(pi)
	e.F.PayBooking(bookingID, amount)
	return pi
}

// paidCash records a desk payment (no provider).
func (e *env) paidCash(bookingID string, amount money.Cents) {
	e.T.Helper()
	now := e.F.Clock.Now()
	e.F.Insert(&store.Payment{
		ID: store.NewID(), EstablishmentID: e.F.Est.ID, BookingID: &bookingID, Provider: "manual", Method: "cash", Direction: "in",
		Amount: amount, Status: "succeeded", VATBreakdown: []store.VATShare{}, BusinessDate: calendar.In(now, time.UTC),
		Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	})
	e.F.PayBooking(bookingID, amount)
}

func (e *env) cancel(bookingID string) *booking.CancelResult {
	e.T.Helper()
	res, err := e.Book.CancelAsCustomer(e.Ctx, audit.SystemActor, e.ES, bookingID, booking.CancelInput{Reason: "cambio programma"})
	if err != nil {
		e.T.Fatalf("cancel: %v", err)
	}
	return res
}

func (e *env) requests(bookingID string) []store.RefundRequest {
	e.T.Helper()
	rs, err := store.ListBookingRefundRequests(e.Ctx, e.DB, e.ES, bookingID)
	if err != nil {
		e.T.Fatal(err)
	}
	return rs
}

func (e *env) only(bookingID string) store.RefundRequest {
	e.T.Helper()
	rs := e.requests(bookingID)
	if len(rs) != 1 {
		e.T.Fatalf("refund requests: %+v", rs)
	}
	return rs[0]
}

func (e *env) run() {
	e.T.Helper()
	if _, err := e.rf.RunDue(e.Ctx); err != nil {
		e.T.Fatal(err)
	}
}

func (e *env) ledgerRefunds(bookingID string) (n int, total money.Cents) {
	e.T.Helper()
	ps, err := e.Payments.ListByBooking(e.Ctx, e.DB, e.ES, bookingID)
	if err != nil {
		e.T.Fatal(err)
	}
	for _, p := range ps {
		if p.Direction == store.PaymentDirectionRefund && p.Status == store.PaymentSucceeded {
			n++
			total += p.Amount
		}
	}
	return n, total
}

// TestRefund_ProviderDownThenUp: the refund is reserved in the
// cancellation transaction (visible before any send), survives a provider
// outage with backoff (1m, 5m), blocks manual refunds meanwhile, and
// completes exactly once when the provider is back; later runs change
// nothing.
func TestRefund_ProviderDownThenUp(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db, 0)
		id := e.confirmed(1, "2027-07-20")
		e.paidOnline(id, 2000)
		e.prov.down.Store(true)

		res := e.cancel(id)
		if res.RefundReserved != 2000 || res.RefundDue != 0 || len(res.RefundRequestIDs) != 1 {
			t.Fatalf("cancel result %+v", res)
		}
		rr := e.only(id)
		if rr.Status != store.RefundRequestPending || rr.Attempts != 0 || rr.Amount != 2000 || rr.Reason != store.RefundReasonCustomerCancellation ||
			rr.IdempotencyKey != refund.IdempotencyKey(id, rr.PaymentIntentID) {
			t.Fatalf("reserved request %+v", rr)
		}
		if n := e.Count((*store.AuditEntry)(nil), "entity_id = ? AND action = ?", id, refund.ActionReserved); n != 1 {
			t.Fatalf("reserve audit: %d", n)
		}

		// Manual refunds race with the guaranteed one: refused.
		if _, err := e.Payments.RefundBooking(e.Ctx, e.F.Manager, "", e.F.Est.ID, payment.RefundRequest{BookingID: id, Amount: 100, Method: "cash"}); !errors.Is(err, payment.ErrRefundInProgress) {
			t.Fatalf("manual refund during guaranteed refund: %v", err)
		}

		start := e.F.Clock.Now()
		e.run()
		rr = e.only(id)
		if rr.Status != store.RefundRequestPending || rr.Attempts != 1 || rr.LastError == nil || *rr.LastError != "provider_unavailable" ||
			rr.RefundIntentID == nil || !rr.NextAttemptAt.Equal(start.Add(time.Minute)) {
			t.Fatalf("after outage %+v", rr)
		}
		calls := e.prov.calls.Load()
		e.run() // not due yet
		if e.prov.calls.Load() != calls {
			t.Fatal("a request was sent before its backoff elapsed")
		}
		e.F.Clock.Advance(time.Minute)
		e.run()
		if rr = e.only(id); rr.Attempts != 2 || !rr.NextAttemptAt.Equal(e.F.Clock.Now().Add(5*time.Minute)) {
			t.Fatalf("second attempt %+v", rr)
		}

		e.prov.down.Store(false)
		e.F.Clock.Advance(5 * time.Minute)
		e.run()
		rr = e.only(id)
		if rr.Status != store.RefundRequestSucceeded || rr.PaymentID == nil || rr.CompletedAt == nil || rr.Attempts != 3 || rr.NextAttemptAt != nil {
			t.Fatalf("after recovery %+v", rr)
		}
		if n, total := e.prov.refunds(); n != 1 || total != 2000 {
			t.Fatalf("provider refunds: %d, %d", n, total)
		}
		if n, total := e.ledgerRefunds(id); n != 1 || total != 2000 {
			t.Fatalf("ledger refunds: %d, %d", n, total)
		}
		if b := e.Booking(id); b.PaidAmount != 0 || b.Status != "cancelled" {
			t.Fatalf("booking after refund: %+v", b)
		}
		d, err := e.Book.DetailIn(e.Ctx, e.DB, e.ES, id)
		if err != nil || len(d.RefundRequests) != 1 {
			t.Fatalf("detail refunds: %v %+v", err, d)
		}
		if m := checkout.ManageRefundOf(d.RefundRequests); m == nil || m.Status != store.RefundRequestSucceeded || m.Amount != 2000 || m.CompletedAt == nil {
			t.Fatalf("manage refund %+v", m)
		}

		// Nothing more happens.
		e.F.Clock.Advance(24 * time.Hour)
		e.run()
		if n, _ := e.prov.refunds(); n != 1 {
			t.Fatal("refund repeated")
		}
		if n, _ := e.ledgerRefunds(id); n != 1 {
			t.Fatal("ledger refund repeated")
		}
	})
}

// TestRefund_DeadLetterThenRetry: attempts exhausted -> failed with an
// owner notification (no personal data) and an audit; a non-failed
// request cannot be retried; the manual retry sends it at once with a
// working provider, exactly once.
func TestRefund_DeadLetterThenRetry(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db, 3)
		id := e.confirmed(1, "2027-07-20")
		e.paidOnline(id, 2000)
		e.prov.down.Store(true)
		e.cancel(id)
		rr := e.only(id)
		if _, err := e.rf.Retry(e.Ctx, e.F.Manager, "", e.F.Est.ID, rr.ID); !errors.Is(err, booking.ErrInvalidTransition) {
			t.Fatalf("retry of a pending request: %v", err)
		}
		for i := 0; i < 3; i++ {
			e.run()
			e.F.Clock.Advance(13 * time.Hour)
		}
		rr = e.only(id)
		if rr.Status != store.RefundRequestFailed || rr.Attempts != 3 || rr.NextAttemptAt != nil {
			t.Fatalf("dead letter %+v", rr)
		}
		var n store.AdminNotification
		if err := db.NewSelect().Model(&n).Where("establishment_id = ?", e.F.Est.ID).Where("kind = ?", refund.NotificationKindRefundFailed).Scan(e.Ctx); err != nil {
			t.Fatal(err)
		}
		if n.Audience != store.NotificationAudienceOwner || n.Severity != "error" || n.Params["refund_request_id"] != rr.ID || n.Params["amount"] != "2000" {
			t.Fatalf("notification %+v", n)
		}
		for k := range n.Params {
			switch k {
			case "refund_request_id", "booking_id", "booking_code", "amount", "attempts":
			default:
				t.Fatalf("unexpected notification param %q", k)
			}
		}
		e.run() // a failed request is never sent by the job
		if got := e.only(id); got.Attempts != 3 {
			t.Fatalf("job touched a failed request: %+v", got)
		}
		if _, err := e.rf.Retry(e.Ctx, e.F.Viewer, "", e.F.Est.ID, rr.ID); err == nil {
			t.Fatal("viewer retried a refund")
		}

		e.prov.down.Store(false)
		got, err := e.rf.Retry(e.Ctx, e.F.Manager, "", e.F.Est.ID, rr.ID)
		if err != nil || got.Status != store.RefundRequestSucceeded || got.IdempotencyKey != rr.IdempotencyKey {
			t.Fatalf("retry: %v %+v", err, got)
		}
		if n, total := e.prov.refunds(); n != 1 || total != 2000 {
			t.Fatalf("provider refunds: %d, %d", n, total)
		}
		if n := e.Count((*store.AuditEntry)(nil), "entity_id = ? AND action IN (?)", id, bun.List([]string{refund.ActionFailed, refund.ActionRetried, refund.ActionSucceeded})); n != 3 {
			t.Fatalf("audits: %d", n)
		}
	})
}

// TestRefund_ReserveCapsAtProviderPayments: only provider payments are
// refunded through the provider, each at most for what it captured; the
// desk part stays a manual refund due.
func TestRefund_ReserveCapsAtProviderPayments(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db, 0)
		id := e.confirmed(1, "2027-07-20")
		first := e.paidOnline(id, 700)
		second := e.paidOnline(id, 800)
		e.paidCash(id, 500)
		res := e.cancel(id)
		if res.Refund.Amount != 2000 || res.RefundReserved != 1500 || res.RefundDue != 500 || len(res.RefundRequestIDs) != 2 {
			t.Fatalf("cancel result %+v", res)
		}
		got := map[string]money.Cents{}
		for _, rr := range e.requests(id) {
			got[rr.PaymentIntentID] = rr.Amount
		}
		if got[first.ID] != 700 || got[second.ID] != 800 {
			t.Fatalf("parts %+v", got)
		}
		e.run()
		if n, total := e.prov.refunds(); n != 2 || total != 1500 {
			t.Fatalf("provider refunds: %d, %d", n, total)
		}
		if b := e.Booking(id); b.PaidAmount != 500 {
			t.Fatalf("paid after provider refunds: %d", b.PaidAmount)
		}
	})
}

// TestConcurrency_CancelRefundOnce: 100 parallel attempts on one booking,
// half customer cancellations and half job runs: one cancellation wins,
// one request is reserved, the provider moves the money exactly once and
// the ledger records it once.
func TestConcurrency_CancelRefundOnce(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db, 0)
		id := e.confirmed(1, "2027-07-20")
		e.paidOnline(id, 2000)
		const n = 100
		var wg sync.WaitGroup
		var cancelled atomic.Int64
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				if i%2 == 0 {
					if _, err := e.Book.CancelAsCustomer(e.Ctx, audit.SystemActor, e.ES, id, booking.CancelInput{}); err == nil {
						cancelled.Add(1)
					}
					return
				}
				_, _ = e.rf.RunDue(e.Ctx)
				if rs, err := store.ListBookingRefundRequests(e.Ctx, db, e.ES, id); err == nil {
					for _, rr := range rs {
						_ = e.rf.Process(e.Ctx, e.ES, rr.ID)
					}
				}
			}(i)
		}
		close(start)
		wg.Wait()
		e.run()
		if cancelled.Load() != 1 {
			t.Fatalf("cancellations: %d", cancelled.Load())
		}
		rr := e.only(id)
		if rr.Status != store.RefundRequestSucceeded {
			t.Fatalf("request %+v", rr)
		}
		if n, total := e.prov.refunds(); n != 1 || total != 2000 {
			t.Fatalf("provider refunds: %d, %d", n, total)
		}
		if n, total := e.ledgerRefunds(id); n != 1 || total != 2000 {
			t.Fatalf("ledger refunds: %d, %d", n, total)
		}
		if k := e.Count((*store.PaymentIntent)(nil), "booking_id = ? AND kind = ?", id, store.IntentKindRefund); k != 1 {
			t.Fatalf("refund intents: %d", k)
		}
	})
}

// TestRefund_PanelCancellation: a panel cancellation with a money refund
// and no desk method reserves the guaranteed refund (reason
// panel_cancellation, author recorded); without refunds.issue it stays due.
func TestRefund_PanelCancellation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db, 0)
		id := e.confirmed(1, "2027-07-20")
		e.paidOnline(id, 2000)
		res, err := e.Book.Cancel(e.Ctx, e.F.Manager, e.F.Est.ID, id, booking.CancelInput{Settlement: booking.SettleRefund})
		if err != nil || res.RefundReserved != 2000 || res.RefundDue != 0 {
			t.Fatalf("panel cancel: %v %+v", err, res)
		}
		rr := e.only(id)
		if rr.Reason != store.RefundReasonPanelCancellation || rr.CreatedByUserID == nil || *rr.CreatedByUserID != e.F.Manager.UserID {
			t.Fatalf("request %+v", rr)
		}
		e.run()
		if got := e.only(id); got.Status != store.RefundRequestSucceeded {
			t.Fatalf("after send %+v", got)
		}

		other := e.confirmed(2, "2027-07-21")
		e.paidOnline(other, 1000)
		res, err = e.Book.Cancel(e.Ctx, e.F.Cashier, e.F.Est.ID, other, booking.CancelInput{Settlement: booking.SettleRefund})
		if err != nil {
			t.Fatal(err)
		}
		if res.RefundReserved != 0 || res.RefundDue != 1000 || len(e.requests(other)) != 0 {
			t.Fatalf("cancel without refunds.issue: %+v", res)
		}
	})
}
