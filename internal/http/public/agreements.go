package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Agreement link and code on the public page (spec 30.2.2, ADR 0028).
// Owner: the agreements builder (Checkout resolves the code; the quote and
// checkout apply PublicSelection.agreement_code).

// GetPublicAgreement implements GET /api/public/v1/{slug}/agreements/{agreementCode}.
func (h *Handlers) GetPublicAgreement(w http.ResponseWriter, r *http.Request, _ api.Slug, _ api.AgreementCode) {
	problem.Write(w, r, problem.NotImplemented)
}
