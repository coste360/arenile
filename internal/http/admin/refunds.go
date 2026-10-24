package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// B. Guaranteed refunds of cancellations (ADR 0019): list and manual
// retry of the refund requests sent by the refund job.

// ListRefundRequests implements GET /api/admin/v1/establishments/{establishmentId}/refund-requests.
func (h *Handlers) ListRefundRequests(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListRefundRequestsParams) {
	if h.Refunds == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	f := store.RefundRequestFilter{}
	if params.Status != nil {
		f.Status = string(*params.Status)
	}
	if params.BookingId != nil {
		f.BookingID = *params.BookingId
	}
	if params.Cursor != nil {
		f.Cursor = *params.Cursor
	}
	if params.Limit != nil {
		f.Limit = *params.Limit
	}
	page, err := h.Refunds.List(r.Context(), principalOf(r), establishmentID, f)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RefundRequestList{Items: make([]api.RefundRequest, len(page.Items))}
	for i := range page.Items {
		out.Items[i] = RefundRequestToAPI(&page.Items[i].RefundRequest, page.Items[i].BookingCode)
	}
	out.NextCursor = nextCursor(page.NextCursor)
	writeJSON(w, http.StatusOK, out)
}

// RetryRefundRequest implements POST /api/admin/v1/establishments/{establishmentId}/refund-requests/{refundRequestId}/retry.
func (h *Handlers) RetryRefundRequest(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, refundRequestID api.RefundRequestID, _ api.RetryRefundRequestParams) {
	if h.Refunds == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	rr, err := h.Refunds.Retry(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, refundRequestID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, RefundRequestToAPI(rr, nil))
}

// RefundRequestToAPI maps a refund request (last_error is a short code,
// never a provider message).
func RefundRequestToAPI(rr *store.RefundRequest, bookingCode *string) api.RefundRequest {
	pi := rr.PaymentIntentID
	out := api.RefundRequest{
		Id: rr.ID, BookingId: rr.BookingID, BookingCode: bookingCode, Amount: int64(rr.Amount), Attempts: rr.Attempts,
		Status: api.RefundRequestStatus(rr.Status), Reason: api.RefundRequestReason(rr.Reason), Provider: rr.Provider,
		PaymentIntentId: &pi, RefundIntentId: rr.RefundIntentID, PaymentId: rr.PaymentID, LastError: rr.LastError,
		NextAttemptAt: utcPtr(rr.NextAttemptAt), CompletedAt: utcPtr(rr.CompletedAt),
		CreatedAt: rr.CreatedAt.UTC(), UpdatedAt: rr.UpdatedAt.UTC(),
	}
	if rr.Test {
		t := true
		out.Test = &t
	}
	return out
}

// refundRequestsToAPI maps the refund requests of a booking detail.
func refundRequestsToAPI(rs []store.RefundRequest, bookingCode string) *[]api.RefundRequest {
	out := make([]api.RefundRequest, len(rs))
	code := bookingCode
	for i := range rs {
		out[i] = RefundRequestToAPI(&rs[i], &code)
	}
	return &out
}
