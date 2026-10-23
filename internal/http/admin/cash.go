package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of the cash domain (internal/domain/cash).
//
// M3 scaffolding: every operation answers 501 not_implemented until the
// owner of the domain package implements it (decode, call the service,
// encode; see maps.go and catalog.go for the patterns).

// ListCashSessions implements GET /api/admin/v1/establishments/{establishmentId}/cash-sessions.
func (h *Handlers) ListCashSessions(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListCashSessionsParams) {
	notImplemented(w, r)
}

// OpenCashSession implements POST /api/admin/v1/establishments/{establishmentId}/cash-sessions.
func (h *Handlers) OpenCashSession(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.OpenCashSessionParams) {
	notImplemented(w, r)
}

// GetCurrentCashSession implements GET /api/admin/v1/establishments/{establishmentId}/cash-sessions/current.
func (h *Handlers) GetCurrentCashSession(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetCurrentCashSessionParams) {
	notImplemented(w, r)
}

// GetCashSession implements GET /api/admin/v1/establishments/{establishmentId}/cash-sessions/{cashSessionId}.
func (h *Handlers) GetCashSession(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CashSessionID) {
	notImplemented(w, r)
}

// AddCashMovement implements POST /api/admin/v1/establishments/{establishmentId}/cash-sessions/{cashSessionId}/movements.
func (h *Handlers) AddCashMovement(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CashSessionID, _ api.AddCashMovementParams) {
	notImplemented(w, r)
}

// CloseCashSession implements POST /api/admin/v1/establishments/{establishmentId}/cash-sessions/{cashSessionId}/close.
func (h *Handlers) CloseCashSession(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CashSessionID, _ api.CloseCashSessionParams) {
	notImplemented(w, r)
}
