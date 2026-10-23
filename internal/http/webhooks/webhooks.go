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
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
)

// Rejections per IP tolerated before the endpoint answers 429 without
// looking at the request (the route is outside /api/ and its limits):
// every unsigned request would otherwise cost a settings read and a key
// ring decryption. Genuine providers never fail the signature.
const (
	RejectionsPerIP  = 30
	rejectionsWindow = time.Minute
)

// Handlers implements the webhook operations of api.ServerInterface.
type Handlers struct {
	Gateway *gateway.Service
	Log     *slog.Logger
	// Clock drives the rejection limiter (optional: no limiter without it).
	Clock clock.Clock

	rejected *auth.RateLimiter
}

// New returns the handlers with the rejection limiter.
func New(gw *gateway.Service, c clock.Clock, log *slog.Logger) *Handlers {
	h := &Handlers{Gateway: gw, Log: log, Clock: c}
	if c != nil {
		h.rejected = auth.NewRateLimiter(c, RejectionsPerIP, rejectionsWindow)
	}
	return h
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
	ip := reqctx.From(r.Context()).ClientIP
	if h.rejected != nil {
		if ok, retry := h.rejected.Peek(ip); !ok {
			problem.Write(w, r, problem.RateLimited, problem.RetryAfter(retry))
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, gateway.MaxWebhookBody)
	out, err := h.Gateway.HandleWebhook(r.Context(), provider, establishmentID, r)
	if err != nil {
		if h.rejected != nil && (errors.Is(err, gateway.ErrSignatureInvalid) || errors.Is(err, configerr.ErrNotFound)) {
			h.rejected.Hit(ip)
		}
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
