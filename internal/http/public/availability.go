package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Public reads of the booking flow (spec 12.3 steps 1-3). Owner: checkout
// builder. Each handler maps the query to the checkout.Service method of
// the same name and the result to the api.Public* schema:
//
//	GetPublicCalendar     -> Checkout.Calendar     -> api.PublicCalendar (Cache-Control public, max-age=30)
//	GetPublicMap          -> Checkout.Map          -> api.PublicMapDocument (ETag = map version, If-None-Match -> 304)
//	GetPublicAvailability -> Checkout.Availability -> api.PublicAvailability (Cache-Control no-cache)
//	GetPublicCatalog      -> Checkout.Catalog      -> api.PublicCatalog
//	QuotePublicBooking    -> Checkout.Quote        -> api.PublicQuote

// GetPublicCalendar implements GET /api/public/v1/{slug}/calendar.
func (h *Handlers) GetPublicCalendar(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.GetPublicCalendarParams) {
	notImplemented(w, r)
}

// GetPublicMap implements GET /api/public/v1/{slug}/maps/{mapId}.
func (h *Handlers) GetPublicMap(w http.ResponseWriter, r *http.Request, slug api.Slug, mapID api.MapID, params api.GetPublicMapParams) {
	notImplemented(w, r)
}

// GetPublicAvailability implements GET /api/public/v1/{slug}/availability.
func (h *Handlers) GetPublicAvailability(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.GetPublicAvailabilityParams) {
	notImplemented(w, r)
}

// GetPublicCatalog implements GET /api/public/v1/{slug}/catalog.
func (h *Handlers) GetPublicCatalog(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.GetPublicCatalogParams) {
	notImplemented(w, r)
}

// QuotePublicBooking implements POST /api/public/v1/{slug}/quote.
func (h *Handlers) QuotePublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug) {
	notImplemented(w, r)
}
