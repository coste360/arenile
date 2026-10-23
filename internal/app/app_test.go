package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/app/builtin"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/config"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

func devConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Parse(func(k string) (string, bool) {
		switch k {
		case config.EnvDev:
			return "true", true
		case config.EnvDataDir:
			return t.TempDir(), true
		case config.EnvAddr:
			return "127.0.0.1:0", true
		case config.EnvLogLevel:
			return "error", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestSmoke_ServeHealthAndReady starts the full application on a random
// port with a temporary SQLite database.
func TestSmoke_ServeHealthAndReady(t *testing.T) {
	cfg := devConfig(t)
	a, err := New(cfg, WithVersion("smoke"), WithoutSignalHandling(), WithLogger(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	select {
	case <-a.Ready():
	case err := <-done:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server not ready")
	}
	base := "http://" + a.Addr()
	for path, want := range map[string]string{"/healthz": "ok", "/readyz": "ok"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != 200 || body["status"] != want {
			t.Fatalf("%s: %d %v", path, resp.StatusCode, body)
		}
	}
	if _, err := os.Stat(cfg.SQLitePath()); err != nil {
		t.Fatalf("sqlite file not created: %v", err)
	}
	wantJobs := []string{
		"outbox.dispatch", "webhooks.deliver", "sessions.cleanup", "idempotency.cleanup",
		"inventory.sweep_expired_holds", "bookings.expire_holds", "bookings.complete",
		"fiscal.issue_pending",
		"payments.reconcile",
		"notify.deliver",
		"device.cleanup_pairing_codes", "fiscalbridge.requeue_expired_leases",
		"webhooks.retention",
	}
	if got := a.Jobs().Jobs(); !sameSet(got, wantJobs) {
		t.Fatalf("registered jobs: got %v, want %v", got, wantJobs)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(ShutdownTimeout + 5*time.Second):
		t.Fatal("graceful shutdown timed out")
	}
}

// TestNew_BootstrapOwner creates the first owner from the environment once,
// then is a no-op on restart even with different values.
func TestNew_BootstrapOwner(t *testing.T) {
	dir := t.TempDir()
	parse := func(email string) config.Config {
		cfg, err := config.Parse(func(k string) (string, bool) {
			switch k {
			case config.EnvDev:
				return "true", true
			case config.EnvDataDir:
				return dir, true
			case config.EnvBootstrapOwnerEmail:
				return email, true
			case config.EnvBootstrapOwnerName:
				return "Titolare", true
			case config.EnvBootstrapOwnerOrg:
				return "Lido Prova", true
			case config.EnvAdminPassword:
				return "una-password-lunga-e-sicura", true
			}
			return "", false
		})
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	ctx := context.Background()
	for i, email := range []string{"owner@lido.it", "altro@lido.it"} {
		a, err := New(parse(email), WithoutSignalHandling(), WithLogger(slog.New(slog.DiscardHandler)))
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if _, err := store.GetUserByEmail(ctx, a.DB(), "owner@lido.it"); err != nil {
			t.Fatalf("run %d: bootstrap owner missing: %v", i, err)
		}
		if _, err := store.GetUserByEmail(ctx, a.DB(), "altro@lido.it"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("run %d: second bootstrap must be skipped, got %v", i, err)
		}
		_ = a.Close()
	}
}

func TestNew_RequiresServeConfig(t *testing.T) {
	cfg, _ := config.Parse(func(k string) (string, bool) {
		if k == config.EnvDataDir {
			return t.TempDir(), true
		}
		return "", false
	})
	if _, err := New(cfg, WithLogger(slog.New(slog.DiscardHandler))); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("production without base url/key must fail: %v", err)
	}
}

type fakeProvider struct{ builtin.Manual }

func (fakeProvider) Code() string { return "stripe" }

func TestRegistry_CoreStripe(t *testing.T) {
	r := NewRegistry(clock.System{}, ext.Extensions{})
	p, ok := r.PaymentProvider("stripe")
	if !ok {
		t.Fatal("stripe must be a core provider")
	}
	if _, isCore := p.(*stripe.Driver); !isCore || !p.Capabilities().Online {
		t.Fatalf("stripe provider = %T", p)
	}
	if got := r.PaymentProviderCodes(); len(got) != 2 || got[0] != "manual" || got[1] != "stripe" {
		t.Fatal(got)
	}
}

func TestRegistry_BuiltinsAndExtensions(t *testing.T) {
	r := NewRegistry(clock.System{}, ext.Extensions{PaymentProviders: []ext.PaymentProvider{fakeProvider{}}})
	if _, ok := r.PaymentProvider("manual"); !ok {
		t.Fatal("manual provider must be registered")
	}
	if _, ok := r.PaymentProvider("stripe"); !ok {
		t.Fatal("extension provider must be registered")
	}
	if got := r.PaymentProviderCodes(); len(got) != 2 || got[0] != "manual" {
		t.Fatal(got)
	}
	f, ok := r.FiscalDriver("none")
	if !ok {
		t.Fatal("none fiscal driver")
	}
	res, err := f.Issue(context.Background(), ext.FiscalDocumentRequest{})
	if err != nil || res.Ref == "" {
		t.Fatal(res, err)
	}
	m, _ := r.PaymentProvider("manual")
	if _, err := m.Refund(context.Background(), ext.RefundRequest{}); !errors.Is(err, ext.ErrNotSupported) {
		t.Fatal("manual refunds are recorded manually")
	}
	pr, err := m.CreatePayment(context.Background(), ext.CreatePaymentRequest{Amount: 1000})
	if err != nil || pr.Mode != ext.PaymentModeNone {
		t.Fatal(pr, err)
	}
}

func TestSeedDemo_Idempotent(t *testing.T) {
	db := storetest.SQLite(t)
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 5, 1, 8, 0, 0, 0, time.UTC))
	first, err := SeedDemo(ctx, db, c)
	if err != nil || !first.Created {
		t.Fatalf("first seed: %+v %v", first, err)
	}
	second, err := SeedDemo(ctx, db, c)
	if err != nil || second.Created || second.EstablishmentID != first.EstablishmentID {
		t.Fatalf("second seed: %+v %v", second, err)
	}
	e, err := store.GetEstablishmentBySlug(ctx, db, DemoSlug)
	if err != nil || !e.BookingEnabled || e.Timezone != "Europe/Rome" || len(e.EnabledLocales) != 4 {
		t.Fatalf("demo establishment: %+v %v", e, err)
	}
	var orgs int
	_ = db.NewSelect().TableExpr("organizations").ColumnExpr("count(*)").Scan(ctx, &orgs)
	if orgs != 1 {
		t.Fatalf("organizations: %d", orgs)
	}
}

// sameSet reports whether a and b hold the same strings, ignoring order.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		if seen[s] == 0 {
			return false
		}
		seen[s]--
	}
	return true
}
