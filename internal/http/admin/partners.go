package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Partners, customer categories, requirement verifications and agreement
// extensions (spec 30.2, ADR 0028). Owner: the agreements builder.
//
//	ListPartners / CreatePartner / GetPartner / UpdatePartner / DeletePartner -> Partners
//	*CustomerCategor*                                                  -> Partners
//	ListCustomerRequirementVerifications / VerifyCustomerRequirement    -> Partners
//	VerifyBookingRequirement -> Bookings.VerifyRequirement (booking state changes stay in booking)
//	ListAgreementVersions / GetAgreementQuotaUsage -> Pricing (pricingconfig) and Inventory

// ListPartners implements GET /partners.
func (h *Handlers) ListPartners(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPartnersParams) {
	notImplemented(w, r)
}

// CreatePartner implements POST /partners.
func (h *Handlers) CreatePartner(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePartnerParams) {
	notImplemented(w, r)
}

// GetPartner implements GET /partners/{partnerId}.
func (h *Handlers) GetPartner(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PartnerID) {
	notImplemented(w, r)
}

// UpdatePartner implements PUT /partners/{partnerId}.
func (h *Handlers) UpdatePartner(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PartnerID, _ api.UpdatePartnerParams) {
	notImplemented(w, r)
}

// DeletePartner implements DELETE /partners/{partnerId}.
func (h *Handlers) DeletePartner(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PartnerID, _ api.DeletePartnerParams) {
	notImplemented(w, r)
}

// ListCustomerCategories implements GET /customer-categories.
func (h *Handlers) ListCustomerCategories(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListCustomerCategoriesParams) {
	notImplemented(w, r)
}

// CreateCustomerCategory implements POST /customer-categories.
func (h *Handlers) CreateCustomerCategory(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateCustomerCategoryParams) {
	notImplemented(w, r)
}

// GetCustomerCategory implements GET /customer-categories/{customerCategoryId}.
func (h *Handlers) GetCustomerCategory(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerCategoryID) {
	notImplemented(w, r)
}

// UpdateCustomerCategory implements PUT /customer-categories/{customerCategoryId}.
func (h *Handlers) UpdateCustomerCategory(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerCategoryID, _ api.UpdateCustomerCategoryParams) {
	notImplemented(w, r)
}

// DeleteCustomerCategory implements DELETE /customer-categories/{customerCategoryId}.
func (h *Handlers) DeleteCustomerCategory(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerCategoryID, _ api.DeleteCustomerCategoryParams) {
	notImplemented(w, r)
}

// ListCustomerRequirementVerifications implements GET /customers/{customerId}/requirement-verifications.
func (h *Handlers) ListCustomerRequirementVerifications(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID) {
	notImplemented(w, r)
}

// VerifyCustomerRequirement implements POST /customers/{customerId}/requirement-verifications.
func (h *Handlers) VerifyCustomerRequirement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID, _ api.VerifyCustomerRequirementParams) {
	notImplemented(w, r)
}

// VerifyBookingRequirement implements POST /bookings/{bookingId}/requirement-verification.
func (h *Handlers) VerifyBookingRequirement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingID, _ api.VerifyBookingRequirementParams) {
	notImplemented(w, r)
}

// ListAgreementVersions implements GET /agreements/{agreementId}/versions.
func (h *Handlers) ListAgreementVersions(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.AgreementID) {
	notImplemented(w, r)
}

// GetAgreementQuotaUsage implements GET /agreements/{agreementId}/quota.
func (h *Handlers) GetAgreementQuotaUsage(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.AgreementID, _ api.GetAgreementQuotaUsageParams) {
	notImplemented(w, r)
}
