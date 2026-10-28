//go:build integration

package acceptance

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/config"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/internal/domain/setup"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

const (
	ownerEmail    = "titolare@acceptance.example"
	ownerPassword = "una-password-lunga-per-i-test"
	envSeed       = "ARENILE_ACCEPTANCE_SEED"
)

// dayStart is the simulated morning: a Saturday of July in high season, at
// 08:00 in Europe/Rome (06:00 UTC).
var dayStart = time.Date(2027, 7, 10, 6, 0, 0, 0, time.UTC)

// env is a running application with the configured demo establishment.
type env struct {
	t     *testing.T
	app   *app.App
	db    *store.DB
	clock *clock.Fake
	srv   *httptest.Server
	loc   *time.Location
	// stripe is the fake Stripe API (ARENILE_STRIPE_API_URL) of the online
	// operations; its clock is the scenario clock.
	stripe *stripetest.Server

	estID  string
	es     store.EstablishmentScope
	season *store.Season
	today  calendar.Date

	basePack   store.Pack
	cabinExtra *store.Extra
	cabinPool  *store.Pool
	passSpots  []store.Spot // rows A and B
	dailySpots []store.Spot // rows C..H

	gazeboPack  store.Pack
	gazeboZone  store.Zone   // turned into a zone-mode zone by the online phase
	gazeboSpots []store.Spot // its spots
}

// eachDialect runs fn on SQLite (file, WAL) and, when configured,
// PostgreSQL.
func eachDialect(t *testing.T, fn func(t *testing.T, e *env)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { fn(t, newEnv(t, "")) })
	t.Run("postgres", func(t *testing.T) {
		base := os.Getenv(storetest.EnvPostgresURL)
		if base == "" {
			t.Skipf("%s not set: skipping PostgreSQL", storetest.EnvPostgresURL)
		}
		fn(t, newEnv(t, freshPostgres(t, base)))
	})
}

// freshPostgres creates a database dropped at the end of the test and
// returns its URL (the same approach as storetest.Postgres, which returns
// an open handle instead of a URL).
func freshPostgres(t *testing.T, base string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := store.Open(ctx, store.Options{PostgresURL: base})
	if err != nil {
		t.Fatalf("open postgres admin: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "arenile_acceptance_" + hex.EncodeToString(b)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close()
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		_, _ = admin.ExecContext(cctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close()
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// newEnv boots the full application with a fake clock, seeds the demo
// establishment (classic template, prices) and its owner.
func newEnv(t *testing.T, databaseURL string) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	c := clock.NewFake(dayStart)
	fake := stripetest.New(t)
	fake.Now = c.Now
	cfg, err := config.Parse(func(k string) (string, bool) {
		switch k {
		case config.EnvDev:
			return "true", true
		case config.EnvDataDir:
			return dir, true
		case config.EnvAutoMigrate:
			return "true", true
		case config.EnvLogLevel:
			return "error", true
		case config.EnvDatabaseURL:
			return databaseURL, databaseURL != ""
		case config.EnvStripeAPIURL:
			return fake.URL, true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	// Many customers from one address: the per-IP public limits (spec 17.2)
	// are raised; the per-establishment checkout limit still applies.
	// Server errors are kept and printed only when the test fails, so a 500
	// in CI shows its cause.
	logs := &lockedBuffer{}
	t.Cleanup(func() {
		if t.Failed() && logs.Len() > 0 {
			t.Logf("server errors:\n%s", logs.String())
		}
	})
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelError}))
	a, err := app.New(cfg, app.WithClock(c), app.WithoutSignalHandling(), app.WithLogger(logger),
		app.WithPublicRateLimits(100000, 100000))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })

	seed, err := app.SeedDemo(ctx, a.DB(), c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Setup.SeedDemo(ctx, seed.EstablishmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Auth.CreateOwner(ctx, auth.CreateOwnerInput{
		Email: ownerEmail, Name: "Titolare Prova", Password: ownerPassword,
		OrganizationName: app.DemoOrganizationName, Source: "seed",
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, app: a, db: a.DB(), clock: c, srv: srv, loc: loc, estID: seed.EstablishmentID, stripe: fake}
	e.loadWorld(ctx)
	return e
}

// loadWorld reads the demo inventory the scenario books (test-side reads
// of the store, never writes).
func (e *env) loadWorld(ctx context.Context) {
	t := e.t
	es, err := store.SystemEstablishmentScope(ctx, e.db, e.estID)
	if err != nil {
		t.Fatal(err)
	}
	e.es = es
	est, err := store.GetEstablishment(ctx, e.db, es)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := time.LoadLocation(est.Timezone); err == nil {
		e.loc = l
	}
	e.today = calendar.In(e.clock.Now(), e.loc)
	if e.season, err = store.GetSeasonByYear(ctx, e.db, es, e.today.Year()); err != nil {
		t.Fatalf("demo season: %v", err)
	}
	packs, err := store.ListAllPacks(ctx, e.db, es)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range packs {
		switch p.Code {
		case setup.PackBase:
			e.basePack = p
		case setup.PackGazebo:
			e.gazeboPack = p
		}
	}
	extras, err := store.ListAllExtras(ctx, e.db, es)
	if err != nil {
		t.Fatal(err)
	}
	for i := range extras {
		if extras[i].Code == "cabin" {
			e.cabinExtra = &extras[i]
		}
	}
	var pools []store.Pool
	if err := e.db.NewSelect().Model(&pools).Where("pl.establishment_id = ?", e.estID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	for i := range pools {
		if pools[i].ComponentCode == "cabin" {
			e.cabinPool = &pools[i]
		}
	}
	if err := e.db.NewSelect().Model(&e.gazeboZone).Where("z.establishment_id = ?", e.estID).Where("z.code = ?", "gazebo").Scan(ctx); err != nil {
		t.Fatalf("gazebo zone: %v", err)
	}
	maps, err := store.ListMaps(ctx, e.db, es, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range maps.Items {
		spots, err := store.ListMapSpots(ctx, e.db, es, m.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, sp := range spots {
			if sp.Active && sp.ZoneID == e.gazeboZone.ID {
				e.gazeboSpots = append(e.gazeboSpots, sp)
				continue
			}
			if !sp.Active || sp.Kind != "umbrella" {
				continue
			}
			if sp.RowLabel == "A" || sp.RowLabel == "B" {
				e.passSpots = append(e.passSpots, sp)
			} else {
				e.dailySpots = append(e.dailySpots, sp)
			}
		}
	}
	if e.gazeboPack.ID == "" || len(e.gazeboSpots) < 3 {
		t.Fatalf("demo gazebos incomplete: pack %q spots %d", e.gazeboPack.ID, len(e.gazeboSpots))
	}
	if e.basePack.ID == "" || e.cabinExtra == nil || e.cabinPool == nil || len(e.passSpots) < 10 || len(e.dailySpots) < 60 {
		t.Fatalf("demo world incomplete: pack %q cabin %v pool %v spots %d/%d", e.basePack.ID, e.cabinExtra != nil, e.cabinPool != nil, len(e.passSpots), len(e.dailySpots))
	}
}

// runJobs runs every registered background job once (hold expiry, booking
// expiry, ...) and then drains the outbox, so that every committed event
// reaches the SSE bus.
func (e *env) runJobs() {
	e.t.Helper()
	ctx := context.Background()
	for _, name := range e.app.Jobs().Jobs() {
		if err := e.app.Jobs().RunNow(ctx, name); err != nil {
			e.t.Fatalf("job %s: %v", name, err)
		}
	}
	e.drainOutbox()
}

// drainOutbox dispatches pending outbox events until none is left.
func (e *env) drainOutbox() {
	e.t.Helper()
	ctx := context.Background()
	for range 1000 {
		pending, err := store.ListPendingOutboxEvents(ctx, e.db, 1)
		if err != nil {
			e.t.Fatal(err)
		}
		if len(pending) == 0 {
			return
		}
		if err := e.app.Jobs().RunNow(ctx, jobs.JobOutboxDispatch); err != nil {
			e.t.Fatalf("outbox dispatch: %v", err)
		}
	}
	e.t.Fatal("outbox not drained")
}

// path builds an establishment-scoped admin API path.
func (e *env) path(suffix string) string {
	return "/api/admin/v1/establishments/" + e.estID + suffix
}

// lockedBuffer is a bytes.Buffer safe for the concurrent writes of the
// server's request goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
