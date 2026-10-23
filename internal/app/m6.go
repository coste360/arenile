package app

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/meta"
	"github.com/coste360/arenile/internal/fiscalbridge"
	desktophttp "github.com/coste360/arenile/internal/http/desktop"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Compile-time checks of the M6/M7 ports (ports & adapters).
var (
	_ device.Principals              = (*auth.Service)(nil)
	_ device.BridgeRevoker           = (*fiscalbridge.Service)(nil)
	_ device.Instance                = instance{}
	_ device.TokenAuthenticator      = (*device.Service)(nil)
	_ fiscalbridge.Queue             = (*fiscalbridge.Service)(nil)
	_ ext.FiscalDriver               = (*fiscalbridge.Driver)(nil)
	_ meta.Drivers                   = (*Registry)(nil)
	_ meta.RuntimeProvider           = control.Control(nil)
	_ desktophttp.UsersCounter       = usersCounter{}
	_ interface{ Jobs() []jobs.Job } = (*device.Service)(nil)
	_ interface{ Jobs() []jobs.Job } = (*fiscalbridge.Service)(nil)
)

// WithDesktop runs the core as the desktop edition (spec 29.1): c reports
// the edition and LAN mode to /meta, backs /api/desktop/v1 and tells the
// pairing QR which address to use. Passed by cmd/arenile-desktop
// (internal/desktop.Runtime).
func WithDesktop(c control.Control) Option { return func(o *options) { o.desktop = c } }

// wireDevices builds the M6/M7 services: instance metadata, the fiscal
// bridge (and its "bridge" fiscal driver in the registry) and the paired
// devices. Requires a.Auth and a.registry.
func (a *App) wireDevices(aw *audit.Writer) {
	c := a.opts.clock
	var rt meta.RuntimeProvider = meta.ServerRuntime{}
	if a.opts.desktop != nil {
		rt = a.opts.desktop
	}
	a.Meta = meta.NewService(meta.Deps{Version: a.opts.version, Clock: c, Runtime: rt, Drivers: a.registry})
	a.FiscalBridge = fiscalbridge.NewService(fiscalbridge.Deps{DB: a.db, Clock: c, Audit: aw, Log: a.log})
	a.registry.fiscal[fiscalbridge.Code] = &fiscalbridge.Driver{Queue: a.FiscalBridge}
	a.Devices = device.NewService(device.Deps{
		DB: a.db, Clock: c, Audit: aw, Log: a.log,
		Principals: a.Auth, Bridge: a.FiscalBridge,
		Instance: instance{baseURL: a.cfg.EffectiveBaseURL(), desktop: a.opts.desktop},
	})
}

// deviceJobs are the background jobs contributed by the M6/M7 services.
func (a *App) deviceJobs() []jobs.Job {
	return append(a.Devices.Jobs(), a.FiscalBridge.Jobs()...)
}

// ServeListener serves the app handler on an additional listener until the
// app shuts down (desktop LAN mode: a PrivateListener wrapped in TLS). The
// returned function closes this listener only (LAN mode switched off).
func (a *App) ServeListener(ln net.Listener) (stop func(context.Context) error) {
	srv := &http.Server{
		Handler: a.handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
	}
	a.mu.Lock()
	a.extra = append(a.extra, srv)
	a.mu.Unlock()
	go func() { _ = serveErr(srv.Serve(ln)) }()
	return srv.Shutdown
}

// instance implements device.Instance.
type instance struct {
	baseURL string
	desktop control.Control
}

func (i instance) PairingTarget(context.Context) (device.PairingTarget, error) {
	if i.desktop == nil {
		return device.PairingTarget{URL: i.baseURL}, nil
	}
	d := i.desktop.Runtime().Desktop
	if d == nil || d.Mode != meta.DesktopLAN || d.LANURL == "" {
		return device.PairingTarget{}, device.ErrLANModeRequired
	}
	return device.PairingTarget{URL: d.LANURL, Fingerprint: d.LANFingerprint}, nil
}

// usersCounter implements desktophttp.UsersCounter.
type usersCounter struct{ db *store.DB }

func (u usersCounter) AnyUser(ctx context.Context) (bool, error) { return store.HasUsers(ctx, u.db) }
