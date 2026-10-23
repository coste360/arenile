package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
)

// M5B customer credits from the magic link (spec 13.4, 12.4, ADR 0017):
// owned by the builder of internal/domain/credit (Handlers.Credits). Only a
// verified link (the magic link of a booking) shows or spends the balance;
// the anonymous checkout never does. Responses carry customer data:
// noStore(w).

// GetManagedCredits implements GET /manage/{token}/credits.
func (h *Handlers) GetManagedCredits(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	noStore(w)
	if !h.ready(w, r) || h.Credits == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	m, err := h.Checkout.Manage(r.Context(), slug, token)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	b := &m.Booking.Booking
	sum, err := h.Credits.Summary(r.Context(), b.EstablishmentID, b.CustomerID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.PublicCreditBalance{Available: int64(sum.Available), Expiring: admin.CreditExpiringToAPI(sum.Expiring)})
}

// ApplyManagedCredit implements POST /manage/{token}/apply-credit: a
// payment of method credit on the booking of the link (partial allowed;
// without amount the minimum of available credit and balance due).
func (h *Handlers) ApplyManagedCredit(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.ApplyManagedCreditParams) {
	noStore(w)
	if !h.ready(w, r) || h.Credits == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	var req api.PublicApplyCreditRequest
	if !decodeOptional(w, r, &req) {
		return
	}
	m, err := h.Checkout.Manage(r.Context(), slug, token)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var amount money.Cents
	if req.Amount != nil {
		amount = money.Cents(*req.Amount)
	}
	key := ""
	if params.IdempotencyKey != nil {
		key = *params.IdempotencyKey
	}
	b := &m.Booking.Booking
	if _, err := h.Credits.ApplyFromLink(r.Context(), b.EstablishmentID, b.ID, amount, key); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	fresh, err := h.Checkout.Manage(r.Context(), slug, token)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("ETag", admin.ETag(fresh.Booking.Booking.Version))
	writeJSON(w, http.StatusOK, h.withCredit(r, managedToAPI(fresh), fresh))
}

// withCredit adds credit_available to a manage view (the balance the
// customer of the link can spend today); left out when the credit service
// is not wired or fails (the manage page works without it).
func (h *Handlers) withCredit(r *http.Request, out api.ManageBooking, m *checkout.Managed) api.ManageBooking {
	if h.Credits == nil || m == nil || m.Booking == nil {
		return out
	}
	b := &m.Booking.Booking
	sum, err := h.Credits.Summary(r.Context(), b.EstablishmentID, b.CustomerID)
	if err != nil {
		return out
	}
	v := int64(sum.Available)
	out.CreditAvailable = &v
	return out
}
