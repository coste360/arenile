//go:build integration

package fiscalbridge_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/fiscalbridge"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

var t0 = time.Date(2027, 7, 15, 8, 0, 0, 0, time.UTC)

// fixture is an establishment with a cashier and two paired desktop devices.
type fixture struct {
	db      *store.DB
	clk     *clock.Fake
	svc     *fiscalbridge.Service
	org     *store.Organization
	est     *store.Establishment
	es      store.EstablishmentScope
	userID  string
	devA    *auth.Principal
	devB    *auth.Principal
	owner   *auth.Principal
	foreign *auth.Principal
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", t0)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
	if err != nil {
		t.Fatal(err)
	}
	orgB, _, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
	if err != nil {
		t.Fatal(err)
	}
	userID := insertUser(t, db, "cassa@example.test")
	clk := clock.NewFake(t0)
	f := &fixture{
		db: db, clk: clk, org: org, est: est, es: es, userID: userID,
		svc: fiscalbridge.NewService(fiscalbridge.Deps{DB: db, Clock: clk, Audit: audit.NewWriter(clk), Recheck: 5 * time.Millisecond}),
	}
	grants := []auth.Grant{{OrganizationID: org.ID, EstablishmentID: est.ID, Role: auth.RoleCashier}}
	f.devA = &auth.Principal{Kind: auth.PrincipalUser, UserID: userID, Grants: grants, DeviceID: insertDevice(t, db, est.ID, userID, "Cassa 1"), DeviceName: "Cassa 1", DeviceEstablishmentID: est.ID}
	f.devB = &auth.Principal{Kind: auth.PrincipalUser, UserID: userID, Grants: grants, DeviceID: insertDevice(t, db, est.ID, userID, "Cassa 2"), DeviceName: "Cassa 2", DeviceEstablishmentID: est.ID}
	f.owner = &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, Role: auth.RoleOwner}}}
	f.foreign = &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgB.ID, Role: auth.RoleOwner}}}
	return f
}

func insertUser(t *testing.T, db *store.DB, email string) string {
	t.Helper()
	u := &store.User{ID: store.NewID(), Email: email, PasswordHash: "x", Name: "Cassa", Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0}
	if _, err := db.NewInsert().Model(u).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func insertDevice(t *testing.T, db *store.DB, estID, userID, name string) string {
	t.Helper()
	d := &store.Device{
		ID: store.NewID(), EstablishmentID: estID, UserID: userID, Name: name, Platform: store.DevicePlatformWindows,
		Kind: store.DeviceKindDesktop, TokenPrefix: "p" + store.NewID()[:6], TokenHash: store.NewID(), CreatedAt: t0, UpdatedAt: t0,
	}
	if _, err := db.NewInsert().Model(d).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	return d.ID
}

func doc(estID, key string) fiscalbridge.Document {
	return fiscalbridge.DocumentFromRequest(ext.FiscalDocumentRequest{
		EstablishmentID: estID, Currency: "EUR", Total: 1220, IdempotencyKey: key, IssuedAt: t0,
		Lines:    []ext.FiscalLine{{Description: "Ombrellone", Quantity: 1, UnitGross: 1220, Gross: 1220, VATRateBP: 2200}},
		Payments: []ext.FiscalPayment{{Method: "cash", Amount: 1220}},
	})
}

func countAudit(t *testing.T, db *store.DB, action, entityID string) int {
	t.Helper()
	q := db.NewSelect().Model((*store.AuditEntry)(nil)).Where("action = ?", action)
	if entityID != "" {
		q = q.Where("entity_id = ?", entityID)
	}
	n, err := q.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func TestService_RegisterPollSubmit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		svc := f.svc

		// No bridge yet: status empty, polls refused, jobs still queue.
		st, err := svc.Status(ctx, f.owner, f.est.ID)
		if err != nil || st.Registered || st.PendingJobs != 0 {
			t.Fatalf("status %+v %v", st, err)
		}
		if _, err := svc.Poll(ctx, f.devA, 0, 10); !errors.Is(err, fiscalbridge.ErrNotActiveBridge) {
			t.Fatalf("poll without registration: %v", err)
		}
		if _, err := svc.Status(ctx, f.foreign, f.est.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign status must be not found: %v", err)
		}
		if _, err := svc.Register(ctx, f.owner, fiscalbridge.Registration{Driver: "noop"}); !errors.Is(err, fiscalbridge.ErrNotDevice) {
			t.Fatalf("session principal must not register: %v", err)
		}
		if _, err := svc.Register(ctx, f.devA, fiscalbridge.Registration{Driver: "Bad Driver!"}); err == nil {
			t.Fatal("invalid driver code accepted")
		}

		id1, _, done, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "fiscal_p1", doc(f.est.ID, "fiscal_p1"))
		if err != nil || done || id1 == "" {
			t.Fatalf("enqueue %v %v %v", id1, done, err)
		}
		again, _, _, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "fiscal_p1", doc(f.est.ID, "fiscal_p1"))
		if err != nil || again != id1 {
			t.Fatalf("enqueue is not idempotent: %v %v", again, err)
		}
		if _, err := svc.BridgeStatus(ctx, f.est.ID); !errors.Is(err, fiscalbridge.ErrNoBridge) {
			t.Fatalf("bridge status without bridge: %v", err)
		}

		st, err = svc.Register(ctx, f.devA, fiscalbridge.Registration{Driver: "noop", DeviceStatus: &fiscalbridge.DeviceStatus{Online: true, Message: "pronto"}})
		if err != nil || !st.Registered || st.DeviceID != f.devA.DeviceID || st.DeviceName != "Cassa 1" || !st.Online || st.PendingJobs != 1 {
			t.Fatalf("register %+v %v", st, err)
		}
		if _, err := svc.Register(ctx, f.devA, fiscalbridge.Registration{Driver: "noop"}); err != nil {
			t.Fatal(err)
		}
		if n := countAudit(t, db, "fiscal_bridge.registered", f.devA.DeviceID); n != 1 {
			t.Fatalf("a refresh must not audit again: %d", n)
		}

		jobs, err := svc.Poll(ctx, f.devA, 0, 10)
		if err != nil || len(jobs) != 1 || jobs[0].ID != id1 || jobs[0].Attempt != 1 || jobs[0].Document == nil || jobs[0].Document.Total != 1220 {
			t.Fatalf("poll %+v %v", jobs, err)
		}
		if !jobs[0].LeaseExpiresAt.Equal(t0.Add(fiscalbridge.LeaseDuration)) {
			t.Fatalf("lease %v", jobs[0].LeaseExpiresAt)
		}
		// Leased: not delivered again while the lease holds.
		if again, err := svc.Poll(ctx, f.devA, 0, 10); err != nil || len(again) != 0 {
			t.Fatalf("leased job delivered twice: %+v %v", again, err)
		}

		issued := t0.Add(time.Minute)
		res := fiscalbridge.Result{Outcome: fiscalbridge.OutcomeSucceeded, Ref: "r1", Number: "0001-0001", IssuedAt: &issued}
		if err := svc.Submit(ctx, f.devB, id1, res); !errors.Is(err, fiscalbridge.ErrLeaseLost) {
			t.Fatalf("another device must not complete the job: %v", err)
		}
		if err := svc.Submit(ctx, f.devA, id1, res); err != nil {
			t.Fatal(err)
		}
		if err := svc.Submit(ctx, f.devA, id1, res); err != nil {
			t.Fatalf("duplicate submit must be idempotent: %v", err)
		}
		if err := svc.Submit(ctx, f.devA, id1, fiscalbridge.Result{Outcome: fiscalbridge.OutcomeFailed, Error: "x"}); !errors.Is(err, fiscalbridge.ErrLeaseLost) {
			t.Fatalf("different outcome after completion: %v", err)
		}
		if n := countAudit(t, db, "fiscal_bridge_job.succeeded", id1); n != 1 {
			t.Fatalf("audit of completion %d", n)
		}
		_, got, done, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "fiscal_p1", doc(f.est.ID, "fiscal_p1"))
		if err != nil || !done || got.Number != "0001-0001" {
			t.Fatalf("enqueue after completion %+v %v %v", got, done, err)
		}
		if err := svc.Submit(ctx, f.devA, store.NewID(), res); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("unknown job: %v", err)
		}

		// Device B takes over: A's polls are refused, B's registration is audited.
		if _, err := svc.Register(ctx, f.devB, fiscalbridge.Registration{Driver: "rt"}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Poll(ctx, f.devA, 0, 10); !errors.Is(err, fiscalbridge.ErrNotActiveBridge) {
			t.Fatalf("replaced bridge must be refused: %v", err)
		}
		st, _ = svc.Status(ctx, f.owner, f.est.ID)
		if st.DeviceID != f.devB.DeviceID || st.Driver != "rt" {
			t.Fatalf("status after takeover %+v", st)
		}
		// Offline after OnlineWindow without contact.
		f.clk.Advance(2 * fiscalbridge.OnlineWindow)
		if ds, err := svc.BridgeStatus(ctx, f.est.ID); err != nil || ds.Online {
			t.Fatalf("stale bridge must be offline: %+v %v", ds, err)
		}
		if err := svc.Unregister(ctx, f.devB); err != nil {
			t.Fatal(err)
		}
		if err := svc.Unregister(ctx, f.devB); err != nil {
			t.Fatalf("unregister is idempotent: %v", err)
		}
		if st, _ := svc.Status(ctx, f.owner, f.est.ID); st.Registered {
			t.Fatal("unregistered bridge still active")
		}
	})
}

func TestService_RetriesAndLeases(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		svc := f.svc
		if _, err := svc.Register(ctx, f.devA, fiscalbridge.Registration{Driver: "noop"}); err != nil {
			t.Fatal(err)
		}
		id, _, _, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "k-retry", doc(f.est.ID, "k-retry"))
		if err != nil {
			t.Fatal(err)
		}
		// Retryable failure: queued again with backoff, the fiscal side keeps waiting.
		if jobs, _ := svc.Poll(ctx, f.devA, 0, 10); len(jobs) != 1 {
			t.Fatalf("first delivery %v", jobs)
		}
		paper := fiscalbridge.Result{Outcome: fiscalbridge.OutcomeFailed, Error: "carta finita", Retryable: true}
		if err := svc.Submit(ctx, f.devA, id, paper); err != nil {
			t.Fatal(err)
		}
		if err := svc.Submit(ctx, f.devA, id, paper); err != nil {
			t.Fatalf("duplicate retryable failure: %v", err)
		}
		if _, _, done, _ := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "k-retry", nil); done {
			t.Fatal("retryable failure must not be final")
		}
		if jobs, _ := svc.Poll(ctx, f.devA, 0, 10); len(jobs) != 0 {
			t.Fatal("job offered before its backoff")
		}
		f.clk.Advance(fiscalbridge.Backoff(1))
		jobs, _ := svc.Poll(ctx, f.devA, 0, 10)
		if len(jobs) != 1 || jobs[0].Attempt != 2 {
			t.Fatalf("second delivery %+v", jobs)
		}
		// Lease expiry (agent crashed): redelivered to the bridge with a new attempt.
		f.clk.Advance(fiscalbridge.LeaseDuration + time.Second)
		jobs, _ = svc.Poll(ctx, f.devA, 0, 10)
		if len(jobs) != 1 || jobs[0].Attempt != 3 {
			t.Fatalf("redelivery after lease expiry %+v", jobs)
		}
		// A late success of the same device is still accepted.
		if err := svc.Submit(ctx, f.devA, id, fiscalbridge.Result{Outcome: fiscalbridge.OutcomeSucceeded, Ref: "late"}); err != nil {
			t.Fatal(err)
		}

		// Exhausted: a job that never gets an answer fails after MaxAttempts deliveries.
		id2, _, _, _ := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindVoid, "void:ref-1", map[string]string{"ref": "ref-1"})
		for i := 1; i <= fiscalbridge.MaxAttempts; i++ {
			jobs, err := svc.Poll(ctx, f.devA, 0, 10)
			if err != nil || len(jobs) != 1 || jobs[0].Ref != "ref-1" || jobs[0].Kind != fiscalbridge.KindVoid {
				t.Fatalf("delivery %d: %+v %v", i, jobs, err)
			}
			f.clk.Advance(fiscalbridge.LeaseDuration + time.Second)
		}
		if jobs, _ := svc.Poll(ctx, f.devA, 0, 10); len(jobs) != 0 {
			t.Fatalf("exhausted job delivered again: %+v", jobs)
		}
		if err := svc.RequeueExpiredLeases(ctx); err != nil {
			t.Fatal(err)
		}
		_, res, done, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindVoid, "void:ref-1", nil)
		if err != nil || !done || res.Outcome != fiscalbridge.OutcomeFailed {
			t.Fatalf("exhausted job %v %+v %v (job %s)", done, res, err, id2)
		}

		// Revoking the device (BridgeRevoker) deactivates it and releases its leases.
		id3, _, _, _ := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "k-revoke", doc(f.est.ID, "k-revoke"))
		if jobs, _ := svc.Poll(ctx, f.devA, 0, 10); len(jobs) != 1 {
			t.Fatal("lease before revoke")
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.DeactivateDevice(ctx, tx, f.es, f.devA.DeviceID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Register(ctx, f.devB, fiscalbridge.Registration{Driver: "noop"}); err != nil {
			t.Fatal(err)
		}
		jobs, _ = svc.Poll(ctx, f.devB, 0, 10)
		if len(jobs) != 1 || jobs[0].ID != id3 {
			t.Fatalf("released job not offered to the new bridge: %+v", jobs)
		}
	})
}

func TestService_LongPollWakesOnEnqueue(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		svc := fiscalbridge.NewService(fiscalbridge.Deps{DB: db, Clock: f.clk, Recheck: time.Hour}) // only the notifier can wake it
		if _, err := svc.Register(ctx, f.devA, fiscalbridge.Registration{Driver: "noop"}); err != nil {
			t.Fatal(err)
		}
		got := make(chan []fiscalbridge.Job, 1)
		go func() {
			jobs, _ := svc.Poll(ctx, f.devA, fiscalbridge.MaxWait, 10)
			got <- jobs
		}()
		time.Sleep(50 * time.Millisecond)
		start := time.Now()
		if _, _, _, err := svc.Enqueue(ctx, f.est.ID, fiscalbridge.KindIssue, "k-wake", doc(f.est.ID, "k-wake")); err != nil {
			t.Fatal(err)
		}
		select {
		case jobs := <-got:
			if len(jobs) != 1 || time.Since(start) > 5*time.Second {
				t.Fatalf("woken poll %+v after %v", jobs, time.Since(start))
			}
		case <-time.After(10 * time.Second):
			t.Fatal("long poll not woken by enqueue")
		}
		// A cancelled poll (client gone) returns without error.
		cctx, cancel := context.WithCancel(ctx)
		time.AfterFunc(50*time.Millisecond, cancel)
		if jobs, err := svc.Poll(cctx, f.devA, fiscalbridge.MaxWait, 10); err != nil || len(jobs) != 0 {
			t.Fatalf("cancelled poll %+v %v", jobs, err)
		}
	})
}

func ExampleBackoff() {
	for i := 1; i <= 7; i++ {
		fmt.Print(fiscalbridge.Backoff(i), " ")
	}
	// Output: 15s 30s 1m0s 2m0s 4m0s 8m0s 10m0s
}
