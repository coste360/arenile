// Package ext defines the public extension points of Arenile.
//
// The interfaces and types in this package are the ONLY stable Go APIs of the
// project: everything under internal/ may change without notice. Integrators,
// hosting providers and driver authors build their own main package that
// imports the Arenile module and registers implementations:
//
//	func main() {
//		cfg, err := config.FromEnv()
//		...
//		a, err := app.New(cfg, app.WithExtensions(ext.Extensions{
//			PaymentProviders: []ext.PaymentProvider{mydriver.New()},
//		}))
//		...
//		a.Run(ctx)
//	}
//
// Implementations must be safe for concurrent use, honour context
// cancellation and never log personal data (emails, phones, tokens).
package ext

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Extensions groups every extension registered by a custom main. Zero values
// mean "use the core default" (or "feature disabled" for optional hooks).
type Extensions struct {
	// PaymentProviders are added to the core providers ("manual"). A provider
	// with the same Code as a core one replaces it.
	PaymentProviders []PaymentProvider
	// FiscalDrivers are added to the core drivers ("none").
	FiscalDrivers []FiscalDriver
	// EventSinks receive every domain event after it leaves the outbox
	// (at-least-once delivery).
	EventSinks []EventSink
	// ConcessionData provides data about the state-owned beach concession
	// (perimeter, satellite imagery, expiry).
	ConcessionData ConcessionDataProvider
	// TenantResolver resolves the establishment from host or headers
	// (e.g. custom domains). Nil means "slug in the URL path".
	TenantResolver TenantResolver
	// AuthProviders add external sign-in methods (e.g. OIDC).
	AuthProviders []AuthProvider
	// Notifiers add delivery channels (WhatsApp, SMS) next to core email.
	Notifiers []Notifier
	// BotProtection adds an invisible challenge to public creations.
	BotProtection BotProtection
	// EventBus replaces the in-memory real-time bus (e.g. PostgreSQL
	// LISTEN/NOTIFY for multi-instance deployments).
	EventBus EventBus
	// HTTPRoutes mounts additional routes (e.g. billing) on the root router.
	// Routes must not collide with /api/admin/v1, /api/public/v1, /admin and /b.
	HTTPRoutes func(r chi.Router)
	// AdminUIPlugins add menu entries and pages to the admin panel.
	AdminUIPlugins []UIPluginManifest
}

// Middleware is a standard net/http middleware.
type Middleware = func(http.Handler) http.Handler
