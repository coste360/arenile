package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of the fiscal domain (internal/domain/fiscal).
//
// M3 scaffolding: every operation answers 501 not_implemented until the
// owner of the domain package implements it (decode, call the service,
// encode; see maps.go and catalog.go for the patterns).

// GetCorrispettivi implements GET /api/admin/v1/establishments/{establishmentId}/fiscal/corrispettivi.
func (h *Handlers) GetCorrispettivi(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetCorrispettiviParams) {
	notImplemented(w, r)
}

// ListFiscalDocuments implements GET /api/admin/v1/establishments/{establishmentId}/fiscal/documents.
func (h *Handlers) ListFiscalDocuments(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListFiscalDocumentsParams) {
	notImplemented(w, r)
}
