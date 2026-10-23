package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of the payment domain (internal/domain/payment).
//
// M3 scaffolding: every operation answers 501 not_implemented until the
// owner of the domain package implements it (decode, call the service,
// encode; see maps.go and catalog.go for the patterns).

// ListBookingPayments implements GET /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/payments.
func (h *Handlers) ListBookingPayments(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID) {
	notImplemented(w, r)
}

// RecordBookingPayment implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/payments.
func (h *Handlers) RecordBookingPayment(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.RecordBookingPaymentParams) {
	notImplemented(w, r)
}

// RecordBookingRefund implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/refunds.
func (h *Handlers) RecordBookingRefund(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.RecordBookingRefundParams) {
	notImplemented(w, r)
}

// ListPayments implements GET /api/admin/v1/establishments/{establishmentId}/payments.
func (h *Handlers) ListPayments(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPaymentsParams) {
	notImplemented(w, r)
}

// RecordCounterSale implements POST /api/admin/v1/establishments/{establishmentId}/payments.
func (h *Handlers) RecordCounterSale(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RecordCounterSaleParams) {
	notImplemented(w, r)
}
