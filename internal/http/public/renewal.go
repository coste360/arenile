package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// M5B renewal proposals from the magic link (spec 10.4, 12.4): owned by the
// builder of internal/domain/renewal (Handlers.Renewals). 501 until
// implemented. Responses carry customer data: call noStore(w).

// GetManagedRenewal implements GET /manage/{token}/renewal.
func (h *Handlers) GetManagedRenewal(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.ManageToken) {
	problem.Write(w, r, problem.NotImplemented)
}

// AcceptManagedRenewal implements POST /manage/{token}/renewal/accept.
func (h *Handlers) AcceptManagedRenewal(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.ManageToken, _ api.AcceptManagedRenewalParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// DeclineManagedRenewal implements POST /manage/{token}/renewal/decline.
func (h *Handlers) DeclineManagedRenewal(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.ManageToken) {
	problem.Write(w, r, problem.NotImplemented)
}
