package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Payment provider settings (M4, spec 13.1, 22). The use cases live in the
// online service (credentials encrypted with the key ring, "test
// connection" through the driver's gateway.CredentialsVerifier). Secrets
// are write-only: responses carry only configured flags, last4 and the
// publishable key; request bodies are never logged here.
//
//	GetPaymentSettings    -> Online.GetPayments    -> api.PaymentSettings + ETag
//	UpdatePaymentSettings -> Online.UpdatePayments -> api.PaymentSettings + ETag (If-Match)
//	TestPaymentSettings   -> Online.TestPayments   -> api.PaymentSettingsTestResult

// GetPaymentSettings implements GET /api/admin/v1/establishments/{establishmentId}/payment-settings.
func (h *Handlers) GetPaymentSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if h.Online == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	ps, err := h.Online.GetPayments(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeVersioned(w, http.StatusOK, ps.Version, PaymentSettingsToAPI(ps))
}

// UpdatePaymentSettings implements PUT /api/admin/v1/establishments/{establishmentId}/payment-settings.
func (h *Handlers) UpdatePaymentSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdatePaymentSettingsParams) {
	if h.Online == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.PaymentSettingsInput
	if !decode(w, r, &req) {
		return
	}
	in := online.PaymentSettingsInput{Provider: req.Provider, Mode: string(req.Mode)}
	if req.Stripe != nil {
		in.Stripe = &online.StripeInput{
			PublishableKey: req.Stripe.PublishableKey, SecretKey: req.Stripe.SecretKey,
			WebhookSecret: req.Stripe.WebhookSecret, EmbeddedMode: req.Stripe.EmbeddedMode,
		}
	}
	ps, err := h.Online.UpdatePayments(r.Context(), principalOf(r), establishmentID, expected, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeVersioned(w, http.StatusOK, ps.Version, PaymentSettingsToAPI(ps))
}

// TestPaymentSettings implements POST /api/admin/v1/establishments/{establishmentId}/payment-settings/test.
func (h *Handlers) TestPaymentSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.TestPaymentSettingsParams) {
	if h.Online == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	res, err := h.Online.TestPayments(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PaymentSettingsTestResult{Ok: res.OK, Provider: res.Provider, Mode: api.ProviderMode(res.Mode)}
	if res.Message != "" {
		out.Message = strPtr(res.Message)
	}
	if res.AccountLabel != "" {
		out.AccountLabel = strPtr(res.AccountLabel)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// PaymentSettingsToAPI maps the payment settings (no secrets by
// construction: online.PaymentSettings carries none).
func PaymentSettingsToAPI(ps *online.PaymentSettings) api.PaymentSettings {
	out := api.PaymentSettings{
		Provider: ps.Provider, AvailableProviders: ps.AvailableProviders, Mode: api.ProviderMode(ps.Mode),
		WebhookUrl: ps.WebhookURL, LivePaymentsBlocked: ps.LivePaymentsBlocked, Version: ps.Version,
		Stripe: api.StripeSettings{
			Configured: ps.Stripe.Configured, EmbeddedMode: ps.Stripe.EmbeddedMode,
			WebhookSecretConfigured: ps.Stripe.WebhookSecretConfigured, VerifiedAt: ps.Stripe.VerifiedAt,
		},
	}
	if out.AvailableProviders == nil {
		out.AvailableProviders = []string{}
	}
	if ps.Stripe.PublishableKey != "" {
		out.Stripe.PublishableKey = strPtr(ps.Stripe.PublishableKey)
	}
	if ps.Stripe.SecretKeyLast4 != "" {
		out.Stripe.SecretKeyLast4 = strPtr(ps.Stripe.SecretKeyLast4)
	}
	return out
}
