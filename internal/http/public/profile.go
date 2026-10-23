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

// onlineConfig maps online.PublicEstablishment to api.PublicOnlineConfig.
// Owner: online settings builder (zones, maps, formulas, slots, terms,
// payment). Until implemented, Online.ResolvePublic answers
// configerr.ErrNotImplemented and the profile omits `online`.
func (h *Handlers) onlineConfig(r *http.Request, slug string) (*api.PublicOnlineConfig, error) {
	if h.Online == nil {
		return nil, configerr.ErrNotImplemented
	}
	if _, err := h.Online.ResolvePublic(r.Context(), slug); err != nil {
		return nil, err
	}
	return nil, configerr.ErrNotImplemented
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
