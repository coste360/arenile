// Package httpserver assembles the HTTP router of Arenile: middlewares,
// the handlers generated from api/openapi.yaml, the embedded SPAs and the
// routes contributed by extensions.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/catalog"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/customer"
	devicedomain "github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/meta"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/season"
	"github.com/coste360/arenile/internal/domain/setup"
	"github.com/coste360/arenile/internal/domain/team"
	"github.com/coste360/arenile/internal/fiscalbridge"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	desktophttp "github.com/coste360/arenile/internal/http/desktop"
	devicehttp "github.com/coste360/arenile/internal/http/device"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/http/public"
	"github.com/coste360/arenile/internal/http/sse"
	"github.com/coste360/arenile/internal/http/static"
	"github.com/coste360/arenile/internal/http/webhooks"
	"github.com/coste360/arenile/pkg/ext"
)

// ReadinessChecker reports whether the service can serve traffic.
type ReadinessChecker interface {
	// Ping checks the database connection.
	Ping(ctx context.Context) error
	// MigrationsAligned reports whether no migration is pending.
	MigrationsAligned(ctx context.Context) (bool, error)
}

// Deps are the dependencies of the router.
type Deps struct {
	DB             bun.IDB
	Clock          clock.Clock
	Log            *slog.Logger
	Version        string
	Auth           *auth.Service
	Establishments *establishment.Service
	// Configuration services (M1/M2); nil ones answer as not configured.
	Team     *team.Service
	Seasons  *season.Service
	Mapping  *mapping.Service
	Catalog  *catalog.Service
	Pricing  *pricingconfig.Service
	Policies *pricingconfig.PolicyService
	Setup    *setup.Service
	// M3 services (bookings from the panel and cash desk).
	Customers *customer.Service
	Bookings  *booking.Service
	Inventory *inventory.Service
	Passes    *pass.Service
	Payments  *payment.Service
	Cash      *cash.Service
	Fiscal    *fiscal.Service
	// M4 services (public page, online payments, notifications).
	Online   *online.Service
	Checkout *checkout.Service
	Notify   *notify.Service
	Gateway  *gateway.Service
	// M6/M7: instance metadata, paired devices, fiscal bridge and the
	// desktop edition control (nil on the server edition).
	Meta         *meta.Service
	Devices      *devicedomain.Service
	FiscalBridge *fiscalbridge.Service
	Desktop      control.Control
	DesktopUsers desktophttp.UsersCounter
	// PairingsPerMinute overrides the per-IP limit of POST /api/device/v1/pair (default 10).
	PairingsPerMinute int
	Bus               ext.EventBus
	Readiness         ReadinessChecker
	TrustedProxies    []netip.Prefix
	SecureCookies     bool
	Static            *static.Handler
	// Metrics, when non-nil, is mounted at /metrics (disabled by default).
	Metrics http.Handler
	// Extensions contribute routes, a tenant resolver and admin UI plugins.
	Extensions ext.Extensions
	// SSEHeartbeat overrides the SSE heartbeat (tests).
	SSEHeartbeat time.Duration
	// PublicReadsPerMinute and PublicCreationsPerMinute override the public
	// API limits (defaults 60 and 10, spec 17.2).
	PublicReadsPerMinute     int
	PublicCreationsPerMinute int
}

// Paths that do not require authentication under /api/admin/,
// /api/device/ and /api/desktop/.
const (
	loginPath      = "/api/admin/v1/auth/login"
	authPathPrefix = "/api/admin/v1/auth/"
	metaPath       = "/api/admin/v1/meta"
	pairPath       = "/api/device/v1/pair"
	firstRunPath   = "/api/desktop/v1/first-run"
)

// New builds the root handler.
func New(d Deps) http.Handler {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Static == nil {
		d.Static = static.New()
	}
	reads, creations := d.PublicReadsPerMinute, d.PublicCreationsPerMinute
	if reads <= 0 {
		reads = 60
	}
	if creations <= 0 {
		creations = 10
	}
	pairings := d.PairingsPerMinute
	if pairings <= 0 {
		pairings = 10
	}
	sessionTTL := 7 * 24 * time.Hour
	if d.Auth != nil {
		sessionTTL = d.Auth.SessionTTL()
	}

	var pluginOrigins []string
	for _, p := range d.Extensions.AdminUIPlugins {
		if o := originOf(p.PageURL); o != "" {
			pluginOrigins = append(pluginOrigins, o)
		}
	}

	r := chi.NewRouter()
	r.Use(
		chimw.GetHead,
		middleware.RequestContext(d.TrustedProxies),
		middleware.AccessLog(d.Log),
		middleware.Recoverer(d.Log),
		middleware.SecurityHeaders(middleware.SecurityOptions{
			EmbedOrigins:      d.Establishments.EmbedOrigins,
			AdminFrameSources: pluginOrigins,
			Log:               d.Log,
		}),
		middleware.OnPrefix("/api/", middleware.LimitBody(1<<20), middleware.RequireJSON),
		middleware.OnPrefix("/api/admin/",
			middleware.Authenticate(adminAuthenticator(d), d.Log),
			middleware.RequireAuth([]string{loginPath, metaPath}, authPathPrefix),
			middleware.CSRF(d.SecureCookies, sessionTTL),
		),
		// M6/M7 device API: device tokens only, no cookies (no CSRF).
		middleware.OnPrefix("/api/device/",
			middleware.OnPrefix(pairPath, middleware.PublicRateLimit(
				auth.NewRateLimiter(d.Clock, pairings, time.Minute),
				auth.NewRateLimiter(d.Clock, pairings, time.Minute),
			)),
			middleware.Authenticate(devicedomain.DeviceOnly{Devices: deviceAuthenticator(d)}, d.Log),
			middleware.RequireAuth([]string{pairPath}, ""),
		),
		desktopGuard(d, sessionTTL),
		middleware.OnPrefix("/api/public/", middleware.PublicRateLimit(
			auth.NewRateLimiter(d.Clock, reads, time.Minute),
			auth.NewRateLimiter(d.Clock, creations, time.Minute),
		)),
		middleware.OnPrefix("/api/", middleware.Idempotency(middleware.IdempotencyOptions{
			DB: d.DB, Clock: d.Clock, Log: d.Log, Skip: []string{authPathPrefix, "/api/device/", "/api/desktop/"},
		})),
	)

	srv := &server{
		Handlers: &admin.Handlers{
			Auth: d.Auth, Establishments: d.Establishments, Log: d.Log, SecureCookies: d.SecureCookies,
			Team: d.Team, Seasons: d.Seasons, Mapping: d.Mapping, Catalog: d.Catalog,
			Pricing: d.Pricing, Policies: d.Policies, Setup: d.Setup,
			Customers: d.Customers, Bookings: d.Bookings, Inventory: d.Inventory, Passes: d.Passes,
			Payments: d.Payments, Cash: d.Cash, Fiscal: d.Fiscal,
			Online: d.Online, Notify: d.Notify, Gateway: d.Gateway,
			Meta: d.Meta, Devices: d.Devices, FiscalBridge: d.FiscalBridge,
			Stream: &sse.Server{Bus: d.Bus, Heartbeat: d.SSEHeartbeat, Log: d.Log},
		},
		deviceHandlers: &deviceHandlers{Devices: d.Devices, FiscalBridge: d.FiscalBridge, Log: d.Log},
		desktopHandlers: &desktopHandlers{
			Control: d.Desktop, Auth: d.Auth, Users: d.DesktopUsers, Log: d.Log, SecureCookies: d.SecureCookies,
		},
		publicHandlers: &public.Handlers{
			Establishments: d.Establishments, Online: d.Online, Checkout: d.Checkout,
			TenantResolver: d.Extensions.TenantResolver, Log: d.Log,
		},
		webhookHandlers: &webhooks.Handlers{Gateway: d.Gateway, Log: d.Log},
		ops:             &ops{readiness: d.Readiness, version: d.Version},
	}
	api.HandlerWithOptions(srv, api.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: bindErrorHandler,
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin/", http.StatusFound) })
	r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	r.Get("/admin/*", d.Static.Admin)
	r.Get("/b/{slug}", d.Static.Booking)
	r.Get("/b/{slug}/*", d.Static.Booking)
	r.Get("/embed.js", d.Static.EmbedJS)
	if d.Metrics != nil {
		r.Handle("/metrics", d.Metrics)
	}
	if d.Extensions.HTTPRoutes != nil {
		d.Extensions.HTTPRoutes(r)
	}
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			problem.Write(w, r, problem.NotFound)
			return
		}
		http.NotFound(w, r)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		problem.Write(w, r, problem.MethodNotAllowed)
	})
	return r
}

// server composes the handler groups into api.ServerInterface: admin
// (/api/admin/v1), public (/api/public/v1), webhooks (/webhooks/payments)
// and the ops endpoints. The groups have disjoint method sets.
type server struct {
	*admin.Handlers
	*publicHandlers
	*webhookHandlers
	*deviceHandlers
	*desktopHandlers
	ops *ops
}

// Aliases give the embedded groups distinct field names.
type (
	publicHandlers  = public.Handlers
	webhookHandlers = webhooks.Handlers
	deviceHandlers  = devicehttp.Handlers
	desktopHandlers = desktophttp.Handlers
)

// adminAuthenticator dispatches bearer credentials on their scheme: device
// tokens to the device service, API keys to auth (strategy, M6/M7).
func adminAuthenticator(d Deps) middleware.Authenticator {
	if d.Devices == nil {
		return d.Auth
	}
	return devicedomain.BearerAuthenticator{Auth: d.Auth, Devices: d.Devices}
}

// desktopGuard protects /api/desktop/ on the desktop edition: loopback
// only, session cookie, CSRF (first-run is public and checks the setup
// token). On the server edition there is no guard and the handlers answer
// 404 not_found, so the API does not reveal desktop-only behaviour.
func desktopGuard(d Deps, sessionTTL time.Duration) middleware.Middleware {
	if d.Desktop == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return middleware.OnPrefix("/api/desktop/",
		middleware.LoopbackOnly(),
		middleware.Authenticate(d.Auth, d.Log),
		middleware.RequireAuth([]string{firstRunPath}, authPathPrefix),
		middleware.CSRF(d.SecureCookies, sessionTTL),
	)
}

// deviceAuthenticator returns the device token authenticator or nil (an
// untyped nil keeps DeviceOnly failing closed when no service is wired).
func deviceAuthenticator(d Deps) devicedomain.TokenAuthenticator {
	if d.Devices == nil {
		return nil
	}
	return d.Devices
}

var _ api.ServerInterface = (*server)(nil)

func (s *server) GetHealthz(w http.ResponseWriter, r *http.Request) { s.ops.healthz(w, r) }
func (s *server) GetReadyz(w http.ResponseWriter, r *http.Request)  { s.ops.readyz(w, r) }

type ops struct {
	readiness ReadinessChecker
	version   string
}

func (o *ops) healthz(w http.ResponseWriter, _ *http.Request) {
	v := o.version
	writeJSON(w, http.StatusOK, api.HealthStatus{Status: api.HealthStatusStatusOk, Version: &v})
}

func (o *ops) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	checks := map[string]string{}
	ok := true
	if o.readiness == nil {
		checks["database"], ok = "not configured", false
	} else {
		if err := o.readiness.Ping(ctx); err != nil {
			checks["database"], ok = "unreachable", false
		} else {
			checks["database"] = "ok"
		}
		aligned, err := o.readiness.MigrationsAligned(ctx)
		switch {
		case err != nil:
			checks["migrations"], ok = "unknown", false
		case !aligned:
			checks["migrations"], ok = "pending", false
		default:
			checks["migrations"] = "ok"
		}
	}
	status, code := api.ReadinessStatusStatus("ok"), http.StatusOK
	if !ok {
		status, code = api.ReadinessStatusStatus("unavailable"), http.StatusServiceUnavailable
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, code, api.ReadinessStatus{Status: status, Checks: checks})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func originOf(u string) string {
	scheme, rest, ok := strings.Cut(u, "://")
	if !ok || (scheme != "https" && scheme != "http") {
		return ""
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	return scheme + "://" + rest
}

// bindErrorHandler maps request binding errors of the generated wrapper to
// problem responses. A missing required If-Match header is 428 precondition_required,
// every other binding error is 400.
func bindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var reqHeader *api.RequiredHeaderError
	if errors.As(err, &reqHeader) && strings.EqualFold(reqHeader.ParamName, "If-Match") {
		problem.Write(w, r, problem.PreconditionRequired)
		return
	}
	problem.Write(w, r, problem.BadRequest, problem.Detail(err.Error()))
}
