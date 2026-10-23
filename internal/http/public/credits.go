package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// M5B customer credits from the magic link (spec 13.4, 12.4): owned by the
// builder of internal/domain/credit (Handlers.Credits). 501 until
// implemented. Responses carry customer data: call noStore(w).

// GetManagedCredits implements GET /manage/{token}/credits.
func (h *Handlers) GetManagedCredits(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.ManageToken) {
	problem.Write(w, r, problem.NotImplemented)
}

// ApplyManagedCredit implements POST /manage/{token}/apply-credit.
func (h *Handlers) ApplyManagedCredit(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.ManageToken, _ api.ApplyManagedCreditParams) {
	problem.Write(w, r, problem.NotImplemented)
}
