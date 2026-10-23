// Package public implements the thin HTTP handlers of the public booking
// API (/api/public/v1): decode, call the domain service, encode. It never
// returns customer data except to the holder of the checkout session token
// (X-Arenile-Checkout) or of the magic link.
//
// Files and owners (M4):
//   - profile.go: GET /{slug} (online settings builder: profile, theme,
//     flow configuration, ETag);
//   - availability.go: calendar, map, availability, catalog, quote
//     (checkout builder);
//   - checkout.go: bookings (create, state, customer, pay, abandon, QR,
//     .ics) (checkout builder);
//   - manage.go: manage area via magic link and new-link requests
//     (checkout builder; QR/.ics rendering and links come from notify).
package public

import (
	"log/slog"
	"net/http"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/pkg/ext"
)

// Handlers implements the public operations of api.ServerInterface.
type Handlers struct {
	Establishments *establishment.Service
	// Online builds the flow configuration of the profile (M4); nil or not
	// yet implemented = profile without `online`.
	Online *online.Service
	// Checkout serves the public flow and the manage area (M4).
	Checkout *checkout.Service
	// TenantResolver optionally maps a custom domain to a slug.
	TenantResolver ext.TenantResolver
	Log            *slog.Logger
}

// noStore marks responses that carry session or customer data.
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// ready answers 501 when the checkout service is not wired (servers built
// without the M4 services, e.g. focused tests).
func (h *Handlers) ready(w http.ResponseWriter, r *http.Request) bool {
	if h.Checkout == nil {
		problem.Write(w, r, problem.NotImplemented)
		return false
	}
	return true
}
