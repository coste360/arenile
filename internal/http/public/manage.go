package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
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
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	m, err := h.Checkout.Manage(r.Context(), slug, token)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := managedToAPI(m)
	h.attachRenewal(r, slug, token, &out)
	out = h.withCredit(r, out, m) // M5B: credit_available (credits.go)
	w.Header().Set("ETag", admin.ETag(m.Booking.Booking.Version))
	writeJSON(w, http.StatusOK, out)
}

// GetManagedBookingQr implements GET /api/public/v1/{slug}/manage/{token}/qr.
func (h *Handlers) GetManagedBookingQr(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.GetManagedBookingQrParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	kind := checkout.DocQRSVG
	if params.Format != nil && *params.Format == "png" {
		kind = checkout.DocQRPNG
	}
	doc, err := h.Checkout.ManageDocument(r.Context(), slug, token, kind)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDocument(w, doc, false)
}

// GetManagedBookingIcs implements GET /api/public/v1/{slug}/manage/{token}/ics.
func (h *Handlers) GetManagedBookingIcs(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	doc, err := h.Checkout.ManageDocument(r.Context(), slug, token, checkout.DocICS)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDocument(w, doc, true)
}

// PayManagedBookingBalance implements POST /api/public/v1/{slug}/manage/{token}/pay.
func (h *Handlers) PayManagedBookingBalance(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.PayManagedBookingBalanceParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	key := ""
	if params.IdempotencyKey != nil {
		key = *params.IdempotencyKey
	}
	out, err := h.Checkout.ManagePay(r.Context(), slug, token, key)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, payToAPI(out))
}

// ModifyManagedBooking implements POST /api/public/v1/{slug}/manage/{token}/modify.
func (h *Handlers) ModifyManagedBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.ModifyManagedBookingParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	expected, err := admin.ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.ManageModifyRequest
	if !decode(w, r, &req) {
		return
	}
	in := checkout.ModifyInput{
		Selection: selectionFromAPI(api.PublicSelection{
			StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: req.Slots, Formula: req.Formula, Lines: req.Lines, Extras: req.Extras, PromoCode: req.PromoCode,
		}),
		DryRun: req.DryRun == nil || *req.DryRun,
	}
	out, err := h.Checkout.ManageModify(r.Context(), slug, token, expected, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	res := api.ManageModifyResult{
		Applied: out.Result.Applied, Conflicts: conflictsToAPI(out.Result.Conflicts), Violations: violationsToAPI(out.Violations),
		PreviousTotal: int64(out.Result.PreviousTotal), NewTotal: int64(out.Result.NewTotal),
		Difference: int64(out.Result.PriceDifference() + out.Fee),
	}
	if out.Fee > 0 {
		res.Fee = ptr(int64(out.Fee))
	}
	if out.Quote != nil {
		q := quoteToAPI(out.Quote)
		res.Quote = &q
	}
	if out.Managed != nil {
		mb := managedToAPI(out.Managed)
		res.Booking = &mb
		w.Header().Set("ETag", admin.ETag(out.Managed.Booking.Booking.Version))
	}
	writeJSON(w, http.StatusOK, res)
}

// CancelManagedBooking implements POST /api/public/v1/{slug}/manage/{token}/cancel.
func (h *Handlers) CancelManagedBooking(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.CancelManagedBookingParams) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	expected, err := admin.ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.ManageCancelRequest
	if !decode(w, r, &req) {
		return
	}
	in := checkout.CancelInput{DryRun: req.DryRun == nil || *req.DryRun}
	if req.Reason != nil {
		in.Reason = *req.Reason
	}
	if req.Settlement != nil {
		in.Settlement = string(*req.Settlement)
	}
	out, err := h.Checkout.ManageCancel(r.Context(), slug, token, expected, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	res := api.ManageCancelResult{
		Applied: out.Result.Applied, Refund: refundToAPI(out.Result.Refund),
		RefundStatus: ptr(api.ManageCancelResultRefundStatus(out.RefundStatus)),
	}
	if out.Managed != nil {
		mb := managedToAPI(out.Managed)
		res.Booking = &mb
	}
	writeJSON(w, http.StatusOK, res)
}

// ReleaseManagedSeasonDays implements POST /api/public/v1/{slug}/manage/{token}/releases.
func (h *Handlers) ReleaseManagedSeasonDays(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	var req api.ManageReleaseRequest
	if !decode(w, r, &req) {
		return
	}
	in := checkout.ReleaseInput{DryRun: req.DryRun == nil || *req.DryRun}
	for _, d := range req.Days {
		in.Days = append(in.Days, checkout.ReleaseDay{Date: d.Date, Slots: slotsOf(d.Slots)})
	}
	out, err := h.Checkout.ManageRelease(r.Context(), slug, token, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	res := api.ManageReleaseResult{Applied: out.Applied, CreditAmount: int64(out.Credit), Released: make([]api.ManageRelease, len(out.Released))}
	for i, rel := range out.Released {
		res.Released[i] = releaseToAPI(rel)
	}
	if out.Managed != nil {
		mb := managedToAPI(out.Managed)
		res.Booking = &mb
	}
	writeJSON(w, http.StatusOK, res)
}

// RequestManageLink implements POST /api/public/v1/{slug}/manage-links:
// always 202 when the request is well formed and within the limits.
func (h *Handlers) RequestManageLink(w http.ResponseWriter, r *http.Request, slug api.Slug) {
	noStore(w)
	if !h.ready(w, r) {
		return
	}
	var req api.ManageLinkRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.Checkout.RequestLink(r.Context(), slug, string(req.Email), req.Code, reqctx.From(r.Context()).ClientIP); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
