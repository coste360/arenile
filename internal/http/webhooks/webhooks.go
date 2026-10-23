// Package webhooks implements the incoming payment provider webhooks
// (spec 13.3): POST /webhooks/payments/{provider}/{establishment_id}.
//
// The route lives outside /api/, so no session, CSRF, JSON content-type
// check, public rate limit or idempotency middleware applies: the request
// is authenticated only by the provider signature, verified by the driver
// with the decrypted credentials of the establishment in the URL. Owner:
// payment gateway builder.
package webhooks

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Handlers implements the webhook operations of api.ServerInterface.
type Handlers struct {
	Gateway *gateway.Service
	Log     *slog.Logger
}

// ReceivePaymentWebhook implements POST /webhooks/payments/{provider}/{establishmentId}.
// The body is passed raw (bounded by gateway.MaxWebhookBody) to the
// gateway, which verifies the signature, deduplicates the event and applies
// it in one transaction. Never log the body or the signature header.
func (h *Handlers) ReceivePaymentWebhook(w http.ResponseWriter, r *http.Request, provider string, establishmentID api.EstablishmentID) {
	if h.Gateway == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, gateway.MaxWebhookBody)
	out, err := h.Gateway.HandleWebhook(r.Context(), provider, establishmentID, r)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	ack := api.WebhookAck{Received: true}
	if out.Duplicate {
		dup := true
		ack.Duplicate = &dup
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ack)
}
