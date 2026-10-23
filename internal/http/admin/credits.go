package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// M5B customer credit ledger (spec 13.4): owned by the builder of
// internal/domain/credit (Handlers.Credits). 501 until implemented.

// GetCustomerCredits implements GET /customers/{customerId}/credits.
func (h *Handlers) GetCustomerCredits(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID, _ api.GetCustomerCreditsParams) {
	notImplemented(w, r)
}

// CreateCustomerCredit implements POST /customers/{customerId}/credits.
func (h *Handlers) CreateCustomerCredit(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID, _ api.CreateCustomerCreditParams) {
	notImplemented(w, r)
}
