package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// B. Guaranteed refunds of cancellations: contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// ListRefundRequests is not implemented yet.
func (h *Handlers) ListRefundRequests(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListRefundRequestsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// RetryRefundRequest is not implemented yet.
func (h *Handlers) RetryRefundRequest(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RefundRequestID, _ api.RetryRefundRequestParams) {
	problem.Write(w, r, problem.NotImplemented)
}
