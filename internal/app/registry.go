package app

import (
	"slices"
	"sort"

	"github.com/coste360/arenile/internal/app/builtin"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/pkg/ext"
)

// Registry holds the extension implementations available to domain
// services: core built-ins plus those registered through ext.Extensions.
type Registry struct {
	payments map[string]ext.PaymentProvider
	fiscal   map[string]ext.FiscalDriver

	Notifiers      []ext.Notifier
	AuthProviders  []ext.AuthProvider
	ConcessionData ext.ConcessionDataProvider
	BotProtection  ext.BotProtection
	TenantResolver ext.TenantResolver
	UIPlugins      []ext.UIPluginManifest
}

// NewRegistry registers the built-ins, then the extensions (an extension
// with the same code replaces the built-in).
func NewRegistry(c clock.Clock, x ext.Extensions) *Registry {
	r := &Registry{
		payments:       map[string]ext.PaymentProvider{},
		fiscal:         map[string]ext.FiscalDriver{},
		Notifiers:      slices.Clone(x.Notifiers),
		AuthProviders:  slices.Clone(x.AuthProviders),
		ConcessionData: x.ConcessionData,
		BotProtection:  x.BotProtection,
		TenantResolver: x.TenantResolver,
		UIPlugins:      slices.Clone(x.AdminUIPlugins),
	}
	r.payments[builtin.ManualCode] = builtin.Manual{}
	// Stripe is a core driver (spec 13.2); an extension with the same code
	// replaces it. Credentials are per establishment (online.Service).
	r.payments[stripe.Code] = stripe.New(stripe.Options{Clock: c})
	r.fiscal[builtin.NoneCode] = builtin.NoneFiscal{Clock: c}
	for _, p := range x.PaymentProviders {
		if p != nil && p.Code() != "" {
			r.payments[p.Code()] = p
		}
	}
	for _, f := range x.FiscalDrivers {
		if f != nil && f.Code() != "" {
			r.fiscal[f.Code()] = f
		}
	}
	return r
}

// PaymentProvider returns the provider with code.
func (r *Registry) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	p, ok := r.payments[code]
	return p, ok
}

// PaymentProviderCodes lists the registered payment providers.
func (r *Registry) PaymentProviderCodes() []string { return sortedKeys(r.payments) }

// FiscalDriver returns the driver with code.
func (r *Registry) FiscalDriver(code string) (ext.FiscalDriver, bool) {
	f, ok := r.fiscal[code]
	return f, ok
}

// FiscalDriverCodes lists the registered fiscal drivers.
func (r *Registry) FiscalDriverCodes() []string { return sortedKeys(r.fiscal) }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
