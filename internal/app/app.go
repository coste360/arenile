// Package app is the composition root of Arenile: it wires configuration,
// database, domain services, jobs, the HTTP server and the extensions
// registered through pkg/ext.
package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/config"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// ShutdownTimeout bounds graceful shutdown.
const ShutdownTimeout = 15 * time.Second

type options struct {
	ext     ext.Extensions
	log     *slog.Logger
	clock   clock.Clock
	version string
	signals bool
}

// Option configures New.
type Option func(*options)

// WithExtensions registers extension implementations.
func WithExtensions(x ext.Extensions) Option { return func(o *options) { o.ext = x } }

// WithLogger overrides the default JSON logger on stderr.
func WithLogger(l *slog.Logger) Option { return func(o *options) { o.log = l } }

// WithClock overrides the system clock (tests).
func WithClock(c clock.Clock) Option { return func(o *options) { o.clock = c } }

// WithVersion sets the version reported by /healthz.
func WithVersion(v string) Option { return func(o *options) { o.version = v } }

// WithoutSignalHandling disables SIGINT/SIGTERM handling in Run (tests,
// embedding in another program).
func WithoutSignalHandling() Option { return func(o *options) { o.signals = false } }

// App is a fully wired Arenile instance.
type App struct {
	cfg      config.Config
	opts     options
	log      *slog.Logger
	db       *store.DB
	keys     *crypto.KeyRing
	registry *Registry
	runner   *jobs.Runner
	handler  http.Handler

	Auth           *auth.Service
	Establishments *establishment.Service
	Outbox         *event.Outbox
	Bus            ext.EventBus

	mu    sync.Mutex
	addr  string
	ready chan struct{}
}

// NewLogger returns the standard JSON logger.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// New wires an App: it validates the configuration, opens the database,
// runs migrations when ARENILE_AUTO_MIGRATE is true and builds services.
func New(cfg config.Config, opts ...Option) (*App, error) {
	o := options{clock: clock.System{}, version: "dev", signals: true}
	for _, fn := range opts {
		fn(&o)
	}
	if o.log == nil {
		o.log = NewLogger(os.Stderr, cfg.LogLevel)
	}
	if err := cfg.ValidateServe(); err != nil {
		return nil, err
	}
	keys, err := crypto.NewKeyRing(cfg.SecretKeys...)
	if err != nil {
		return nil, fmt.Errorf("app: key ring: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := OpenDB(ctx, cfg)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, opts: o, log: o.log, db: db, keys: keys, ready: make(chan struct{})}
	if err := a.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := a.wire(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := a.bootstrapOwner(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return a, nil
}

// OpenDB opens the database selected by the configuration.
func OpenDB(ctx context.Context, cfg config.Config) (*store.DB, error) {
	opts := store.Options{SQLitePath: cfg.SQLitePath()}
	if cfg.UsesPostgres() {
		opts = store.Options{PostgresURL: cfg.DatabaseURL}
	}
	db, err := store.Open(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("app: open database: %w", err)
	}
	return db, nil
}

func (a *App) migrate(ctx context.Context) error {
	m, err := a.db.Migrator()
	if err != nil {
		return err
	}
	if a.cfg.AutoMigrate {
		applied, err := m.Up(ctx)
		if err != nil {
			return err
		}
		if len(applied) > 0 {
			a.log.Info("migrations applied", slog.Any("versions", applied))
		}
		return nil
	}
	aligned, err := m.Aligned(ctx)
	if err != nil {
		return err
	}
	if !aligned {
		a.log.Warn("database has pending migrations: run `arenile migrate up`")
	}
	return nil
}

func (a *App) wire() error {
	c := a.opts.clock
	x := a.opts.ext
	aw := audit.NewWriter(c)
	authSvc, err := auth.NewService(a.db, c, a.keys, aw, a.log, auth.Options{RequireTOTPForOwners: a.cfg.RequireTOTPForOwners})
	if err != nil {
		return err
	}
	a.Auth = authSvc
	a.Establishments = establishment.NewService(a.db)
	a.Outbox = event.NewOutbox(c)
	a.registry = NewRegistry(c, x)
	if x.EventBus != nil {
		a.Bus = x.EventBus
	} else {
		a.Bus = event.NewBroker(a.log, 64)
	}
	dispatcher := event.NewDispatcher(a.db, c, x.EventSinks, a.Bus, a.log)
	deliverer := event.NewDeliverer(a.db, c, a.keys, nil, aw, a.log, event.DeliveryOptions{})

	a.runner = jobs.NewRunner(a.log)
	for _, j := range jobs.CoreJobs(jobs.CoreDeps{
		Outbox:          dispatcher,
		Webhooks:        deliverer,
		SessionsCleanup: authSvc.CleanupExpiredSessions,
		IdempotencyCleanup: func(ctx context.Context) (int64, error) {
			return store.DeleteExpiredIdempotencyKeys(ctx, a.db, c.Now())
		},
		Log: a.log,
	}) {
		if err := a.runner.Register(j); err != nil {
			return err
		}
	}

	a.handler = httpserver.New(httpserver.Deps{
		DB: a.db, Clock: c, Log: a.log, Version: a.opts.version,
		Auth: authSvc, Establishments: a.Establishments, Bus: a.Bus,
		Readiness:      readiness{db: a.db},
		TrustedProxies: a.cfg.TrustedProxies,
		SecureCookies:  a.cfg.SecureCookies(),
		Extensions:     x,
	})
	return nil
}

// bootstrapOwner creates the first owner from ARENILE_BOOTSTRAP_OWNER_* when
// the database has no users yet (ADR 0006). It is a no-op afterwards, so the
// variables can stay set across restarts.
func (a *App) bootstrapOwner(ctx context.Context) error {
	b := a.cfg.BootstrapOwner
	if !b.Enabled() {
		return nil
	}
	res, err := a.Auth.CreateOwner(ctx, auth.CreateOwnerInput{
		Email: b.Email, Name: b.Name, Password: b.Password, OrganizationName: b.Organization,
		OnlyIfNoUsers: true, Source: "bootstrap",
	})
	switch {
	case errors.Is(err, auth.ErrUsersExist), errors.Is(err, auth.ErrUserExists):
		a.log.Debug("bootstrap owner skipped: users already exist")
		return nil
	case err != nil:
		return fmt.Errorf("app: bootstrap owner: %w", err)
	}
	a.log.Info("bootstrap owner created", slog.String("user_id", res.User.ID), slog.String("organization_id", res.Organization.ID))
	return nil
}

// Handler returns the root HTTP handler.
func (a *App) Handler() http.Handler { return a.handler }

// DB returns the database.
func (a *App) DB() *store.DB { return a.db }

// Registry returns the extension registry.
func (a *App) Registry() *Registry { return a.registry }

// Jobs returns the job runner.
func (a *App) Jobs() *jobs.Runner { return a.runner }

// Ready is closed once the server listens.
func (a *App) Ready() <-chan struct{} { return a.ready }

// Addr returns the listening address once Ready is closed.
func (a *App) Addr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.addr
}

// Close releases resources when Run is not used.
func (a *App) Close() error { return a.db.Close() }

// Run serves HTTP and runs background jobs until ctx is cancelled or the
// process receives SIGINT/SIGTERM, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	if a.opts.signals {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
	}
	defer a.db.Close()

	jobsCtx, stopJobs := context.WithCancel(context.WithoutCancel(ctx))
	defer stopJobs()
	a.runner.Start(jobsCtx)

	errCh := make(chan error, 2)
	var servers []*http.Server
	newServer := func(h http.Handler) *http.Server {
		return &http.Server{
			Handler: h, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
			WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
			ErrorLog: slog.NewLogLogger(a.log.Handler(), slog.LevelWarn),
		}
	}

	if len(a.cfg.TLSAutocertDomains) > 0 {
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(a.cfg.TLSAutocertDomains...),
			Cache:      autocert.DirCache(filepath.Join(a.cfg.DataDir, "autocert")),
		}
		httpsSrv := newServer(a.handler)
		httpsSrv.TLSConfig = &tls.Config{GetCertificate: m.GetCertificate, MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1", "acme-tls/1"}}
		httpSrv := newServer(m.HTTPHandler(nil))
		lnTLS, err := net.Listen("tcp", ":443") //nolint:gosec // G102: ACME/HTTPS must listen on all interfaces
		if err != nil {
			return fmt.Errorf("app: listen :443: %w", err)
		}
		ln, err := net.Listen("tcp", ":80") //nolint:gosec // G102: ACME HTTP-01 must listen on all interfaces
		if err != nil {
			_ = lnTLS.Close()
			return fmt.Errorf("app: listen :80: %w", err)
		}
		servers = append(servers, httpsSrv, httpSrv)
		a.setAddr(lnTLS.Addr().String())
		go func() { errCh <- serveErr(httpsSrv.ServeTLS(lnTLS, "", "")) }()
		go func() { errCh <- serveErr(httpSrv.Serve(ln)) }()
	} else {
		ln, err := net.Listen("tcp", a.cfg.Addr)
		if err != nil {
			return fmt.Errorf("app: listen %s: %w", a.cfg.Addr, err)
		}
		srv := newServer(a.handler)
		servers = append(servers, srv)
		a.setAddr(ln.Addr().String())
		go func() { errCh <- serveErr(srv.Serve(ln)) }()
	}
	a.log.Info("arenile listening", slog.String("addr", a.Addr()), slog.String("version", a.opts.version),
		slog.String("database", string(a.db.Kind)), slog.Bool("dev", a.cfg.Dev))
	close(a.ready)

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errCh:
	}
	a.log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	for _, s := range servers {
		if err := s.Shutdown(shutdownCtx); err != nil {
			a.log.Warn("http shutdown", slog.String("error", err.Error()))
		}
	}
	stopJobs()
	a.runner.Wait()
	return runErr
}

func (a *App) setAddr(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.addr = s
}

func serveErr(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

type readiness struct{ db *store.DB }

func (r readiness) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }

func (r readiness) MigrationsAligned(ctx context.Context) (bool, error) {
	m, err := r.db.Migrator()
	if err != nil {
		return false, err
	}
	return m.Aligned(ctx)
}
