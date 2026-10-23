package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Checkout session endpoints (spec 10.1 steps 3-7). Owner: checkout
// builder. The session token arrives in params.XArenileCheckout (header
// X-Arenile-Checkout) and is passed to the service untouched; never log it.
// Responses with session data call noStore(w).
//
//	CreatePublicBooking  -> Checkout.Create         -> 201 api.CheckoutSession
//	GetPublicBooking     -> Checkout.State          -> api.CheckoutState (never confirms)
//	UpdatePublicBooking  -> Checkout.UpdateCustomer -> api.CheckoutState
//	AbandonPublicBooking -> Checkout.Abandon        -> 204
//	PayPublicBooking     -> Checkout.Pay            -> api.PayResult
//	GetPublicBookingQr   -> Checkout.Document(DocQRSVG|DocQRPNG) -> image/svg+xml | image/png
//	GetPublicBookingIcs  -> Checkout.Document(DocICS) -> text/calendar, Content-Disposition attachment

// CreatePublicBooking implements POST /api/public/v1/{slug}/bookings.
func (h *Handlers) CreatePublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.CreatePublicBookingParams) {
	notImplemented(w, r)
}

// GetPublicBooking implements GET /api/public/v1/{slug}/bookings/{bookingId}.
func (h *Handlers) GetPublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.GetPublicBookingParams) {
	notImplemented(w, r)
}

// UpdatePublicBooking implements PATCH /api/public/v1/{slug}/bookings/{bookingId}.
func (h *Handlers) UpdatePublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.UpdatePublicBookingParams) {
	notImplemented(w, r)
}

// AbandonPublicBooking implements DELETE /api/public/v1/{slug}/bookings/{bookingId}.
func (h *Handlers) AbandonPublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.AbandonPublicBookingParams) {
	notImplemented(w, r)
}

// PayPublicBooking implements POST /api/public/v1/{slug}/bookings/{bookingId}/pay.
func (h *Handlers) PayPublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.PayPublicBookingParams) {
	notImplemented(w, r)
}

// GetPublicBookingQr implements GET /api/public/v1/{slug}/bookings/{bookingId}/qr.
func (h *Handlers) GetPublicBookingQr(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.GetPublicBookingQrParams) {
	notImplemented(w, r)
}

// GetPublicBookingIcs implements GET /api/public/v1/{slug}/bookings/{bookingId}/ics.
func (h *Handlers) GetPublicBookingIcs(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.GetPublicBookingIcsParams) {
	notImplemented(w, r)
}
