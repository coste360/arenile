package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the booking domain (internal/domain/booking): decode, call the
// service (which checks permissions and runs the unit of work), encode.
// Idempotency-Key on POST /bookings is honoured by the idempotency
// middleware for every POST; If-Match is parsed here and checked by the
// service against the booking version (412 version_conflict).

// ListBookings implements GET /api/admin/v1/establishments/{establishmentId}/bookings.
func (h *Handlers) ListBookings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListBookingsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := store.BookingFilter{From: params.From, To: params.To, SeasonPass: params.SeasonPass, BalanceDue: params.BalanceDue, Test: params.Test}
	if params.Status != nil {
		for _, s := range *params.Status {
			f.Statuses = append(f.Statuses, string(s))
		}
	}
	f.CustomerID = pcStrOr(params.CustomerId)
	f.SpotID = pcStrOr(params.SpotId)
	f.Q = pcStrOr(params.Q)
	if params.Channel != nil {
		f.Channel = string(*params.Channel)
	}
	page, err := h.Bookings.List(r.Context(), principalOf(r), establishmentID, booking.ListInput{Filter: f, Cursor: cursor, Limit: limit})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.BookingList{Items: make([]api.BookingSummary, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, BookingSummaryToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings.
func (h *Handlers) CreateBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateBookingParams) {
	var req api.BookingCreate
	if !decode(w, r, &req) {
		return
	}
	d, err := h.Bookings.Create(r.Context(), principalOf(r), establishmentID, bkCreateFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, d.Booking.Version, BookingDetailToAPI(d))
}

// QuoteBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/quote.
func (h *Handlers) QuoteBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.QuoteBookingParams) {
	var req api.BookingCreate
	if !decode(w, r, &req) {
		return
	}
	q, err := h.Bookings.Quote(r.Context(), principalOf(r), establishmentID, bkCreateFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	ev := api.PolicyEvaluation{Rules: PolicyRulesToAPI(q.Result.Policy.Rules), Violations: bkViolations(q.Result.Violations)}
	if q.Result.Policy.PolicyID != "" {
		id := q.Result.Policy.PolicyID
		ev.PolicyId = &id
	}
	writeJSON(w, http.StatusOK, api.BookingQuote{
		Available: len(q.Conflicts) == 0, Breakdown: BreakdownToAPI(q.Result.Breakdown), Conflicts: bkConflicts(q.Conflicts),
		DepositDue: int64(q.Result.DepositDue), Policy: ev,
	})
}

// GetBooking implements GET /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}.
func (h *Handlers) GetBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID) {
	d, err := h.Bookings.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Booking.Version, BookingDetailToAPI(d))
}

// UpdateBooking implements PATCH /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}.
func (h *Handlers) UpdateBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.UpdateBookingParams) {
	im, err := RequireIfMatch(&params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var raw map[string]json.RawMessage
	var req api.BookingPatch
	if !decode(w, r, &raw) || !bkRedecode(w, r, raw, &req) {
		return
	}
	in := booking.PatchInput{
		CustomerID: req.CustomerId, StartsOn: req.StartsOn, EndsOn: req.EndsOn,
		NotesCustomer: req.NotesCustomer, NotesInternal: req.NotesInternal, Force: pcBoolOr(req.Force, false),
		DryRun: pcBoolOr(params.DryRun, false),
	}
	if req.Slots != nil {
		s := calendar.Slots(*req.Slots)
		in.Slots = &s
	}
	if req.Formula != nil {
		f := string(*req.Formula)
		in.Formula = &f
	}
	if req.Lines != nil {
		in.Lines = bkLinesFromAPI(*req.Lines)
		if in.Lines == nil {
			in.Lines = []booking.LineInput{}
		}
	}
	if _, ok := raw["extras"]; ok {
		in.ExtrasSet = true
		in.Extras = bkExtrasFromAPI(req.Extras)
	}
	res, err := h.Bookings.Update(r.Context(), principalOf(r), establishmentID, id, bkExpected(im), in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, res.Detail.Booking.Version, bkChangeToAPI(res))
}

// bkRedecode decodes the already parsed object into v (the raw map tells
// absent fields from explicit ones).
func bkRedecode(w http.ResponseWriter, r *http.Request, raw map[string]json.RawMessage, v any) bool {
	b, err := json.Marshal(raw)
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if err != nil {
		problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
		return false
	}
	return true
}

// bkExpected maps a parsed If-Match ("*" = any version) to the expected version.
func bkExpected(im IfMatch) *int64 {
	if im.Any {
		return nil
	}
	v := im.Version
	return &v
}

// bkRequireIfMatch is the precondition of the actions launched from the
// operational map (check-in, no-show, cancel, move; ADR 0019 C): a missing
// If-Match is 428 precondition_required, "*" means any version. A dry run
// writes nothing, so it accepts a missing header.
func bkRequireIfMatch(h *string, dryRun bool) (*int64, error) {
	if dryRun {
		return ParseIfMatch(h)
	}
	im, err := RequireIfMatch(h)
	if err != nil {
		return nil, err
	}
	return bkExpected(im), nil
}

// bkAction decodes the optional BookingActionRequest and the If-Match header
// (mandatory when required is true).
func (h *Handlers) bkAction(w http.ResponseWriter, r *http.Request, ifMatch *string, required bool) (booking.ActionInput, bool) {
	expected, err := bkRequireIfMatch(ifMatch, !required)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return booking.ActionInput{}, false
	}
	in := booking.ActionInput{ExpectedVersion: expected}
	if r.ContentLength != 0 && r.Body != nil {
		var req api.BookingActionRequest
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
			return booking.ActionInput{}, false
		}
		in.Note = strings.TrimSpace(pcStrOr(req.Note))
	}
	return in, true
}

type bkActionFn func(in booking.ActionInput) (*booking.Detail, error)

func (h *Handlers) bkRunAction(w http.ResponseWriter, r *http.Request, ifMatch *string, fn bkActionFn) {
	h.bkRunActionIf(w, r, ifMatch, false, fn)
}

// bkRunActionIf is bkRunAction with a mandatory If-Match when required is true.
func (h *Handlers) bkRunActionIf(w http.ResponseWriter, r *http.Request, ifMatch *string, required bool, fn bkActionFn) {
	in, ok := h.bkAction(w, r, ifMatch, required)
	if !ok {
		return
	}
	d, err := fn(in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Booking.Version, BookingDetailToAPI(d))
}

// ConfirmBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/confirm.
func (h *Handlers) ConfirmBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.ConfirmBookingParams) {
	h.bkRunAction(w, r, params.IfMatch, func(in booking.ActionInput) (*booking.Detail, error) {
		return h.Bookings.Confirm(r.Context(), principalOf(r), establishmentID, id, in)
	})
}

// CheckInBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/check-in.
func (h *Handlers) CheckInBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.CheckInBookingParams) {
	h.bkRunActionIf(w, r, params.IfMatch, true, func(in booking.ActionInput) (*booking.Detail, error) {
		return h.Bookings.CheckIn(r.Context(), principalOf(r), establishmentID, id, in)
	})
}

// MarkBookingNoShow implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/no-show.
func (h *Handlers) MarkBookingNoShow(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.MarkBookingNoShowParams) {
	h.bkRunActionIf(w, r, params.IfMatch, true, func(in booking.ActionInput) (*booking.Detail, error) {
		return h.Bookings.MarkNoShow(r.Context(), principalOf(r), establishmentID, id, in)
	})
}

// ApproveBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/approve.
func (h *Handlers) ApproveBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.ApproveBookingParams) {
	h.bkRunAction(w, r, params.IfMatch, func(in booking.ActionInput) (*booking.Detail, error) {
		return h.Bookings.Approve(r.Context(), principalOf(r), establishmentID, id, in)
	})
}

// RejectBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/reject.
func (h *Handlers) RejectBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.RejectBookingParams) {
	h.bkRunAction(w, r, params.IfMatch, func(in booking.ActionInput) (*booking.Detail, error) {
		return h.Bookings.Reject(r.Context(), principalOf(r), establishmentID, id, in)
	})
}

// CancelBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/cancel.
func (h *Handlers) CancelBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.CancelBookingParams) {
	var req api.BookingCancelRequest
	if !decode(w, r, &req) {
		return
	}
	expected, err := bkRequireIfMatch(params.IfMatch, pcBoolOr(req.DryRun, false))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	in := booking.CancelInput{
		ExpectedVersion: expected, Reason: pcStrOr(req.Reason), Weather: pcBoolOr(req.Weather, false), DryRun: pcBoolOr(req.DryRun, false),
	}
	if req.Settlement != nil {
		in.Settlement = booking.Settlement(*req.Settlement)
	}
	if req.RefundAmount != nil {
		a := money.Cents(*req.RefundAmount)
		in.RefundAmount = &a
	}
	if req.RefundMethod != nil {
		in.RefundMethod = string(*req.RefundMethod)
	}
	res, err := h.Bookings.Cancel(r.Context(), principalOf(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.BookingCancelResult{Applied: res.Applied, Booking: BookingDetailToAPI(res.Detail), Refund: bkRefundToAPI(res.Refund)}
	writeVersioned(w, http.StatusOK, res.Detail.Booking.Version, out)
}

// MoveBooking implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/move.
func (h *Handlers) MoveBooking(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.MoveBookingParams) {
	var req api.BookingMoveRequest
	if !decode(w, r, &req) {
		return
	}
	expected, err := bkRequireIfMatch(params.IfMatch, pcBoolOr(req.DryRun, false))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	res, err := h.Bookings.Move(r.Context(), principalOf(r), establishmentID, id, booking.MoveInput{
		ExpectedVersion: expected, LineID: req.LineId, SpotID: req.SpotId, From: req.From, DryRun: pcBoolOr(req.DryRun, false),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, res.Detail.Booking.Version, bkChangeToAPI(res))
}

// AddBookingNote implements POST /api/admin/v1/establishments/{establishmentId}/bookings/{bookingId}/notes.
func (h *Handlers) AddBookingNote(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, _ api.AddBookingNoteParams) {
	var req api.BookingNoteInput
	if !decode(w, r, &req) {
		return
	}
	in := booking.NoteInput{Text: req.Text}
	if req.Visibility != nil {
		in.Visibility = string(*req.Visibility)
	}
	d, err := h.Bookings.AddNote(r.Context(), principalOf(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Booking.Version, BookingDetailToAPI(d))
}

// AutoAssign implements POST /api/admin/v1/establishments/{establishmentId}/assignments/auto.
func (h *Handlers) AutoAssign(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.AutoAssignParams) {
	var req api.AutoAssignRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.Bookings.AutoAssign(r.Context(), principalOf(r), establishmentID, booking.AutoAssignInput{
		From: req.From, To: req.To, ZoneID: pcStrOr(req.ZoneId), DryRun: pcBoolOr(req.DryRun, false),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.AutoAssignResult{Applied: res.Applied, Assigned: []api.AutoAssignment{}, Unassigned: []api.AutoUnassigned{}}
	for _, a := range res.Assigned {
		out.Assigned = append(out.Assigned, api.AutoAssignment{BookingId: a.BookingID, LineId: a.LineID, SpotId: a.SpotID, SpotLabel: strPtr(a.SpotLabel)})
	}
	for _, u := range res.Unassigned {
		out.Unassigned = append(out.Unassigned, api.AutoUnassigned{BookingId: u.BookingID, LineId: u.LineID, Reason: u.Reason})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- conversions ----------------------------------------------------------------

func bkExtrasFromAPI(xs *[]api.SimulateExtra) []booking.ExtraInput {
	if xs == nil {
		return nil
	}
	out := make([]booking.ExtraInput, 0, len(*xs))
	for _, x := range *xs {
		out = append(out, booking.ExtraInput{ExtraID: x.ExtraId, Quantity: x.Quantity})
	}
	return out
}

func bkLinesFromAPI(ls []api.BookingLineInput) []booking.LineInput {
	out := make([]booking.LineInput, 0, len(ls))
	for _, l := range ls {
		out = append(out, booking.LineInput{
			PackID: l.PackId, ZoneID: pcStrOr(l.ZoneId), SpotID: pcStrOr(l.SpotId), Quantity: pcIntOr(l.Quantity, 1),
			Extras: bkExtrasFromAPI(l.Extras),
		})
	}
	return out
}

func bkCustomerInput(c *api.CustomerInput) *customer.Input {
	if c == nil {
		return nil
	}
	in := &customer.Input{
		FirstName: pcStrOr(c.FirstName), LastName: c.LastName, Phone: pcStrOr(c.Phone), Notes: pcStrOr(c.Notes),
		Tags: pcSliceOf(c.Tags), AgreementID: pcStrOr(c.AgreementId),
	}
	if c.Email != nil {
		in.Email = string(*c.Email)
	}
	if c.Locale != nil {
		in.Locale = string(*c.Locale)
	}
	if c.MarketingConsent != nil {
		in.MarketingConsent = &store.MarketingConsent{GivenAt: c.MarketingConsent.GivenAt, Source: c.MarketingConsent.Source, TextVersion: c.MarketingConsent.TextVersion}
	}
	return in
}

func bkCreateFromAPI(req api.BookingCreate) booking.CreateInput {
	in := booking.CreateInput{
		CustomerID: pcStrOr(req.CustomerId), Customer: bkCustomerInput(req.Customer),
		StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: calendar.Slots(req.Slots), Formula: string(req.Formula),
		Lines: bkLinesFromAPI(req.Lines), Extras: bkExtrasFromAPI(req.Extras), PromoCode: pcStrOr(req.PromoCode),
		AgreementID: pcStrOr(req.AgreementId), NotesCustomer: pcStrOr(req.NotesCustomer), NotesInternal: pcStrOr(req.NotesInternal),
		HoldID: pcStrOr(req.HoldId), Force: pcBoolOr(req.Force, false),
	}
	if req.Channel != nil {
		in.Channel = string(*req.Channel)
	}
	if req.Status != nil {
		in.Status = booking.Status(*req.Status)
	}
	if req.HoldTtlSeconds != nil {
		in.HoldTTL = time.Duration(*req.HoldTtlSeconds) * time.Second
		if *req.HoldTtlSeconds <= 0 {
			in.HoldTTL = -1 // rejected by validation
		}
	}
	return in
}

func bkViolations(vs []policy.Violation) []api.PolicyViolation {
	out := make([]api.PolicyViolation, 0, len(vs))
	for _, v := range vs {
		pv := api.PolicyViolation{Code: string(v.Code)}
		if len(v.Params) > 0 {
			params := v.Params
			pv.Params = &params
		}
		out = append(out, pv)
	}
	return out
}

func bkConflicts(cs []availability.Conflict) []api.AllocationConflict {
	out := make([]api.AllocationConflict, 0, len(cs))
	for i, c := range cs {
		ac := api.AllocationConflict{Date: c.Date, Slot: api.Slot(c.Slot), Reason: api.AllocationConflictReason(c.Reason)}
		if c.SpotID != "" {
			ac.SpotId = &cs[i].SpotID
		}
		if c.PoolID != "" {
			ac.PoolId = &cs[i].PoolID
		}
		if c.ZoneID != "" {
			ac.ZoneId = &cs[i].ZoneID
		}
		out = append(out, ac)
	}
	return out
}

func bkChangeToAPI(res *booking.ChangeResult) api.BookingChangeResult {
	return api.BookingChangeResult{
		Applied: res.Applied, Booking: BookingDetailToAPI(res.Detail), Conflicts: bkConflicts(res.Conflicts),
		NewTotal: int64(res.NewTotal), PreviousTotal: int64(res.PreviousTotal), PriceDifference: int64(res.PriceDifference()),
	}
}

func bkRefundToAPI(rf booking.RefundPreview) api.RefundPreview {
	out := api.RefundPreview{Amount: int64(rf.Amount), As: api.RefundPreviewAs(rf.As), PaidAmount: int64(rf.PaidAmount), RefundPercent: rf.RefundPercent}
	if rf.Tier != nil {
		t := api.PolicyCancellationTier{RefundPercent: rf.Tier.RefundPercent, UntilDaysBefore: rf.Tier.UntilDaysBefore}
		if rf.Tier.As != "" {
			as := api.PolicyCancellationTierAs(rf.Tier.As)
			t.As = &as
		}
		out.Tier = &t
	}
	return out
}

func bkDisplayName(first, last string) string {
	return strings.TrimSpace(strings.TrimSpace(last) + " " + strings.TrimSpace(first))
}

// BookingSummaryToAPI maps a list row.
func BookingSummaryToAPI(b *store.BookingRow) api.BookingSummary {
	return api.BookingSummary{
		Id: b.ID, Code: b.Code, Channel: api.Channel(b.Channel), Status: api.BookingStatus(b.Status),
		CustomerId: b.CustomerID, CustomerDisplayName: bkDisplayName(b.CustomerFirstName, b.CustomerLastName),
		StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: api.Slots(b.Slots), Formula: api.Formula(b.Formula),
		TotalAmount: int64(b.TotalAmount), PaidAmount: int64(b.PaidAmount), DepositDue: int64(b.DepositDue),
		BalanceDue: int64(b.BalanceDue()), IsSeasonPass: b.IsSeasonPass, HoldExpiresAt: utcPtr(b.HoldExpiresAt),
		SpotLabels: nonNil(b.SpotLabels), CreatedAt: b.CreatedAt.UTC(), Version: b.Version,
		Test: testFlag(b.Test),
	}
}

// testFlag maps the test-mode flag of a row: present only when true.
func testFlag(t bool) *bool {
	if !t {
		return nil
	}
	return &t
}

func bkCustomerToAPI(c *store.Customer) api.Customer {
	out := api.Customer{
		Id: c.ID, FirstName: c.FirstName, LastName: c.LastName, DisplayName: bkDisplayName(c.FirstName, c.LastName),
		Email: c.Email, Phone: c.Phone, Locale: c.Locale, Notes: c.Notes, Tags: nonNil(c.Tags), AgreementId: c.AgreementID,
		AnonymizedAt: utcPtr(c.AnonymizedAt), CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC(), Version: max(c.Version, 1),
	}
	if c.MarketingConsent != nil {
		out.MarketingConsent = &api.MarketingConsent{GivenAt: c.MarketingConsent.GivenAt, Source: c.MarketingConsent.Source, TextVersion: c.MarketingConsent.TextVersion}
	}
	return out
}

func bkPaymentToAPI(p *store.Payment, code string) api.Payment {
	out := api.Payment{
		Id: p.ID, Amount: int64(p.Amount), Direction: api.PaymentDirection(p.Direction), Method: api.PaymentMethod(p.Method),
		Provider: p.Provider, Status: api.PaymentStatus(p.Status), CreatedAt: p.CreatedAt.UTC(),
		BookingId: p.BookingID, CashSessionId: p.CashSessionID, ProviderRef: p.ProviderRef,
		ReceivedByUserId: p.ReceivedByUserID, RefundOfPaymentId: p.RefundOfPaymentID,
		Test: testFlag(p.Test),
	}
	if code != "" {
		out.BookingCode = &code
	}
	if p.Note != "" {
		n := p.Note
		out.Note = &n
	}
	if p.Description != "" {
		desc := p.Description
		out.Description = &desc
	}
	return out
}

// BookingDetailToAPI maps a booking detail.
func BookingDetailToAPI(d *booking.Detail) api.BookingDetail {
	b := &d.Booking
	st := booking.Status(b.Status)
	out := api.BookingDetail{
		Id: b.ID, Code: b.Code, Channel: api.Channel(b.Channel), Status: api.BookingStatus(b.Status),
		StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: api.Slots(b.Slots), Formula: api.Formula(b.Formula),
		SeasonId: b.SeasonID, IsSeasonPass: b.IsSeasonPass, AgreementId: b.AgreementID,
		TotalAmount: int64(b.TotalAmount), PaidAmount: int64(b.PaidAmount), DepositDue: int64(b.DepositDue), BalanceDue: int64(b.BalanceDue()),
		HoldExpiresAt: utcPtr(b.HoldExpiresAt), NotesCustomer: b.NotesCustomer, NotesInternal: b.NotesInternal,
		CreatedByUserId: b.CreatedByUserID, CancelledAt: utcPtr(b.CancelledAt), CancelReason: b.CancelReason,
		CreatedAt: b.CreatedAt.UTC(), UpdatedAt: b.UpdatedAt.UTC(), Version: b.Version,
		Customer: bkCustomerToAPI(&d.Customer), NextStatuses: []api.BookingStatus{},
		Lines: []api.BookingLine{}, Extras: []api.BookingExtra{}, Allocations: []api.BookingAllocation{},
		Payments: []api.Payment{}, History: []api.BookingHistoryEntry{},
		Test: testFlag(b.Test),
	}
	for _, n := range st.NextStatuses() {
		out.NextStatuses = append(out.NextStatuses, api.BookingStatus(n))
	}
	var bd pricing.PriceBreakdown
	if err := json.Unmarshal(b.PriceSnapshot, &bd); err == nil {
		out.PriceSnapshot = BreakdownToAPI(bd)
	} else {
		out.PriceSnapshot = BreakdownToAPI(pricing.PriceBreakdown{})
	}
	if len(b.PriceCurrent) > 0 {
		var cur pricing.PriceBreakdown
		if err := json.Unmarshal(b.PriceCurrent, &cur); err == nil {
			pc := BreakdownToAPI(cur)
			out.PriceCurrent = &pc
		}
	}
	if snap, err := policy.ParseSnapshot(b.PolicySnapshot); err == nil {
		out.PolicySnapshot = api.PolicySnapshot{Rules: PolicyRulesToAPI(snap.Rules)}
		if snap.PolicyID != "" {
			id := snap.PolicyID
			out.PolicySnapshot.PolicyId = &id
		}
	} else {
		out.PolicySnapshot = api.PolicySnapshot{Rules: PolicyRulesToAPI(policy.Rules{})}
	}
	extrasByLine := map[string][]api.BookingExtra{}
	for _, x := range d.Extras {
		ax := api.BookingExtra{Id: x.ID, ExtraId: x.ExtraID, Quantity: x.Quantity, Total: int64(x.Total), BookingLineId: x.BookingLineID, ExtraCode: strPtr(d.ExtraCodes[x.ExtraID])}
		if x.BookingLineID == nil {
			out.Extras = append(out.Extras, ax)
		} else {
			extrasByLine[*x.BookingLineID] = append(extrasByLine[*x.BookingLineID], ax)
		}
	}
	for _, l := range d.Lines {
		al := api.BookingLine{
			Id: l.ID, PackId: l.PackID, ZoneId: l.ZoneID, SpotId: l.SpotID, AssignmentStatus: api.AssignmentStatus(l.AssignmentStatus),
			Quantity: l.Quantity, LineTotal: int64(l.LineTotal), Extras: extrasByLine[l.ID], PackCode: strPtr(d.PackCodes[l.PackID]),
		}
		if l.SpotID != nil {
			al.SpotLabel = strPtr(d.SpotLabels[*l.SpotID])
		}
		if al.Extras == nil {
			al.Extras = []api.BookingExtra{}
		}
		out.Lines = append(out.Lines, al)
	}
	for _, a := range d.SpotAllocations {
		spot := a.SpotID
		out.Allocations = append(out.Allocations, api.BookingAllocation{
			BookingLineId: a.BookingLineID, Date: a.Date, Slot: api.Slot(a.Slot), Kind: api.AllocationKind(a.Kind),
			SpotId: &spot, ExpiresAt: utcPtr(a.ExpiresAt),
		})
	}
	for _, a := range d.PoolAllocations {
		pool, qty := a.PoolID, a.Quantity
		out.Allocations = append(out.Allocations, api.BookingAllocation{
			BookingLineId: a.BookingLineID, Date: a.Date, Slot: api.Slot(a.Slot), Kind: api.AllocationKind(a.Kind),
			PoolId: &pool, Quantity: &qty, ExpiresAt: utcPtr(a.ExpiresAt),
		})
	}
	for i := range d.Payments {
		out.Payments = append(out.Payments, bkPaymentToAPI(&d.Payments[i], b.Code))
	}
	for _, e := range d.History {
		he := api.BookingHistoryEntry{Action: e.Action, ActorId: e.ActorID, ActorType: api.BookingHistoryEntryActorType(e.ActorType), At: e.CreatedAt.UTC()}
		if len(e.Diff) > 0 {
			var m map[string]any
			if json.Unmarshal(e.Diff, &m) == nil && len(m) > 0 {
				he.Details = &m
			}
		}
		out.History = append(out.History, he)
	}
	if len(d.RefundRequests) > 0 {
		out.Refunds = refundRequestsToAPI(d.RefundRequests, b.Code)
	}
	return out
}
