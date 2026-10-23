package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of the booking domain (internal/domain/booking).
//
// M3 scaffolding: every operation answers 501 not_implemented until the
// owner of the domain package implements it (decode, call the service,
// encode; see maps.go and catalog.go for the patterns).

// ListBookings implements GET /api/admin/v1/establishments/{establishmentId}/bookings.
func (h *Handlers) ListBookings(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListBookingsParams) {
	notImplemented(w, r)
}

// CreateBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings.
func (h *Handlers) CreateBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateBookingParams) {
	notImplemented(w, r)
}

// QuoteBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/quote.
func (h *Handlers) QuoteBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.QuoteBookingParams) {
	notImplemented(w, r)
}

// GetBooking implements GET /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}.
func (h *Handlers) GetBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID) {
	notImplemented(w, r)
}

// UpdateBooking implements PATCH /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}.
func (h *Handlers) UpdateBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.UpdateBookingParams) {
	notImplemented(w, r)
}

// ConfirmBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/confirm.
func (h *Handlers) ConfirmBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.ConfirmBookingParams) {
	notImplemented(w, r)
}

// CheckInBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/check-in.
func (h *Handlers) CheckInBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.CheckInBookingParams) {
	notImplemented(w, r)
}

// MarkBookingNoShow implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/no-show.
func (h *Handlers) MarkBookingNoShow(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.MarkBookingNoShowParams) {
	notImplemented(w, r)
}

// ApproveBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/approve.
func (h *Handlers) ApproveBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.ApproveBookingParams) {
	notImplemented(w, r)
}

// RejectBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/reject.
func (h *Handlers) RejectBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.RejectBookingParams) {
	notImplemented(w, r)
}

// CancelBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/cancel.
func (h *Handlers) CancelBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.CancelBookingParams) {
	notImplemented(w, r)
}

// MoveBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/move.
func (h *Handlers) MoveBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.MoveBookingParams) {
	notImplemented(w, r)
}

// AddBookingNote implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/notes.
func (h *Handlers) AddBookingNote(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.AddBookingNoteParams) {
	notImplemented(w, r)
}

// AutoAssign implements POST /api/admin/v1/establishments/{establishmentId}/assignments/auto.
func (h *Handlers) AutoAssign(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.AutoAssignParams) {
	notImplemented(w, r)
}
