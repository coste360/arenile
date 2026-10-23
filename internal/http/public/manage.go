package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Manage area via magic link (spec 12.4). Owner: checkout builder. The
// token is a path parameter: never log it (the access log records only the
// route pattern). Every response calls noStore(w) (also sets
// Referrer-Policy: no-referrer).
//
//	GetManagedBooking        -> Checkout.Manage         -> api.ManageBooking (ETag = booking version)
//	GetManagedBookingQr      -> Checkout.ManageDocument -> image
//	GetManagedBookingIcs     -> Checkout.ManageDocument -> text/calendar
//	PayManagedBookingBalance -> Checkout.ManagePay      -> api.PayResult
//	ModifyManagedBooking     -> Checkout.ManageModify   -> api.ManageModifyResult (If-Match)
//	CancelManagedBooking     -> Checkout.ManageCancel   -> api.ManageCancelResult (If-Match)
//	ReleaseManagedSeasonDays -> Checkout.ManageRelease  -> api.ManageReleaseResult
//	RequestManageLink        -> Checkout.RequestLink    -> 202 always (no enumeration)

// GetManagedBooking implements GET /api/public/v1/{slug}/manage/{token}.
func (h *Handlers) GetManagedBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	notImplemented(w, r)
}

// GetManagedBookingQr implements GET /api/public/v1/{slug}/manage/{token}/qr.
func (h *Handlers) GetManagedBookingQr(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.GetManagedBookingQrParams) {
	notImplemented(w, r)
}

// GetManagedBookingIcs implements GET /api/public/v1/{slug}/manage/{token}/ics.
func (h *Handlers) GetManagedBookingIcs(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	notImplemented(w, r)
}

// PayManagedBookingBalance implements POST /api/public/v1/{slug}/manage/{token}/pay.
func (h *Handlers) PayManagedBookingBalance(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.PayManagedBookingBalanceParams) {
	notImplemented(w, r)
}

// ModifyManagedBooking implements POST /api/public/v1/{slug}/manage/{token}/modify.
func (h *Handlers) ModifyManagedBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.ModifyManagedBookingParams) {
	notImplemented(w, r)
}

// CancelManagedBooking implements POST /api/public/v1/{slug}/manage/{token}/cancel.
func (h *Handlers) CancelManagedBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.CancelManagedBookingParams) {
	notImplemented(w, r)
}

// ReleaseManagedSeasonDays implements POST /api/public/v1/{slug}/manage/{token}/releases.
func (h *Handlers) ReleaseManagedSeasonDays(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	notImplemented(w, r)
}

// RequestManageLink implements POST /api/public/v1/{slug}/manage-links.
func (h *Handlers) RequestManageLink(w http.ResponseWriter, r *http.Request, slug api.Slug) {
	notImplemented(w, r)
}
