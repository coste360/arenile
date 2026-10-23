package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// M5B public waitlist (spec 10.5): owned by the builder of
// internal/domain/waitlist (Handlers.Waitlist). 501 until implemented.
// Responses carry customer data: call noStore(w).

// JoinWaitlist implements POST /waitlist.
func (h *Handlers) JoinWaitlist(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.JoinWaitlistParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetPublicWaitlistEntry implements GET /waitlist/{waitlistToken}.
func (h *Handlers) GetPublicWaitlistEntry(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.WaitlistToken) {
	problem.Write(w, r, problem.NotImplemented)
}

// LeaveWaitlist implements DELETE /waitlist/{waitlistToken}.
func (h *Handlers) LeaveWaitlist(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.WaitlistToken) {
	problem.Write(w, r, problem.NotImplemented)
}

// AcceptWaitlistOffer implements POST /waitlist/{waitlistToken}/accept.
func (h *Handlers) AcceptWaitlistOffer(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.WaitlistToken, _ api.AcceptWaitlistOfferParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// DeclineWaitlistOffer implements POST /waitlist/{waitlistToken}/decline.
func (h *Handlers) DeclineWaitlistOffer(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.WaitlistToken) {
	problem.Write(w, r, problem.NotImplemented)
}
