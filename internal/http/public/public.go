// Package public implements the thin HTTP handlers of the public booking
// API (/api/public/v1). It never returns customer data.
package public

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/pkg/ext"
)

// Handlers implements the public operations of api.ServerInterface.
type Handlers struct {
	Establishments *establishment.Service
	// TenantResolver optionally maps a custom domain to a slug.
	TenantResolver ext.TenantResolver
	Log            *slog.Logger
}

// GetPublicEstablishment implements GET /api/public/v1/{slug}.
func (h *Handlers) GetPublicEstablishment(w http.ResponseWriter, r *http.Request, slug api.Slug) {
	if h.TenantResolver != nil {
		if s, ok, err := h.TenantResolver.ResolveSlug(r); err != nil {
			problem.Error(w, r, h.Log, err)
			return
		} else if ok && s != slug {
			problem.Write(w, r, problem.NotFound)
			return
		}
	}
	e, err := h.Establishments.GetPublic(r.Context(), slug)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PublicEstablishment{
		Slug: e.Slug, Name: e.Name, Timezone: e.Timezone, Currency: e.Currency, DefaultLocale: e.DefaultLocale,
		EnabledLocales: e.EnabledLocales, BookingEnabled: e.BookingEnabled, Theme: admin.ThemeToAPI(e.Theme),
	}
	if out.EnabledLocales == nil {
		out.EnabledLocales = []string{}
	}
	if e.ContactEmail != "" {
		out.ContactEmail = &e.ContactEmail
	}
	if e.ContactPhone != "" {
		out.ContactPhone = &e.ContactPhone
	}
	addr := admin.AddressToAPI(e.Address)
	if addr != (api.Address{}) {
		out.Address = &addr
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
