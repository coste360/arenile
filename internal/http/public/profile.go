package public

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// GetPublicEstablishment implements GET /api/public/v1/{slug}: profile,
// theme, languages and (M4) the online flow configuration, with a content
// ETag and If-None-Match support.
func (h *Handlers) GetPublicEstablishment(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.GetPublicEstablishmentParams) {
	if h.TenantResolver != nil {
		if s, ok, err := h.TenantResolver.ResolveSlug(r); err != nil {
			problem.Error(w, r, h.Log, err)
			return
		} else if ok && s != slug {
			problem.Write(w, r, problem.NotFound)
			return
		}
	}
	e, err := h.Establishments.GetPublic(r.Context(), slug)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PublicEstablishment{
		Slug: e.Slug, Name: e.Name, Timezone: e.Timezone, Currency: e.Currency, DefaultLocale: e.DefaultLocale,
		EnabledLocales: e.EnabledLocales, BookingEnabled: e.BookingEnabled, Theme: admin.ThemeToAPI(e.Theme),
	}
	if out.EnabledLocales == nil {
		out.EnabledLocales = []string{}
	}
	if e.ContactEmail != "" {
		out.ContactEmail = &e.ContactEmail
	}
	if e.ContactPhone != "" {
		out.ContactPhone = &e.ContactPhone
	}
	addr := admin.AddressToAPI(e.Address)
	if addr != (api.Address{}) {
		out.Address = &addr
	}
	if e.Season != nil {
		out.Season = &api.PublicSeason{Year: e.Season.Year, OpensOn: e.Season.OpensOn, ClosesOn: e.Season.ClosesOn}
	}
	if e.BookingEnabled {
		cfg, err := h.onlineConfig(r, slug)
		switch {
		case err == nil:
			out.Online = cfg
		case errors.Is(err, configerr.ErrNotImplemented), errors.Is(err, configerr.ErrNotFound):
			// M4 scaffolding or booking switched off meanwhile: profile only.
		default:
			problem.Error(w, r, h.Log, err)
			return
		}
	}

	body, err := json.Marshal(out)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	sum := sha256.Sum256(body)
	tag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.Header().Set("Vary", "Accept-Encoding")
	if params.IfNoneMatch != nil && etagMatches(*params.IfNoneMatch, tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(body, '\n'))
}

// onlineConfig builds api.PublicOnlineConfig from the checkout profile
// (zones visible online, maps, formulas, slots, current terms, payment).
// configerr.ErrNotFound when online booking is off: the profile omits it.
func (h *Handlers) onlineConfig(r *http.Request, slug string) (*api.PublicOnlineConfig, error) {
	if h.Checkout == nil {
		return nil, configerr.ErrNotImplemented
	}
	p, err := h.Checkout.Profile(r.Context(), slug)
	if err != nil {
		return nil, err
	}
	pe := p.Establishment
	out := &api.PublicOnlineConfig{
		Formulas: formulas(p.Formulas), HoldMinutes: p.HoldMinutes, MinDate: p.MinDate, MaxDate: p.MaxDate,
		PromoCodesEnabled: pe.Flow.PromoCodesEnabled, RequirePhone: false, // deprecated (ADR 0020): phone optional ShowRatePeriods: pe.Flow.ShowRatePeriods,
		ShowPricesOnMap: pe.Establishment.Theme.ShowPricesOnMap == nil || *pe.Establishment.Theme.ShowPricesOnMap,
		Payment: api.PublicPaymentConfig{
			Provider: pe.Provider, Online: pe.Online, Embedded: pe.Embedded, TestMode: pe.TestMode, PublishableKey: optStr(pe.PublishableKey),
		},
		Maps:  make([]api.PublicMapRef, len(p.Maps)),
		Zones: make([]api.PublicZone, len(p.Zones)),
		Slots: make([]api.Slots, len(p.Slots)),
	}
	for i, sl := range p.Slots {
		out.Slots[i] = api.Slots(sl)
	}
	for i, m := range p.Maps {
		out.Maps[i] = api.PublicMapRef{Id: m.ID, Name: text(m.Name)}
	}
	for i, z := range p.Zones {
		out.Zones[i] = api.PublicZone{Id: z.ID, Code: z.Code, Name: text(z.Name), AssignmentMode: api.AssignmentMode(z.AssignmentMode), MapId: z.MapID, SortOrder: z.SortOrder}
	}
	if t := pe.Terms; t != nil {
		texts := make(map[string]api.TermsTexts, len(t.Texts))
		for l, tx := range t.Texts {
			texts[l] = api.TermsTexts{Terms: tx.Terms, Privacy: optStr(tx.Privacy), Marketing: optStr(tx.Marketing)}
		}
		out.Terms = &api.PublicTerms{Version: t.Version, Texts: texts}
	}
	return out, nil
}

// etagMatches implements the weak comparison of If-None-Match (RFC 9110
// 13.1.2): a list of tags, "*" or W/ prefixes.
func etagMatches(header, tag string) bool {
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimSpace(t)
		if t == "*" || strings.TrimPrefix(t, "W/") == tag {
			return true
		}
	}
	return false
}
