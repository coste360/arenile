package admin

import (
	"net/http"
	"slices"
	"strings"

	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Online settings of the public page (M4, spec 12). Owner: online settings
// builder (GetOnlineSettings, UpdateOnlineSettings, ListTermsVersions,
// PublishTermsVersion -> h.Online) and notifications builder
// (ResendBookingConfirmation, RegenerateMagicLink -> h.Notify).
//
//	GetOnlineSettings         -> Online.Get            -> api.OnlineSettings + ETag
//	UpdateOnlineSettings      -> Online.Update         -> api.OnlineSettings + ETag (If-Match, 412 version_conflict)
//	ListTermsVersions         -> Online.ListTerms      -> api.TermsVersionList
//	PublishTermsVersion       -> Online.PublishTerms   -> 201 api.TermsVersion
//	ResendBookingConfirmation -> Notify.ResendConfirmation -> 202 api.MessageQueued
//	RegenerateMagicLink       -> Notify.RegenerateLink -> 201 api.MagicLinkResult (Cache-Control no-store)

// GetOnlineSettings implements GET /api/admin/v1/establishments/{establishmentId}/online-settings.
func (h *Handlers) GetOnlineSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if h.Online == nil {
		notImplemented(w, r)
		return
	}
	s, err := h.Online.Get(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, OnlineSettingsToAPI(s))
}

// UpdateOnlineSettings implements PUT /api/admin/v1/establishments/{establishmentId}/online-settings.
func (h *Handlers) UpdateOnlineSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdateOnlineSettingsParams) {
	if h.Online == nil {
		notImplemented(w, r)
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.OnlineSettingsInput
	if !decode(w, r, &req) {
		return
	}
	in := online.Input{
		BookingEnabled: req.BookingEnabled, AllowedEmbedOrigins: req.AllowedEmbedOrigins, Theme: ThemeFromAPI(req.Theme),
		Flow: onlineFlowFromAPI(req.Flow),
	}
	s, err := h.Online.Update(r.Context(), principalOf(r), establishmentID, expected, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, OnlineSettingsToAPI(s))
}

// ListTermsVersions implements GET /api/admin/v1/establishments/{establishmentId}/terms-versions.
func (h *Handlers) ListTermsVersions(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if h.Online == nil {
		notImplemented(w, r)
		return
	}
	items, current, err := h.Online.ListTerms(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.TermsVersionList{Items: make([]api.TermsVersion, len(items))}
	for i := range items {
		out.Items[i] = TermsVersionToAPI(&items[i], items[i].ID == current)
	}
	writeJSON(w, http.StatusOK, out)
}

// PublishTermsVersion implements POST /api/admin/v1/establishments/{establishmentId}/terms-versions.
func (h *Handlers) PublishTermsVersion(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.PublishTermsVersionParams) {
	if h.Online == nil {
		notImplemented(w, r)
		return
	}
	var req api.TermsVersionInput
	if !decode(w, r, &req) {
		return
	}
	in := online.TermsInput{Version: req.Version, Texts: map[string]store.TermsTexts{}, MakeCurrent: req.MakeCurrent == nil || *req.MakeCurrent}
	for l, t := range req.Texts {
		in.Texts[l] = store.TermsTexts{Terms: t.Terms, Privacy: estDeref(t.Privacy), Marketing: estDeref(t.Marketing)}
	}
	tv, err := h.Online.PublishTerms(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, TermsVersionToAPI(tv, in.MakeCurrent))
}

// ResendBookingConfirmation implements POST .../bookings/{bookingId}/resend-confirmation.
func (h *Handlers) ResendBookingConfirmation(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, bookingID api.BookingID, params api.ResendBookingConfirmationParams) {
	if h.Notify == nil {
		notImplemented(w, r)
		return
	}
	m, err := h.Notify.ResendConfirmation(r.Context(), principalOf(r), establishmentID, bookingID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusAccepted, api.MessageQueued{MessageId: m.ID})
}

// RegenerateMagicLink implements POST .../bookings/{bookingId}/magic-link.
// The link is returned once and never logged.
func (h *Handlers) RegenerateMagicLink(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, bookingID api.BookingID, params api.RegenerateMagicLinkParams) {
	w.Header().Set("Cache-Control", "no-store")
	if h.Notify == nil {
		notImplemented(w, r)
		return
	}
	var req api.MagicLinkRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	send := req.SendEmail == nil || *req.SendEmail
	link, sent, err := h.Notify.RegenerateLink(r.Context(), principalOf(r), establishmentID, bookingID, send)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.MagicLinkResult{ExpiresAt: link.ExpiresAt.UTC(), Sent: sent}
	if link.URL != "" {
		u := link.URL
		out.ManageUrl = &u
	}
	writeJSON(w, http.StatusCreated, out)
}

// ---- mapping ----------------------------------------------------------------------

func onlineFlowFromAPI(f api.OnlineFlowConfig) online.Config {
	out := online.Config{
		ShowRatePeriods: f.ShowRatePeriods, PromoCodesEnabled: f.PromoCodesEnabled, RequirePhone: f.RequirePhone,
	}
	for _, fm := range f.Formulas {
		out.Formulas = append(out.Formulas, string(fm))
	}
	for _, s := range f.Slots {
		out.Slots = append(out.Slots, string(s))
	}
	if f.MaxDaysAhead != nil {
		out.MaxDaysAhead = *f.MaxDaysAhead
	}
	return out
}

func onlineFlowToAPI(c online.Config) api.OnlineFlowConfig {
	out := api.OnlineFlowConfig{
		Formulas: make([]api.Formula, 0, len(c.Formulas)), Slots: make([]api.Slots, 0, len(c.Slots)),
		ShowRatePeriods: c.ShowRatePeriods, PromoCodesEnabled: c.PromoCodesEnabled, RequirePhone: c.RequirePhone,
	}
	for _, f := range c.Formulas {
		out.Formulas = append(out.Formulas, api.Formula(f))
	}
	for _, s := range c.EffectiveSlots() {
		out.Slots = append(out.Slots, api.Slots(s))
	}
	if c.MaxDaysAhead > 0 {
		n := c.MaxDaysAhead
		out.MaxDaysAhead = &n
	}
	return out
}

// OnlineSettingsToAPI maps the online settings with the embed snippets.
func OnlineSettingsToAPI(s *online.Settings) api.OnlineSettings {
	out := api.OnlineSettings{
		AllowedEmbedOrigins: slices.Clone(s.AllowedEmbedOrigins), BookingEnabled: s.BookingEnabled,
		Theme: ThemeToAPI(s.Theme), Flow: onlineFlowToAPI(s.Flow), Version: s.Version, Embed: EmbedSnippets(s.PageURL, s.Slug),
	}
	if out.AllowedEmbedOrigins == nil {
		out.AllowedEmbedOrigins = []string{}
	}
	if s.CurrentTerms != nil {
		tv := TermsVersionToAPI(s.CurrentTerms, true)
		out.CurrentTerms = &tv
	}
	return out
}

// EmbedSnippets builds the iframe and script snippets of spec 12.1 for the
// public page pageURL (<base>/b/<slug>).
func EmbedSnippets(pageURL, slug string) api.EmbedSnippets {
	base := strings.TrimSuffix(pageURL, "/b/"+slug)
	return api.EmbedSnippets{
		PageUrl: pageURL,
		Iframe: `<iframe src="` + pageURL + `?embed=1" style="width:100%;border:0" allow="payment" loading="lazy" ` +
			`title="Prenota il tuo posto"></iframe>`,
		Script: `<div data-arenile="` + slug + `" data-lang="it"></div>` + "\n" + `<script src="` + base + `/embed.js" async></script>`,
	}
}

// TermsVersionToAPI maps a terms version.
func TermsVersionToAPI(tv *store.TermsVersion, current bool) api.TermsVersion {
	out := api.TermsVersion{Id: tv.ID, Version: tv.Version, PublishedAt: tv.PublishedAt.UTC(), Current: current, Texts: map[string]api.TermsTexts{}}
	for l, t := range tv.Texts {
		tt := api.TermsTexts{Terms: t.Terms}
		if t.Privacy != "" {
			p := t.Privacy
			tt.Privacy = &p
		}
		if t.Marketing != "" {
			m := t.Marketing
			tt.Marketing = &m
		}
		out.Texts[l] = tt
	}
	return out
}
