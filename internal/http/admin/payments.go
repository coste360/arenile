package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the payment domain (internal/domain/payment, spec 13): manual
// payments of a booking, manual refunds, counter sales and the payments
// list. Permissions are checked by the service.

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
