package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Partner account and M8 reports (spec 30.2.4-30.2.5, ADR 0028). Owner:
// the partner account and reports builder.
//
//	GetPartnerStatement   -> Reports.PartnerStatement + Partners (payments); json | xlsx | html
//	ListPartnerPayments / RecordPartnerPayment / VoidPartnerPayment -> Partners
//	GetPartnersReport     -> Reports.Partners; json | csv | xlsx
//	GetAgreementsRegister -> Reports.AgreementsRegister; json | xlsx | html

// GetPartnerStatement implements GET /partners/{partnerId}/statement.
func (h *Handlers) GetPartnerStatement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PartnerID, _ api.GetPartnerStatementParams) {
	notImplemented(w, r)
}

// ListPartnerPayments implements GET /partners/{partnerId}/payments.
func (h *Handlers) ListPartnerPayments(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PartnerID, _ api.ListPartnerPaymentsParams) {
	notImplemented(w, r)
}

// RecordPartnerPayment implements POST /partners/{partnerId}/payments.
func (h *Handlers) RecordPartnerPayment(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PartnerID, _ api.RecordPartnerPaymentParams) {
	notImplemented(w, r)
}

// VoidPartnerPayment implements POST /partners/{partnerId}/payments/{partnerPaymentId}/void.
func (h *Handlers) VoidPartnerPayment(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PartnerID, _ api.PartnerPaymentID, _ api.VoidPartnerPaymentParams) {
	notImplemented(w, r)
}

// GetPartnersReport implements GET /reports/partners.
func (h *Handlers) GetPartnersReport(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetPartnersReportParams) {
	notImplemented(w, r)
}

// GetAgreementsRegister implements GET /reports/agreements-register.
func (h *Handlers) GetAgreementsRegister(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetAgreementsRegisterParams) {
	notImplemented(w, r)
}
