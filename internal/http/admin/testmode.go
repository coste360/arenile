package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/testmode"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// A. Test mode (spec 15.4, ADR 0019): switch, preview link of the public
// page and bulk purge of the test bookings. Owner: internal/domain/testmode.

// GetTestMode implements GET /establishments/{id}/test-mode.
func (h *Handlers) GetTestMode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if h.TestMode == nil {
		notImplemented(w, r)
		return
	}
	st, err := h.TestMode.Get(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, st.Version, testModeToAPI(st))
}

// UpdateTestMode implements PUT /establishments/{id}/test-mode.
func (h *Handlers) UpdateTestMode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdateTestModeParams) {
	if h.TestMode == nil {
		notImplemented(w, r)
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.TestModeInput
	if !decode(w, r, &req) {
		return
	}
	in := testmode.Input{Enabled: req.Enabled}
	if req.PublicEnabled != nil {
		in.PublicEnabled = *req.PublicEnabled
	}
	st, err := h.TestMode.Update(r.Context(), principalOf(r), establishmentID, expected, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, st.Version, testModeToAPI(st))
}

// CreateTestModePreviewLink implements POST /establishments/{id}/test-mode/preview-link.
func (h *Handlers) CreateTestModePreviewLink(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateTestModePreviewLinkParams) {
	if h.TestMode == nil {
		notImplemented(w, r)
		return
	}
	link, err := h.TestMode.PreviewLink(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.TestModePreviewLink{Url: link.URL, Token: link.Token, ExpiresAt: link.ExpiresAt.UTC()})
}

// PurgeTestBookings implements POST /establishments/{id}/test-bookings/purge.
func (h *Handlers) PurgeTestBookings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.PurgeTestBookingsParams) {
	if h.TestMode == nil {
		notImplemented(w, r)
		return
	}
	var req api.TestBookingsPurgeRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.TestMode.Purge(r.Context(), principalOf(r), establishmentID, req.DryRun != nil && *req.DryRun)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.TestBookingsPurgeResult{Bookings: res.Bookings, Payments: res.Payments, Deleted: res.Deleted})
}

func testModeToAPI(st *testmode.Status) api.TestMode {
	out := api.TestMode{
		Enabled: st.Enabled, PublicEnabled: st.PublicEnabled, TestBookingsCount: st.TestBookings,
		StripeTestConfigured: st.StripeTestConfigured, Version: st.Version,
		EnabledByUserId: st.EnabledByUserID,
	}
	if st.EnabledAt != nil {
		t := st.EnabledAt.UTC()
		out.EnabledAt = &t
	}
	return out
}
