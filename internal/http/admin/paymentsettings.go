package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Payment provider settings (M4, spec 13.1, 22). Owner: online settings
// builder. Secrets are write-only: responses carry only configured flags,
// last4 and the publishable key; never log request bodies here.
//
//	GetPaymentSettings    -> Online.GetPayments    -> api.PaymentSettings + ETag
//	UpdatePaymentSettings -> Online.UpdatePayments -> api.PaymentSettings + ETag (If-Match)
//	TestPaymentSettings   -> Online.TestPayments   -> api.PaymentSettingsTestResult

// GetPaymentSettings implements GET /api/admin/v1/establishments/{establishmentId}/payment-settings.
func (h *Handlers) GetPaymentSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdatePaymentSettings implements PUT /api/admin/v1/establishments/{establishmentId}/payment-settings.
func (h *Handlers) UpdatePaymentSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdatePaymentSettingsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// TestPaymentSettings implements POST /api/admin/v1/establishments/{establishmentId}/payment-settings/test.
func (h *Handlers) TestPaymentSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.TestPaymentSettingsParams) {
	problem.Write(w, r, problem.NotImplemented)
}
