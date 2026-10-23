package public

import (
	"errors"
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/waitlist"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
)

// M5B public waitlist (spec 10.5): owned by the builder of
// internal/domain/waitlist (Handlers.Waitlist). The join always answers 202
// without data (no enumeration); the personal link is a path parameter:
// never logged. Responses carry customer data: noStore(w).

func (h *Handlers) waitlistReady(w http.ResponseWriter, r *http.Request) bool {
	if h.Waitlist == nil {
		problem.Write(w, r, problem.NotImplemented)
		return false
	}
	return true
}

// JoinWaitlist implements POST /waitlist.
func (h *Handlers) JoinWaitlist(w http.ResponseWriter, r *http.Request, slug api.Slug, _ api.JoinWaitlistParams) {
	noStore(w)
	if !h.waitlistReady(w, r) {
		return
	}
	var req api.PublicWaitlistJoin
	if !decode(w, r, &req) {
		return
	}
	in := waitlist.PublicJoinInput{
		StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: calendar.Slots(req.Slots), PackID: req.PackId,
		FirstName: req.Customer.FirstName, LastName: req.Customer.LastName, Email: string(req.Customer.Email),
		AcceptTermsVersion: req.Customer.AcceptTermsVersion, IP: reqctx.From(r.Context()).ClientIP,
	}
	if req.ZoneId != nil {
		in.ZoneID = *req.ZoneId
	}
	if req.Quantity != nil {
		in.Quantity = *req.Quantity
	}
	if req.Customer.Phone != nil {
		in.Phone = *req.Customer.Phone
	}
	if req.Customer.Locale != nil {
		in.Locale = *req.Customer.Locale
	}
	if err := h.Waitlist.Join(r.Context(), slug, in); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// GetPublicWaitlistEntry implements GET /waitlist/{waitlistToken}.
func (h *Handlers) GetPublicWaitlistEntry(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.WaitlistToken) {
	noStore(w)
	if !h.waitlistReady(w, r) {
		return
	}
	e, err := h.Waitlist.GetByToken(r.Context(), slug, token)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, publicWaitlistToAPI(e))
}

// LeaveWaitlist implements DELETE /waitlist/{waitlistToken}.
func (h *Handlers) LeaveWaitlist(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.WaitlistToken) {
	noStore(w)
	if !h.waitlistReady(w, r) {
		return
	}
	if err := h.Waitlist.LeaveByToken(r.Context(), slug, token); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AcceptWaitlistOffer implements POST /waitlist/{waitlistToken}/accept:
// the offer hold becomes a held booking with a checkout session (same
// response as createPublicBooking).
func (h *Handlers) AcceptWaitlistOffer(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.WaitlistToken, params api.AcceptWaitlistOfferParams) {
	noStore(w)
	if !h.waitlistReady(w, r) {
		return
	}
	key := ""
	if params.IdempotencyKey != nil {
		key = *params.IdempotencyKey
	}
	s, err := h.Waitlist.AcceptByToken(r.Context(), slug, token, key)
	if err != nil {
		if errors.Is(err, configerr.ErrNotImplemented) {
			problem.Write(w, r, problem.NotImplemented)
			return
		}
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.CheckoutSession{
		BookingId: s.BookingID, CheckoutToken: s.Token, HoldExpiresAt: s.HoldExpiresAt.UTC(), Status: api.BookingStatus(s.Status), Quote: quoteToAPI(s.Quote),
	})
}

// DeclineWaitlistOffer implements POST /waitlist/{waitlistToken}/decline.
func (h *Handlers) DeclineWaitlistOffer(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.WaitlistToken) {
	noStore(w)
	if !h.waitlistReady(w, r) {
		return
	}
	var req api.PublicWaitlistDeclineRequest
	if !decodeOptional(w, r, &req) {
		return
	}
	e, err := h.Waitlist.DeclineByToken(r.Context(), slug, token, req.Leave != nil && *req.Leave)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, publicWaitlistToAPI(e))
}

func publicWaitlistToAPI(e *waitlist.PublicEntry) api.PublicWaitlistEntry {
	q := e.Entry.Quantity
	out := api.PublicWaitlistEntry{
		Status: api.WaitlistEntryStatus(e.Entry.Status), StartsOn: e.Entry.StartsOn, EndsOn: e.Entry.EndsOn,
		Slots: api.Slots(e.Entry.Slots), PackName: e.PackName, Quantity: &q,
	}
	if e.ZoneName != "" {
		z := e.ZoneName
		out.ZoneName = &z
	}
	if e.Offer != nil {
		o := api.PublicWaitlistOffer{Status: api.WaitlistOfferStatus(e.Offer.Status), ExpiresAt: e.Offer.ExpiresAt.UTC()}
		if e.Offer.SpotLabel != "" {
			l := e.Offer.SpotLabel
			o.SpotLabel = &l
		}
		out.Offer = &o
	}
	return out
}
