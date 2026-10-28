package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Agreement link and code on the public page (spec 30.2.2, ADR 0028): the
// page reads the agreement of /b/{slug}?c={code}; the quote and the booking
// apply PublicSelection.agreement_code (checkout.Selection.AgreementCode).

// GetPublicAgreement implements GET /api/public/v1/{slug}/agreements/{agreementCode}:
// only active agreements, valid today and sold online; 404 otherwise.
func (h *Handlers) GetPublicAgreement(w http.ResponseWriter, r *http.Request, slug api.Slug, code api.AgreementCode) {
	if h.Checkout == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	a, err := h.Checkout.GetAgreement(r.Context(), slug, code)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PublicAgreement{
		Code: code, Name: a.Name, PartnerName: optStr(a.PartnerName), Requirement: optStr(a.M8.Requirement),
		Summary: optStr(a.Summary), ValidFrom: a.ValidFrom, ValidTo: a.ValidTo,
	}
	if a.Code != nil {
		out.Code = *a.Code
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func agreementStatusToAPI(s *checkout.AgreementStatus) *api.AgreementStatus {
	if s == nil {
		return nil
	}
	return &api.AgreementStatus{
		Code: s.Code, Name: optStr(s.Name), Requirement: optStr(s.Requirement), Status: api.AgreementStatusStatus(s.Status),
	}
}
