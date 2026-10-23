//go:build integration

package fiscalbridge_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/fiscalbridge"
	"github.com/coste360/arenile/internal/http/api"
	devicehttp "github.com/coste360/arenile/internal/http/device"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

const testToken = "ar_dev_testprefix_secret"

// instance serves the device fiscal-bridge API of svc over HTTP with a
// fake device-token authenticator (the real one is internal/domain/device).
func instance(t *testing.T, svc *fiscalbridge.Service, p *auth.Principal) *httptest.Server {
	t.Helper()
	h := &devicehttp.Handlers{FiscalBridge: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/device/v1/fiscal-bridge", h.RegisterFiscalBridge)
	mux.HandleFunc("DELETE /api/device/v1/fiscal-bridge", h.UnregisterFiscalBridge)
	mux.HandleFunc("GET /api/device/v1/fiscal-bridge/jobs", func(w http.ResponseWriter, r *http.Request) {
		var params api.PollFiscalBridgeJobsParams
		if v := r.URL.Query().Get("wait"); v != "" {
			n, _ := strconv.Atoi(v)
			params.Wait = &n
		}
		if v := r.URL.Query().Get("max"); v != "" {
			n, _ := strconv.Atoi(v)
			params.Max = &n
		}
		h.PollFiscalBridgeJobs(w, r, params)
	})
	mux.HandleFunc("POST /api/device/v1/fiscal-bridge/jobs/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		h.SubmitFiscalBridgeJobResult(w, r, r.PathValue("id"))
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// flaky simulates a bad network between the cash desk PC and the instance.
type flaky struct {
	base http.RoundTripper
	// dropPolls fails that many polls before they reach the instance.
	dropPolls atomic.Int32
	// blockSubmits fails every submission before it reaches the instance.
	blockSubmits atomic.Bool
	// loseAcks fails that many submissions after the instance processed
	// them (the response is lost).
	loseAcks atomic.Int32
	submits  atomic.Int32
}

func (f *flaky) RoundTrip(r *http.Request) (*http.Response, error) {
	isSubmit := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/result")
	isPoll := r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs")
	if isPoll && f.dropPolls.Add(-1) >= 0 {
		return nil, errors.New("simulated: network unreachable")
	}
	if isSubmit {
		f.submits.Add(1)
		if f.blockSubmits.Load() {
			return nil, errors.New("simulated: connection refused")
		}
	}
	resp, err := f.base.RoundTrip(r)
	if err == nil && isSubmit && f.loseAcks.Add(-1) >= 0 {
		_ = resp.Body.Close()
		return nil, errors.New("simulated: connection reset after the request was processed")
	}
	return resp, err
}

// countingDriver is the local printer: it counts the physical issues.
type countingDriver struct {
	mu     sync.Mutex
	issued map[string]int
}

func (*countingDriver) Code() string { return "rt-test" }
func (d *countingDriver) Issue(_ context.Context, req ext.FiscalDocumentRequest) (*ext.FiscalDocumentResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.issued[req.IdempotencyKey]++
	return &ext.FiscalDocumentResult{Ref: "rt_" + req.PaymentID, Number: "0042-" + strconv.Itoa(len(d.issued)), IssuedAt: t0}, nil
}
func (*countingDriver) Void(context.Context, string) error { return nil }
func (*countingDriver) Status(context.Context) (ext.DeviceStatus, error) {
	return ext.DeviceStatus{Online: true, Message: "RT pronto"}, nil
}

func (d *countingDriver) count(key string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.issued[key]
}

type registry struct{ bridge ext.FiscalDriver }

func (r registry) FiscalDriver(code string) (ext.FiscalDriver, bool) {
	if code == fiscalbridge.Code {
		return r.bridge, true
	}
	return nil, false
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestBridge_EndToEnd: payment recorded on the instance -> fiscal document
// pending -> job dispatched to the bridge -> executed once on the local
// driver of the agent -> acked -> document issued with the device number,
// across dropped polls, lost acks and a lease expiry with reconnect.
func TestBridge_EndToEnd(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		svc := f.svc
		f.est.Settings = store.JSON(`{"fiscal":{"driver":"bridge"}}`)
		if _, err := db.NewUpdate().Model(f.est).Column("settings").WherePK().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		fsvc := fiscal.NewService(fiscal.Deps{
			DB: db, Clock: f.clk, Audit: audit.NewWriter(f.clk), Outbox: event.NewOutbox(f.clk),
			Drivers: registry{bridge: &fiscalbridge.Driver{Queue: svc}},
		})
		record := func(amount money.Cents) *store.Payment {
			t.Helper()
			taxable, tax := money.VATSplit(amount, 2200)
			pm := &store.Payment{
				ID: store.NewID(), Provider: "manual", Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn, Amount: amount,
				Status: store.PaymentSucceeded, BusinessDate: calendar.MustParse("2027-07-15"),
				VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: amount, Taxable: taxable, Tax: tax}}, CreatedAt: t0, UpdatedAt: t0,
			}
			if err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
				if err := store.InsertPayment(ctx, tx, f.es, pm); err != nil {
					return err
				}
				return fsvc.RecordPayment(ctx, tx, f.es, pm)
			}); err != nil {
				t.Fatal(err)
			}
			return pm
		}
		docOf := func(pm *store.Payment) store.FiscalDocument {
			t.Helper()
			page, err := store.ListFiscalDocuments(ctx, db, f.es, store.FiscalDocumentFilter{}, "", 50)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range page.Items {
				if d.PaymentID == pm.ID {
					return d
				}
			}
			t.Fatalf("no fiscal document for payment %s", pm.ID)
			return store.FiscalDocument{}
		}

		srv := instance(t, svc, f.devA)
		net := &flaky{base: http.DefaultTransport}
		net.dropPolls.Store(2)
		drv := &countingDriver{issued: map[string]int{}}
		results := fiscalbridge.NewMemoryResults()
		agent := fiscalbridge.NewAgent(fiscalbridge.AgentDeps{
			BaseURL: srv.URL, Token: testToken, HTTPClient: &http.Client{Transport: net}, Driver: drv,
			Results: results, Clock: f.clk, PollWait: 1,
			Sleep: func(ctx context.Context, _ time.Duration) error { time.Sleep(time.Millisecond); return ctx.Err() },
		})
		actx, stop := context.WithCancel(ctx)
		runErr := make(chan error, 1)
		go func() { runErr <- agent.Run(actx) }()
		t.Cleanup(func() { stop(); <-runErr })
		eventually(t, "bridge registration", func() bool {
			st, _ := svc.Status(ctx, f.owner, f.est.ID)
			return st.Registered && st.Driver == "rt-test"
		})

		// 1. Lost acks: the instance stores the result but the agent never
		// sees the 204; its retries hit the idempotent duplicate path.
		net.loseAcks.Store(2)
		pm1 := record(1220)
		if d := docOf(pm1); d.Status != store.FiscalPending || d.Driver != fiscalbridge.Code {
			t.Fatalf("document must wait for the bridge: %+v", d)
		}
		if err := fsvc.IssuePending(ctx); err != nil {
			t.Fatal(err)
		}
		eventually(t, "first job acked", func() bool { return agent.State().Processed >= 1 })
		if err := fsvc.IssuePending(ctx); err != nil {
			t.Fatal(err)
		}
		d1 := docOf(pm1)
		if d1.Status != store.FiscalIssued || d1.DocumentNumber == nil || *d1.DocumentNumber != "0042-1" {
			t.Fatalf("document 1 %+v", d1)
		}
		if n := drv.count("fiscal_" + pm1.ID); n != 1 {
			t.Fatalf("receipt 1 printed %d times", n)
		}

		// 2. Reconnect: every submission fails, the lease expires and the
		// job is delivered again; the agent re-submits the remembered
		// outcome without printing a second receipt.
		net.blockSubmits.Store(true)
		before := net.submits.Load()
		pm2 := record(610)
		if err := fsvc.IssuePending(ctx); err != nil {
			t.Fatal(err)
		}
		eventually(t, "printed while the instance is unreachable", func() bool { return drv.count("fiscal_"+pm2.ID) == 1 })
		eventually(t, "submissions attempted", func() bool { return net.submits.Load()-before >= 5 })
		net.blockSubmits.Store(false)
		f.clk.Advance(fiscalbridge.LeaseDuration + time.Second)
		eventually(t, "second job acked", func() bool {
			_, res, done, _ := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "fiscal_"+pm2.ID, nil)
			return done && res.Outcome == fiscalbridge.OutcomeSucceeded
		})
		if err := fsvc.IssuePending(ctx); err != nil {
			t.Fatal(err)
		}
		if d2 := docOf(pm2); d2.Status != store.FiscalIssued || *d2.DocumentNumber != "0042-2" {
			t.Fatalf("document 2 %+v", d2)
		}
		if n := drv.count("fiscal_" + pm2.ID); n != 1 {
			t.Fatalf("receipt 2 printed %d times", n)
		}
		if n := countAudit(t, db, "fiscal_bridge_job.succeeded", ""); n != 2 {
			t.Fatalf("each job must be acked exactly once, audit has %d", n)
		}
		if n := countAudit(t, db, "fiscal_document.issued", ""); n != 2 {
			t.Fatalf("fiscal documents issued %d", n)
		}

		// 3. Revoked device: the agent stops with ErrDeviceRevoked.
		stop()
		<-runErr
		revoked := fiscalbridge.NewAgent(fiscalbridge.AgentDeps{BaseURL: srv.URL, Token: "ar_dev_other_x", Driver: drv})
		if err := revoked.Run(ctx); !errors.Is(err, fiscalbridge.ErrDeviceRevoked) {
			t.Fatalf("revoked token: %v", err)
		}
		runErr <- nil // satisfy the cleanup
	})
}

// TestConcurrency_FiscalBridgeAck: 100 parallel submissions of the result
// of the same job (agent retries racing with a reconnected agent) complete
// it exactly once, and 100 parallel polls never lease a job twice.
func TestConcurrency_FiscalBridgeAck(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		svc := f.svc
		if _, err := svc.Register(ctx, f.devA, fiscalbridge.Registration{Driver: "noop"}); err != nil {
			t.Fatal(err)
		}
		id, _, _, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "k-ack", doc(f.est.ID, "k-ack"))
		if err != nil {
			t.Fatal(err)
		}
		if jobs, err := svc.Poll(ctx, f.devA, 0, 1); err != nil || len(jobs) != 1 {
			t.Fatalf("lease %v %v", jobs, err)
		}
		const n = 100
		ok := fiscalbridge.Result{Outcome: fiscalbridge.OutcomeSucceeded, Ref: "r", Number: "0001-0001"}
		other := fiscalbridge.Result{Outcome: fiscalbridge.OutcomeFailed, Error: "diverso"}
		var wg sync.WaitGroup
		var lost atomic.Int32
		start := make(chan struct{})
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				res, isOK := ok, i%10 != 9
				if !isOK {
					res = other // a stale, different outcome must never win silently
				}
				err := svc.Submit(ctx, f.devA, id, res)
				switch {
				case err == nil:
				case errors.Is(err, fiscalbridge.ErrLeaseLost):
					lost.Add(1)
				default:
					t.Errorf("submit (ok=%v): %v", isOK, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		succeeded := countAudit(t, db, "fiscal_bridge_job.succeeded", id)
		failed := countAudit(t, db, "fiscal_bridge_job.failed", id)
		if succeeded+failed != 1 {
			t.Fatalf("job completed %d+%d times", succeeded, failed)
		}
		job, err := store.GetFiscalBridgeJob(ctx, db, f.es, id)
		if err != nil {
			t.Fatal(err)
		}
		// Every submission that disagrees with the stored outcome is refused.
		wantLost := int32(n / 10)
		if job.Status == store.FiscalBridgeJobFailed {
			wantLost = n - n/10
		}
		if lost.Load() != wantLost {
			t.Fatalf("status %s: %d refused, want %d", job.Status, lost.Load(), wantLost)
		}

		// Parallel pollers over 20 jobs: each job is leased exactly once.
		for i := range 20 {
			key := "k-poll-" + strconv.Itoa(i)
			if _, _, _, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, key, doc(f.est.ID, key)); err != nil {
				t.Fatal(err)
			}
		}
		var mu sync.Mutex
		seen := map[string]int{}
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				jobs, err := svc.Poll(ctx, f.devA, 0, 3)
				if err != nil {
					t.Errorf("poll: %v", err)
					return
				}
				mu.Lock()
				for _, j := range jobs {
					seen[j.ID]++
				}
				mu.Unlock()
			}()
		}
		wg.Wait()
		// Whatever the racing pollers left is taken by a final poll.
		rest, err := svc.Poll(ctx, f.devA, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, j := range rest {
			seen[j.ID]++
		}
		if len(seen) != 20 {
			t.Fatalf("leased %d distinct jobs, want 20", len(seen))
		}
		for jid, c := range seen {
			if c != 1 {
				t.Fatalf("job %s leased %d times", jid, c)
			}
		}
	})
}
