package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the pricing builder (internal/domain/pricingconfig (Service)).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// ListAgreements is not implemented yet.
func (h *Handlers) ListAgreements(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListAgreementsParams) {
	notImplemented(w, r)
}

// CreateAgreement is not implemented yet.
func (h *Handlers) CreateAgreement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateAgreementParams) {
	notImplemented(w, r)
}

// DeleteAgreement is not implemented yet.
func (h *Handlers) DeleteAgreement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.AgreementID, _ api.DeleteAgreementParams) {
	notImplemented(w, r)
}

// GetAgreement is not implemented yet.
func (h *Handlers) GetAgreement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.AgreementID) {
	notImplemented(w, r)
}

// UpdateAgreement is not implemented yet.
func (h *Handlers) UpdateAgreement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.AgreementID, _ api.UpdateAgreementParams) {
	notImplemented(w, r)
}

// ListFixedPrices is not implemented yet.
func (h *Handlers) ListFixedPrices(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListFixedPricesParams) {
	notImplemented(w, r)
}

// CreateFixedPrice is not implemented yet.
func (h *Handlers) CreateFixedPrice(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateFixedPriceParams) {
	notImplemented(w, r)
}

// DeleteFixedPrice is not implemented yet.
func (h *Handlers) DeleteFixedPrice(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.FixedPriceID, _ api.DeleteFixedPriceParams) {
	notImplemented(w, r)
}

// GetFixedPrice is not implemented yet.
func (h *Handlers) GetFixedPrice(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.FixedPriceID) {
	notImplemented(w, r)
}

// UpdateFixedPrice is not implemented yet.
func (h *Handlers) UpdateFixedPrice(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.FixedPriceID, _ api.UpdateFixedPriceParams) {
	notImplemented(w, r)
}

// ListPriceRules is not implemented yet.
func (h *Handlers) ListPriceRules(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPriceRulesParams) {
	notImplemented(w, r)
}

// CreatePriceRule is not implemented yet.
func (h *Handlers) CreatePriceRule(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePriceRuleParams) {
	notImplemented(w, r)
}

// DeletePriceRule is not implemented yet.
func (h *Handlers) DeletePriceRule(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PriceRuleID, _ api.DeletePriceRuleParams) {
	notImplemented(w, r)
}

// GetPriceRule is not implemented yet.
func (h *Handlers) GetPriceRule(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PriceRuleID) {
	notImplemented(w, r)
}

// UpdatePriceRule is not implemented yet.
func (h *Handlers) UpdatePriceRule(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PriceRuleID, _ api.UpdatePriceRuleParams) {
	notImplemented(w, r)
}

// GetPriceCoverage is not implemented yet.
func (h *Handlers) GetPriceCoverage(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetPriceCoverageParams) {
	notImplemented(w, r)
}

// GetPriceMatrix is not implemented yet.
func (h *Handlers) GetPriceMatrix(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetPriceMatrixParams) {
	notImplemented(w, r)
}

// SimulatePrice is not implemented yet.
func (h *Handlers) SimulatePrice(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SimulatePriceParams) {
	notImplemented(w, r)
}

// ListPromoCodes is not implemented yet.
func (h *Handlers) ListPromoCodes(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPromoCodesParams) {
	notImplemented(w, r)
}

// CreatePromoCode is not implemented yet.
func (h *Handlers) CreatePromoCode(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePromoCodeParams) {
	notImplemented(w, r)
}

// DeletePromoCode is not implemented yet.
func (h *Handlers) DeletePromoCode(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PromoCodeID, _ api.DeletePromoCodeParams) {
	notImplemented(w, r)
}

// GetPromoCode is not implemented yet.
func (h *Handlers) GetPromoCode(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PromoCodeID) {
	notImplemented(w, r)
}

// UpdatePromoCode is not implemented yet.
func (h *Handlers) UpdatePromoCode(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PromoCodeID, _ api.UpdatePromoCodeParams) {
	notImplemented(w, r)
}

// CopySeason is not implemented yet.
func (h *Handlers) CopySeason(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonID, _ api.CopySeasonParams) {
	notImplemented(w, r)
}
