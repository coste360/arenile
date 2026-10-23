package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// A. Test mode (spec 15.4): contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// GetTestMode is not implemented yet.
func (h *Handlers) GetTestMode(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateTestMode is not implemented yet.
func (h *Handlers) UpdateTestMode(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.UpdateTestModeParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateTestModePreviewLink is not implemented yet.
func (h *Handlers) CreateTestModePreviewLink(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateTestModePreviewLinkParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// PurgeTestBookings is not implemented yet.
func (h *Handlers) PurgeTestBookings(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PurgeTestBookingsParams) {
	problem.Write(w, r, problem.NotImplemented)
}
