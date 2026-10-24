package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// F. Full delivery attempt history of outgoing webhooks (ADR 0019 F): one
// item per HTTP attempt, newest first; the service checks users.manage.

// ListWebhookDeliveryAttempts implements GET .../deliveries/{deliveryId}/attempts.
func (h *Handlers) ListWebhookDeliveryAttempts(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, endpointID api.WebhookEndpointID, deliveryID api.WebhookDeliveryID, params api.ListWebhookDeliveryAttemptsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Webhooks.ListAttempts(r.Context(), principalOf(r), establishmentID, endpointID, deliveryID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.WebhookDeliveryAttemptList{Items: make([]api.WebhookDeliveryAttempt, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, webhookAttemptToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func webhookAttemptToAPI(a *store.WebhookDeliveryAttempt) api.WebhookDeliveryAttempt {
	return api.WebhookDeliveryAttempt{
		Id: a.ID, DeliveryId: a.DeliveryID, Attempt: a.Attempt, StatusCode: a.StatusCode,
		Error: a.Error, ResponseExcerpt: a.ResponseExcerpt, DurationMs: a.DurationMS, AttemptedAt: a.AttemptedAt,
	}
}
