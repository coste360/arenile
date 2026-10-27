package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
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
func (h *Handlers) SwapBookings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.SwapBookingsParams) {
	var req api.BookingSwapRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.Bookings.Swap(r.Context(), principalOf(r), establishmentID, booking.SwapInput{
		A: swapSideFromAPI(req.A), B: swapSideFromAPI(req.B), From: req.From, DryRun: pcBoolOr(req.DryRun, false),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.BookingSwapResult{
		Applied: res.Applied, From: res.From, To: res.To, A: swapOutcomeToAPI(res.A), B: swapOutcomeToAPI(res.B),
		Conflicts: bkConflicts(res.Conflicts), Undo: undoToAPI(res.Undo),
	})
}

// MoveBookingGroup implements POST /bookings/group-move.
func (h *Handlers) MoveBookingGroup(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.MoveBookingGroupParams) {
	var req api.GroupMoveRequest
	if !decode(w, r, &req) {
		return
	}
	in := booking.GroupMoveInput{From: req.From, DryRun: pcBoolOr(req.DryRun, false)}
	for _, m := range req.Moves {
		in.Moves = append(in.Moves, booking.GroupMoveItem{BookingID: m.BookingId, LineID: m.LineId, SpotID: m.SpotId, ExpectedVersion: m.ExpectedVersion})
	}
	res, err := h.Bookings.GroupMove(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.GroupMoveResult{Applied: res.Applied, Results: []api.BookingChangeResult{}, Conflicts: bkConflicts(res.Conflicts), Undo: undoToAPI(res.Undo)}
	for i := range res.Results {
		out.Results = append(out.Results, bkChangeToAPI(&res.Results[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetLastMove implements GET /moves/last.
func (h *Handlers) GetLastMove(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	info, err := h.Bookings.LastMove(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.LastMove{Move: undoToAPI(info)})
}

// UndoMove implements POST /moves/{moveId}/undo.
func (h *Handlers) UndoMove(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, moveID api.MoveID, _ api.UndoMoveParams) {
	ds, err := h.Bookings.UndoMove(r.Context(), principalOf(r), establishmentID, moveID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.MoveUndoResult{Undone: true, Bookings: []api.BookingDetail{}}
	for _, d := range ds {
		out.Bookings = append(out.Bookings, BookingDetailToAPI(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// DuplicateBooking implements POST /bookings/{bookingId}/duplicate.
func (h *Handlers) DuplicateBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, _ api.DuplicateBookingParams) {
	var req api.BookingDuplicateRequest
	if !decode(w, r, &req) {
		return
	}
	in := booking.DuplicateInput{StartsOn: req.StartsOn, EndsOn: req.EndsOn, DryRun: pcBoolOr(req.DryRun, false)}
	if req.Slots != nil {
		sl := calendar.Slots(*req.Slots)
		in.Slots = &sl
	}
	if req.Formula != nil {
		f := string(*req.Formula)
		in.Formula = &f
	}
	if req.Status != nil {
		st := string(*req.Status)
		in.Status = &st
	}
	res, err := h.Bookings.Duplicate(r.Context(), principalOf(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.BookingDuplicateResult{Applied: res.Applied, Total: int64(res.Total), Conflicts: bkConflicts(res.Conflicts)}
	if res.Price != nil {
		bd := BreakdownToAPI(res.Price.Breakdown)
		out.Price = &bd
	}
	if res.Detail == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	d := BookingDetailToAPI(res.Detail)
	out.Booking = &d
	writeVersioned(w, http.StatusCreated, res.Detail.Booking.Version, out)
}

// GetBookingShareSummary implements GET /bookings/{bookingId}/share-summary.
func (h *Handlers) GetBookingShareSummary(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID) {
	notImplemented(w, r)
}

// GetBookingSummaryPrint implements GET /bookings/{bookingId}/summary-print.
func (h *Handlers) GetBookingSummaryPrint(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.GetBookingSummaryPrintParams) {
	notImplemented(w, r)
}

// ---- conversions (moves, segments) ------------------------------------------------

func swapSideFromAPI(s api.BookingSwapSide) booking.SwapSide {
	return booking.SwapSide{BookingID: s.BookingId, LineID: s.LineId, ExpectedVersion: s.ExpectedVersion}
}

func swapOutcomeToAPI(o booking.SwapOutcome) api.BookingSwapOutcome {
	return api.BookingSwapOutcome{
		Booking: BookingDetailToAPI(o.Detail), FromSpotId: o.FromSpotID, ToSpotId: o.ToSpotID,
		PreviousTotal: int64(o.PreviousTotal), NewTotal: int64(o.NewTotal), PriceDifference: int64(o.NewTotal - o.PreviousTotal),
	}
}

func undoToAPI(u *booking.UndoInfo) *api.MoveUndoInfo {
	if u == nil {
		return nil
	}
	return &api.MoveUndoInfo{MoveId: u.MoveID, Kind: api.MoveUndoInfoKind(u.Kind), ExpiresAt: u.ExpiresAt.UTC(), BookingIds: append([]string{}, u.BookingIDs...)}
}

// bkSegmentsFromAPI converts the segments of a booking line (split stay).
func bkSegmentsFromAPI(ss *[]api.BookingSegmentInput) []booking.SegmentInput {
	if ss == nil {
		return nil
	}
	out := make([]booking.SegmentInput, 0, len(*ss))
	for _, s := range *ss {
		out = append(out, booking.SegmentInput{SpotID: s.SpotId, StartsOn: s.StartsOn, EndsOn: s.EndsOn})
	}
	return out
}

// bookingDetailSegments sets the segments of the lines (from the spot
// allocations) and the last undoable move of a booking detail.
func bookingDetailSegments(out *api.BookingDetail, d *booking.Detail) {
	segs := booking.LineSegments(d)
	for i := range out.Lines {
		ls, ok := segs[out.Lines[i].Id]
		if !ok {
			continue
		}
		as := make([]api.BookingLineSegment, 0, len(ls))
		for _, s := range ls {
			as = append(as, api.BookingLineSegment{SpotId: s.SpotID, SpotLabel: strPtr(d.SpotLabels[s.SpotID]), StartsOn: s.StartsOn, EndsOn: s.EndsOn})
		}
		out.Lines[i].Segments = &as
	}
	out.LastMove = undoToAPI(d.LastMove)
}
