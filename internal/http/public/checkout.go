package public

import (
	"net/http"
	"strconv"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
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
// Repeated requests with the same Idempotency-Key replay the first answer
// (middleware), so a double tap on a slow network creates one hold.
func (h *Handlers) CreatePublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.CreatePublicBookingParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	var req api.PublicBookingCreate
	if !decode(w, r, &req) {
		return
	}
	sel := selectionFromAPI(api.PublicSelection{
		StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: req.Slots, Formula: req.Formula, Lines: req.Lines, Extras: req.Extras, PromoCode: req.PromoCode,
	})
	in := checkout.CreateInput{Selection: sel, EmbedOrigin: embedOrigin(r)}
	if req.Locale != nil {
		in.Locale = *req.Locale
	}
	s, err := h.Checkout.Create(r.Context(), slug, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.CheckoutSession{
		BookingId: s.BookingID, CheckoutToken: s.Token, HoldExpiresAt: s.HoldExpiresAt.UTC(), Status: api.BookingStatus(s.Status), Quote: quoteToAPI(s.Quote),
	})
}

// embedOrigin is the origin of the page embedding the widget, as sent by
// the browser (Origin header of the iframe document is the booking page
// itself, so only the explicit X-Arenile-Embed-Origin set by embed.js is
// used; it is informational: conversions analytics).
func embedOrigin(r *http.Request) string {
	o := r.Header.Get("X-Arenile-Embed-Origin")
	if len(o) > 255 {
		return ""
	}
	return o
}

// GetPublicBooking implements GET /api/public/v1/{slug}/bookings/{bookingId}.
func (h *Handlers) GetPublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.GetPublicBookingParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	st, err := h.Checkout.State(r.Context(), slug, bookingID, params.XArenileCheckout)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, stateToAPI(st))
}

// UpdatePublicBooking implements PATCH /api/public/v1/{slug}/bookings/{bookingId}.
func (h *Handlers) UpdatePublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.UpdatePublicBookingParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	var req api.CheckoutCustomerInput
	if !decode(w, r, &req) {
		return
	}
	in := checkout.CustomerInput{
		FirstName: req.FirstName, LastName: req.LastName, Email: string(req.Email), AcceptTermsVersion: req.AcceptTermsVersion,
		MarketingConsent: req.MarketingConsent, PromoCode: req.PromoCode,
	}
	if req.Phone != nil {
		in.Phone = *req.Phone
	}
	if req.Locale != nil {
		in.Locale = *req.Locale
	}
	if req.Notes != nil {
		in.Notes = *req.Notes
	}
	if req.Extras != nil {
		in.Extras, in.ExtrasSet = extrasFromAPI(req.Extras), true
	}
	st, err := h.Checkout.UpdateCustomer(r.Context(), slug, bookingID, params.XArenileCheckout, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, stateToAPI(st))
}

// AbandonPublicBooking implements DELETE /api/public/v1/{slug}/bookings/{bookingId}.
func (h *Handlers) AbandonPublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.AbandonPublicBookingParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	if err := h.Checkout.Abandon(r.Context(), slug, bookingID, params.XArenileCheckout); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PayPublicBooking implements POST /api/public/v1/{slug}/bookings/{bookingId}/pay.
func (h *Handlers) PayPublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.PayPublicBookingParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	var req api.CheckoutPayRequest
	if !decodeOptional(w, r, &req) {
		return
	}
	in := checkout.PayInput{}
	if req.Settlement != nil {
		in.Settlement = string(*req.Settlement)
	}
	if req.ReturnTo != nil {
		in.ReturnTo = *req.ReturnTo
	}
	if params.IdempotencyKey != nil {
		in.IdempotencyKey = *params.IdempotencyKey
	}
	out, err := h.Checkout.Pay(r.Context(), slug, bookingID, params.XArenileCheckout, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, payToAPI(out))
}

// GetPublicBookingQr implements GET /api/public/v1/{slug}/bookings/{bookingId}/qr.
func (h *Handlers) GetPublicBookingQr(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.GetPublicBookingQrParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	kind := checkout.DocQRSVG
	if params.Format != nil && *params.Format == "png" {
		kind = checkout.DocQRPNG
	}
	doc, err := h.Checkout.Document(r.Context(), slug, bookingID, params.XArenileCheckout, kind)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDocument(w, doc, false)
}

// GetPublicBookingIcs implements GET /api/public/v1/{slug}/bookings/{bookingId}/ics.
func (h *Handlers) GetPublicBookingIcs(w http.ResponseWriter, r *http.Request, slug api.Slug, bookingID api.BookingID, params api.GetPublicBookingIcsParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	doc, err := h.Checkout.Document(r.Context(), slug, bookingID, params.XArenileCheckout, checkout.DocICS)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDocument(w, doc, true)
}

// writeDocument sends a binary document (attachment for downloads).
func writeDocument(w http.ResponseWriter, doc *checkout.Document, attachment bool) {
	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(doc.Body)))
	disp := "inline"
	if attachment {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", disp+`; filename="`+doc.Filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(doc.Body)
}
