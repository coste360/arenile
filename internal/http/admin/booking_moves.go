package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Moves and booking actions (spec 30.7, 30.9, ADR 0028). Owner: the moves
// builder.
//
//	SwapBookings           -> Bookings.Swap       -> api.BookingSwapResult
//	MoveBookingGroup       -> Bookings.GroupMove  -> api.GroupMoveResult
//	GetLastMove            -> Bookings.LastMove   -> api.LastMove
//	UndoMove               -> Bookings.UndoMove   -> api.MoveUndoResult
//	DuplicateBooking       -> Bookings.Duplicate  -> api.BookingDuplicateResult (200 dry run, 201)
//	GetBookingShareSummary -> Bookings + Notify (manage link) -> api.BookingShareSummary
//	GetBookingSummaryPrint -> internal/printout (HTML, ADR 0019)

// SwapBookings implements POST /bookings/swap.
func (h *Handlers) SwapBookings(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SwapBookingsParams) {
	notImplemented(w, r)
}

// MoveBookingGroup implements POST /bookings/group-move.
func (h *Handlers) MoveBookingGroup(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.MoveBookingGroupParams) {
	notImplemented(w, r)
}

// GetLastMove implements GET /moves/last.
func (h *Handlers) GetLastMove(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	notImplemented(w, r)
}

// UndoMove implements POST /moves/{moveId}/undo.
func (h *Handlers) UndoMove(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.MoveID, _ api.UndoMoveParams) {
	notImplemented(w, r)
}

// DuplicateBooking implements POST /bookings/{bookingId}/duplicate.
func (h *Handlers) DuplicateBooking(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.DuplicateBookingParams) {
	notImplemented(w, r)
}

// GetBookingShareSummary implements GET /bookings/{bookingId}/share-summary.
func (h *Handlers) GetBookingShareSummary(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID) {
	notImplemented(w, r)
}

// GetBookingSummaryPrint implements GET /bookings/{bookingId}/summary-print.
func (h *Handlers) GetBookingSummaryPrint(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.GetBookingSummaryPrintParams) {
	notImplemented(w, r)
}
