package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of the customer domain (internal/domain/customer).
//
// M3 scaffolding: every operation answers 501 not_implemented until the
// owner of the domain package implements it (decode, call the service,
// encode; see maps.go and catalog.go for the patterns).

// ListCustomers implements GET /api/admin/v1/establishments/{establishmentId}/customers.
func (h *Handlers) ListCustomers(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListCustomersParams) {
	notImplemented(w, r)
}

// CreateCustomer implements POST /api/admin/v1/establishments/{establishmentId}/customers.
func (h *Handlers) CreateCustomer(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateCustomerParams) {
	notImplemented(w, r)
}

// GetCustomer implements GET /api/admin/v1/establishments/{establishmentId}/customers/{customerId}.
func (h *Handlers) GetCustomer(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID) {
	notImplemented(w, r)
}

// UpdateCustomer implements PUT /api/admin/v1/establishments/{establishmentId}/customers/{customerId}.
func (h *Handlers) UpdateCustomer(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID, _ api.UpdateCustomerParams) {
	notImplemented(w, r)
}

// ExportCustomer implements GET /api/admin/v1/establishments/{establishmentId}/customers/{customerId}/export.
func (h *Handlers) ExportCustomer(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID) {
	notImplemented(w, r)
}

// AnonymizeCustomer implements POST /api/admin/v1/establishments/{establishmentId}/customers/{customerId}/anonymize.
func (h *Handlers) AnonymizeCustomer(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CustomerID, _ api.AnonymizeCustomerParams) {
	notImplemented(w, r)
}
