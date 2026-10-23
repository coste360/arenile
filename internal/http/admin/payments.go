package admin

import (
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the payment domain (internal/domain/payment, spec 13): manual
// payments of a booking, manual refunds, refunds through the online
// provider (via_provider, internal/domain/payment/gateway), counter sales
// and the payments list. Permissions are checked by the services.

// ListBookingPayments implements GET /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/payments.
func (h *Handlers) ListBookingPayments(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, bookingID api.BookingID) {
	items, err := h.Payments.BookingPayments(r.Context(), principalOf(r), establishmentID, bookingID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PaymentList{Items: make([]api.Payment, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, PaymentToAPI(&items[i], nil))
	}
	writeJSON(w, http.StatusOK, out)
}

// RecordBookingPayment implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/payments.
func (h *Handlers) RecordBookingPayment(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, bookingID api.BookingID, params api.RecordBookingPaymentParams) {
	var req api.ManualPaymentInput
	if !decode(w, r, &req) {
		return
	}
	in := payment.Input{
		Amount: money.Cents(req.Amount), Method: string(req.Method), CashSessionID: req.CashSessionId,
		IdempotencyKey: paymentDeref(params.IdempotencyKey),
	}
	if req.Note != nil {
		in.Note = *req.Note
	}
	if req.OverpaymentAsCredit != nil {
		in.OverpaymentAsCredit = *req.OverpaymentAsCredit
	}
	res, err := h.Payments.RecordPayment(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, bookingID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, PaymentToAPI(&res.Payment, nil))
}

// RecordBookingRefund implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/refunds.
func (h *Handlers) RecordBookingRefund(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, bookingID api.BookingID, params api.RecordBookingRefundParams) {
	var req api.RefundInput
	if !decode(w, r, &req) {
		return
	}
	if req.ViaProvider != nil && *req.ViaProvider {
		h.refundViaProvider(w, r, establishmentID, bookingID, params, req)
		return
	}
	in := payment.RefundRequest{
		BookingID: bookingID, Amount: money.Cents(req.Amount), Method: string(req.Method),
		CashSessionID: req.CashSessionId, IdempotencyKey: paymentDeref(params.IdempotencyKey),
		PaymentID: paymentDeref(req.PaymentId), Reason: paymentDeref(req.Reason),
	}
	if req.As != nil {
		in.As = payment.RefundAs(*req.As)
	}
	res, err := h.Payments.RefundBooking(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, PaymentToAPI(&res.Payment, nil))
}

// refundViaProvider is the via_provider branch of RecordBookingRefund (spec
// 13.4): the refund is executed by the provider of payment_id through the
// payment gateway; the response is the refund as known now (status pending
// until the provider confirms, succeeded when it confirmed synchronously).
func (h *Handlers) refundViaProvider(w http.ResponseWriter, r *http.Request, establishmentID, bookingID string, params api.RecordBookingRefundParams, req api.RefundInput) {
	if h.Gateway == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	if req.As != nil && *req.As == api.RefundInputAsCredit {
		problem.Error(w, r, h.Log, configerr.Invalid("as", "Il rimborso tramite provider restituisce denaro: non può essere un credito."))
		return
	}
	in := gateway.RefundRequest{
		BookingID: bookingID, PaymentID: paymentDeref(req.PaymentId), Amount: money.Cents(req.Amount),
		Reason: paymentDeref(req.Reason), IdempotencyKey: paymentDeref(params.IdempotencyKey),
	}
	ri, err := h.Gateway.RefundBooking(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, RefundIntentToAPI(ri, in.PaymentID))
}

// RefundIntentToAPI maps a provider refund (payment_intents row of kind
// refund) to api.Payment: the id is the ledger row once the provider
// confirmed, the intent id before; created and pending map to pending,
// expired and cancelled to cancelled.
func RefundIntentToAPI(ri *store.PaymentIntent, refundOf string) api.Payment {
	id, status := ri.ID, api.PaymentStatusPending
	switch ri.Status {
	case store.IntentSucceeded:
		status = api.PaymentStatusSucceeded
		if ri.PaymentID != nil {
			id = *ri.PaymentID
		}
	case store.IntentFailed:
		status = api.PaymentStatusFailed
	case store.IntentCancelled, store.IntentExpired:
		status = api.PaymentStatusCancelled
	}
	out := api.Payment{
		Id: id, Amount: int64(ri.Amount), Direction: api.PaymentDirection(store.PaymentDirectionRefund),
		Method: api.PaymentMethod(store.PaymentMethodCard), Provider: ri.Provider, Status: status,
		BookingId: ri.BookingID, CreatedAt: ri.CreatedAt.UTC(), Description: strPtr("Rimborso tramite " + ri.Provider),
	}
	if !strings.HasPrefix(ri.ExternalID, "pending:") {
		out.ProviderRef = strPtr(ri.ExternalID)
	}
	if refundOf != "" {
		out.RefundOfPaymentId = strPtr(refundOf)
	}
	return out
}

// ListPayments implements GET /api/admin/v1/establishments/{establishmentId}/payments.
func (h *Handlers) ListPayments(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPaymentsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := store.PaymentFilter{From: params.From, To: params.To, CashSessionID: paymentDeref(params.CashSessionId)}
	if params.Method != nil {
		f.Method = string(*params.Method)
	}
	if params.Direction != nil {
		f.Direction = string(*params.Direction)
	}
	page, err := h.Payments.List(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PaymentList{Items: make([]api.Payment, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, PaymentToAPI(&page.Items[i].Payment, page.Items[i].BookingCode))
	}
	writeJSON(w, http.StatusOK, out)
}

// RecordCounterSale implements POST /api/admin/v1/establishments/{establishmentId}/payments.
func (h *Handlers) RecordCounterSale(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.RecordCounterSaleParams) {
	var req api.CounterSaleInput
	if !decode(w, r, &req) {
		return
	}
	in := payment.CounterSaleInput{
		Amount: money.Cents(req.Amount), Method: string(req.Method), Description: req.Description,
		CustomerID: paymentDeref(req.CustomerId), VATRateBP: req.VatRateBp, CashSessionID: req.CashSessionId,
		IdempotencyKey: paymentDeref(params.IdempotencyKey),
	}
	res, err := h.Payments.RecordCounterSale(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, PaymentToAPI(&res.Payment, nil))
}

// PaymentToAPI maps a payment; bookingCode is optional (lists join it).
func PaymentToAPI(p *store.Payment, bookingCode *string) api.Payment {
	out := api.Payment{
		Id: p.ID, Amount: int64(p.Amount), Direction: api.PaymentDirection(p.Direction), Method: api.PaymentMethod(p.Method),
		Provider: p.Provider, ProviderRef: p.ProviderRef, Status: api.PaymentStatus(p.Status),
		BookingId: p.BookingID, BookingCode: bookingCode, CashSessionId: p.CashSessionID,
		RefundOfPaymentId: p.RefundOfPaymentID, ReceivedByUserId: p.ReceivedByUserID, CreatedAt: p.CreatedAt.UTC(),
	}
	if p.Description != "" {
		out.Description = strPtr(p.Description)
	}
	if p.Note != "" {
		out.Note = strPtr(p.Note)
	}
	return out
}

func paymentDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func paymentClientIP(r *http.Request) string { return reqctx.From(r.Context()).ClientIP }
