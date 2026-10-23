package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// F. Full delivery attempt history of outgoing webhooks: contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// ListWebhookDeliveryAttempts is not implemented yet.
func (h *Handlers) ListWebhookDeliveryAttempts(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.WebhookEndpointID, _ api.WebhookDeliveryID, _ api.ListWebhookDeliveryAttemptsParams) {
	problem.Write(w, r, problem.NotImplemented)
}
