package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Outgoing webhook endpoints managed from the panel (M5A, spec 18.2).
// Owner: integrations backend builder. Responses that carry a plaintext
// secret set Cache-Control: no-store; secrets are never logged.
//
//	ListWebhookEndpoints  -> Webhooks.List           -> api.WebhookEndpointList
//	CreateWebhookEndpoint -> Webhooks.Create         -> 201 api.WebhookEndpointCreated
//	GetWebhookEndpoint    -> Webhooks.Get            -> api.WebhookEndpoint
//	UpdateWebhookEndpoint -> Webhooks.Update         -> api.WebhookEndpoint
//	DeleteWebhookEndpoint -> Webhooks.Delete         -> 204
//	RotateWebhookSecret   -> Webhooks.RotateSecret   -> api.WebhookSecretRotated
//	SendWebhookTest       -> Webhooks.SendTest       -> api.WebhookDelivery
//	ListWebhookDeliveries -> Webhooks.ListDeliveries -> api.WebhookDeliveryList
//	GetWebhookDelivery    -> Webhooks.GetDelivery    -> api.WebhookDeliveryDetail
//	RedeliverWebhook      -> Webhooks.Redeliver      -> 202 api.WebhookDelivery

// ListWebhookEndpoints implements GET /api/admin/v1/establishments/{establishmentId}/webhook-endpoints.
func (h *Handlers) ListWebhookEndpoints(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateWebhookEndpoint implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints.
func (h *Handlers) CreateWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.CreateWebhookEndpointParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetWebhookEndpoint implements GET /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}.
func (h *Handlers) GetWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateWebhookEndpoint implements PATCH /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}.
func (h *Handlers) UpdateWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, params api.UpdateWebhookEndpointParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// DeleteWebhookEndpoint implements DELETE /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}.
func (h *Handlers) DeleteWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, params api.DeleteWebhookEndpointParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// RotateWebhookSecret implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/rotate-secret.
func (h *Handlers) RotateWebhookSecret(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, params api.RotateWebhookSecretParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// SendWebhookTest implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/test.
func (h *Handlers) SendWebhookTest(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, params api.SendWebhookTestParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ListWebhookDeliveries implements GET /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/deliveries.
func (h *Handlers) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, params api.ListWebhookDeliveriesParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetWebhookDelivery implements GET /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/deliveries/{deliveryId}.
func (h *Handlers) GetWebhookDelivery(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, deliveryID api.WebhookDeliveryID) {
	problem.Write(w, r, problem.NotImplemented)
}

// RedeliverWebhook implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/deliveries/{deliveryId}/redeliver.
func (h *Handlers) RedeliverWebhook(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, deliveryID api.WebhookDeliveryID, params api.RedeliverWebhookParams) {
	problem.Write(w, r, problem.NotImplemented)
}
