package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/domain/webhook"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Outgoing webhook endpoints managed from the panel (M5A, spec 18.2). Owner
// only; a request authenticated with an API key always gets 403. Responses
// that carry a plaintext secret set Cache-Control: no-store; secrets are
// never logged.
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
	if !h.webhooksReady(w, r) {
		return
	}
	eps, err := h.Webhooks.List(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.WebhookEndpointList{Items: make([]api.WebhookEndpoint, len(eps))}
	for i := range eps {
		out.Items[i] = WebhookEndpointToAPI(&eps[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateWebhookEndpoint implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints.
func (h *Handlers) CreateWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateWebhookEndpointParams) {
	if !h.webhooksReady(w, r) {
		return
	}
	var req api.WebhookEndpointCreate
	if !decode(w, r, &req) {
		return
	}
	in := webhook.CreateInput{URL: req.Url, Description: estDeref(req.Description), Events: req.Events, Active: true, IP: estClientIP(r)}
	if req.Active != nil {
		in.Active = *req.Active
	}
	ep, secret, err := h.Webhooks.Create(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, api.WebhookEndpointCreated{Endpoint: WebhookEndpointToAPI(ep), Secret: secret})
}

// GetWebhookEndpoint implements GET /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}.
func (h *Handlers) GetWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID) {
	if !h.webhooksReady(w, r) {
		return
	}
	ep, err := h.Webhooks.Get(r.Context(), principalOf(r), establishmentID, endpointID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, WebhookEndpointToAPI(ep))
}

// UpdateWebhookEndpoint implements PATCH /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}.
func (h *Handlers) UpdateWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, _ api.UpdateWebhookEndpointParams) {
	if !h.webhooksReady(w, r) {
		return
	}
	var req api.WebhookEndpointUpdate
	if !decode(w, r, &req) {
		return
	}
	in := webhook.UpdateInput{URL: req.Url, Description: req.Description, Active: req.Active, IP: estClientIP(r)}
	if req.Events != nil {
		in.Events = append([]string{}, *req.Events...)
	}
	ep, err := h.Webhooks.Update(r.Context(), principalOf(r), establishmentID, endpointID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, WebhookEndpointToAPI(ep))
}

// DeleteWebhookEndpoint implements DELETE /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}.
func (h *Handlers) DeleteWebhookEndpoint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, _ api.DeleteWebhookEndpointParams) {
	if !h.webhooksReady(w, r) {
		return
	}
	if err := h.Webhooks.Delete(r.Context(), principalOf(r), establishmentID, endpointID, estClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RotateWebhookSecret implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/rotate-secret.
func (h *Handlers) RotateWebhookSecret(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, _ api.RotateWebhookSecretParams) {
	if !h.webhooksReady(w, r) {
		return
	}
	// The body is optional: an empty body rotates with the default grace.
	var req api.WebhookSecretRotate
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta non leggibile o troppo grande."))
		return
	}
	if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &req) != nil {
		problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
		return
	}
	grace := webhook.DefaultGrace
	if req.GraceHours != nil {
		grace = time.Duration(*req.GraceHours) * time.Hour
	}
	ep, secret, err := h.Webhooks.RotateSecret(r.Context(), principalOf(r), establishmentID, endpointID, grace, estClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.WebhookSecretRotated{Endpoint: WebhookEndpointToAPI(ep), Secret: secret})
}

// SendWebhookTest implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/test.
func (h *Handlers) SendWebhookTest(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, _ api.SendWebhookTestParams) {
	if !h.webhooksReady(w, r) {
		return
	}
	d, err := h.Webhooks.SendTest(r.Context(), principalOf(r), establishmentID, endpointID, estClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, WebhookDeliveryToAPI(d))
}

// ListWebhookDeliveries implements GET /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/deliveries.
func (h *Handlers) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, params api.ListWebhookDeliveriesParams) {
	if !h.webhooksReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := webhook.DeliveryFilter{Cursor: cursor, Limit: limit}
	if params.Status != nil {
		f.Status = webhook.DeliveryStatus(*params.Status)
	}
	items, next, err := h.Webhooks.ListDeliveries(r.Context(), principalOf(r), establishmentID, endpointID, f)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.WebhookDeliveryList{Items: make([]api.WebhookDelivery, len(items)), NextCursor: nextCursor(next)}
	for i := range items {
		out.Items[i] = WebhookDeliveryToAPI(&items[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// GetWebhookDelivery implements GET /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/deliveries/{deliveryId}.
func (h *Handlers) GetWebhookDelivery(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, deliveryID api.WebhookDeliveryID) {
	if !h.webhooksReady(w, r) {
		return
	}
	d, err := h.Webhooks.GetDelivery(r.Context(), principalOf(r), establishmentID, endpointID, deliveryID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	base := WebhookDeliveryToAPI(&d.Delivery)
	out := api.WebhookDeliveryDetail{
		Id: base.Id, EndpointId: base.EndpointId, EventId: base.EventId, EventType: base.EventType, Status: base.Status,
		Attempts: base.Attempts, StatusCode: base.StatusCode, LastError: base.LastError, NextAttemptAt: base.NextAttemptAt,
		DeliveredAt: base.DeliveredAt, FailedAt: base.FailedAt, CreatedAt: base.CreatedAt, UpdatedAt: base.UpdatedAt,
		RequestBody: map[string]any{},
	}
	if err := json.Unmarshal(d.RequestBody, &out.RequestBody); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if d.ResponseExcerpt != "" {
		ex := d.ResponseExcerpt
		out.ResponseExcerpt = &ex
	}
	writeJSON(w, http.StatusOK, out)
}

// RedeliverWebhook implements POST /api/admin/v1/establishments/{establishmentId}/webhook-endpoints/{endpointId}/deliveries/{deliveryId}/redeliver.
func (h *Handlers) RedeliverWebhook(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, deliveryID api.WebhookDeliveryID, _ api.RedeliverWebhookParams) {
	if !h.webhooksReady(w, r) {
		return
	}
	d, err := h.Webhooks.Redeliver(r.Context(), principalOf(r), establishmentID, endpointID, deliveryID, estClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusAccepted, WebhookDeliveryToAPI(d))
}

// WebhookEndpointToAPI maps an endpoint (never a secret).
func WebhookEndpointToAPI(ep *webhook.Endpoint) api.WebhookEndpoint {
	out := api.WebhookEndpoint{
		Id: ep.ID, Url: ep.URL, Description: ep.Description, Events: append([]string{}, ep.Events...),
		Active: ep.Active, Status: api.WebhookEndpointStatus(ep.Status), ConsecutiveFailures: ep.ConsecutiveFailures,
		DisabledAt: utcPtr(ep.DisabledAt), PreviousSecretExpiresAt: utcPtr(ep.PreviousSecretExpiresAt),
		LastDeliveryAt: utcPtr(ep.LastDeliveryAt), CreatedAt: ep.CreatedAt.UTC(), UpdatedAt: ep.UpdatedAt.UTC(),
	}
	if ep.DisabledReason != "" {
		reason := ep.DisabledReason
		out.DisabledReason = &reason
	}
	return out
}

// WebhookDeliveryToAPI maps a delivery log row.
func WebhookDeliveryToAPI(d *webhook.Delivery) api.WebhookDelivery {
	out := api.WebhookDelivery{
		Id: d.ID, EndpointId: d.EndpointID, EventId: d.EventID, EventType: d.EventType,
		Status: api.WebhookDeliveryStatus(d.Status), Attempts: d.Attempts, StatusCode: d.StatusCode,
		NextAttemptAt: utcPtr(d.NextAttemptAt), DeliveredAt: utcPtr(d.DeliveredAt), FailedAt: utcPtr(d.FailedAt),
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
	if d.LastError != "" {
		msg := d.LastError
		out.LastError = &msg
	}
	return out
}

// webhooksReady answers 501 when the webhook service is not wired.
func (h *Handlers) webhooksReady(w http.ResponseWriter, r *http.Request) bool {
	if h.Webhooks == nil {
		notImplemented(w, r)
		return false
	}
	return true
}
