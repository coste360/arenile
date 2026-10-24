package public

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/http/static"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/pkg/ext"
)

// Server-rendered Open Graph / Twitter meta of the public page /b/{slug}
// (deferred features F): link previews on WhatsApp and social networks
// show the establishment instead of the generic Arenile card.

// PublicProfiles returns the public profile of an establishment by slug
// (implemented by establishment.Service).
type PublicProfiles interface {
	GetPublic(ctx context.Context, slug string) (*establishment.Public, error)
}

// pageTexts are the default title and description of the public page per
// locale (same strings as the booking SPA: booking.title, booking.metaDescription).
var pageTexts = map[string]struct{ title, description, ogLocale string }{
	"it": {"Prenota il tuo posto in spiaggia", "Prenota online il tuo posto in spiaggia, senza commissioni.", "it_IT"},
	"en": {"Book your spot on the beach", "Book your beach spot online, commission-free.", "en_GB"},
	"de": {"Buchen Sie Ihren Platz am Strand", "Buchen Sie Ihren Strandplatz online, provisionsfrei.", "de_DE"},
	"fr": {"Réservez votre place à la plage", "Réservez votre place à la plage en ligne, sans commission.", "fr_FR"},
}

// DefaultOGImage is the Arenile social card served with the booking SPA.
const (
	DefaultOGImage       = "/b/og-image.png"
	defaultOGImageWidth  = 1200
	defaultOGImageHeight = 630
	maxDescriptionRunes  = 200
)

// BookingPageMeta returns the resolver of the public page previews. The
// tenant resolver (multi-tenant hosting) is honoured like on the API: a
// slug that is not the host's tenant keeps the default page. baseURL is
// ARENILE_BASE_URL: when set (and no tenant resolver maps hosts to
// establishments) og:url and og:image are built on it, never on the Host
// header a client can forge.
func BookingPageMeta(profiles PublicProfiles, tenants ext.TenantResolver, baseURL string, log *slog.Logger) static.PageMetaFunc {
	origin := ""
	if tenants == nil {
		origin = trimOrigin(baseURL)
	}
	return func(r *http.Request, slug string) (static.PageMeta, bool) {
		if profiles == nil {
			return static.PageMeta{}, false
		}
		if tenants != nil {
			if s, ok, err := tenants.ResolveSlug(r); err != nil || (ok && s != slug) {
				return static.PageMeta{}, false
			}
		}
		e, err := profiles.GetPublic(r.Context(), slug)
		if err != nil {
			if log != nil && r.Context().Err() == nil && !isNotFound(err) {
				log.WarnContext(r.Context(), "public page meta unavailable", slog.String("error", err.Error()))
			}
			return static.PageMeta{}, false
		}
		if origin != "" {
			return PageMetaOf(e, origin), true
		}
		return PageMetaOf(e, requestOrigin(r)), true
	}
}

// trimOrigin reduces a configured base URL to scheme://host ("" when it is
// not an absolute http(s) URL).
func trimOrigin(base string) string {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// PageMetaOf builds the preview of an establishment: "<name> – <title>" in
// its default locale, the custom intro text (or the default description),
// and the establishment logo (or the Arenile card) as image.
func PageMetaOf(e *establishment.Public, origin string) static.PageMeta {
	loc := e.DefaultLocale
	tx, ok := pageTexts[loc]
	if !ok {
		loc, tx = "it", pageTexts["it"]
	}
	desc := tx.description
	if intro := introText(e, loc); intro != "" {
		desc = intro
	}
	m := static.PageMeta{
		Title: e.Name + " – " + tx.title, Description: desc, SiteName: e.Name,
		URL: origin + "/b/" + url.PathEscape(e.Slug), Locale: tx.ogLocale, Lang: loc,
	}
	if logo := absoluteLogo(e.Theme.LogoURL, origin); logo != "" {
		m.Image = logo
	} else {
		m.Image, m.ImageWidth, m.ImageHeight = origin+DefaultOGImage, defaultOGImageWidth, defaultOGImageHeight
	}
	return m
}

// introText is the manager's custom intro in loc (fallback: any locale is
// not used, to avoid a description in the wrong language), whitespace
// collapsed and truncated for previews.
func introText(e *establishment.Public, loc string) string {
	s := strings.Join(strings.Fields(e.Theme.CustomTexts[loc]["intro"]), " ")
	if r := []rune(s); len(r) > maxDescriptionRunes {
		s = strings.TrimSpace(string(r[:maxDescriptionRunes-1])) + "…"
	}
	return s
}

// absoluteLogo resolves the theme logo (validated as https:// or a local
// path by the establishment service) to an absolute URL; "" when unusable.
func absoluteLogo(logo, origin string) string {
	logo = strings.TrimSpace(logo)
	switch {
	case logo == "":
		return ""
	case strings.HasPrefix(logo, "/") && !strings.HasPrefix(logo, "//"):
		return origin + logo
	}
	u, err := url.Parse(logo)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}

// requestOrigin is scheme://host of the request (scheme from the trusted
// proxy headers resolved by middleware.RequestContext).
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if reqctx.From(r.Context()).HTTPS {
		scheme = "https"
	}
	host := r.Host
	if host == "" || strings.ContainsAny(host, "/\\ \"'<>") {
		host = "localhost"
	}
	return scheme + "://" + host
}

func isNotFound(err error) bool { return errors.Is(err, establishment.ErrNotFound) }
