package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Find spot (spec 30.6, ADR 0028). Owner: the find-spot builder.
//
//	FindSpot             -> SpotFinder.Search -> api.FindSpotResult
//	BookFindSpotProposal -> SpotFinder.Book   -> api.BookingDetail (201)

// FindSpot implements POST /find-spot.
func (h *Handlers) FindSpot(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.FindSpotParams) {
	notImplemented(w, r)
}

// BookFindSpotProposal implements POST /find-spot/book.
func (h *Handlers) BookFindSpotProposal(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookFindSpotProposalParams) {
	notImplemented(w, r)
}
