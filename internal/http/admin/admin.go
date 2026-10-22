// Package admin implements the thin HTTP handlers of the admin API
// (/api/admin/v1): decode, call the domain service, encode.
package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/catalog"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/season"
	"github.com/coste360/arenile/internal/domain/setup"
	"github.com/coste360/arenile/internal/domain/team"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/httpctx"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/http/sse"
	"github.com/coste360/arenile/internal/store"
)

// Handlers implements the admin operations of api.ServerInterface. The
// configuration handlers (M1/M2) live in one file per area, each owned by
// the builder of the matching domain package: establishments_write.go,
// team.go, seasons.go, maps.go, catalog.go, pricing.go, policies.go, setup.go.
type Handlers struct {
	Auth           *auth.Service
	Establishments *establishment.Service
	Team           *team.Service
	Seasons        *season.Service
	Mapping        *mapping.Service
	Catalog        *catalog.Service
	Pricing        *pricingconfig.Service
	Policies       *pricingconfig.PolicyService
	Setup          *setup.Service
	Stream         *sse.Server
	Log            *slog.Logger
	// SecureCookies sets the Secure attribute on cookies (false only in dev over http).
	SecureCookies bool
}

// Login implements POST /api/admin/v1/auth/login.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req api.LoginRequest
	if !decode(w, r, &req) {
		return
	}
	info := httpctx.From(r.Context())
	in := auth.LoginInput{
		Email:     string(req.Email),
		Password:  req.Password,
		IP:        info.ClientIP,
		UserAgent: r.UserAgent(),
	}
	if req.TotpCode != nil {
		in.TOTPCode = *req.TotpCode
	}
	if c, err := r.Cookie(middleware.SessionCookie); err == nil {
		in.PreviousToken = c.Value
	}
	res, err := h.Auth.Login(r.Context(), in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	ttl := h.Auth.SessionTTL()
	// Secure is configurable so that plain-HTTP local development works.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure set from config (HTTPS base URL)
		Name: middleware.SessionCookie, Value: res.Token, Path: "/", HttpOnly: true, Secure: h.SecureCookies,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds()), Expires: res.ExpiresAt,
	})
	// Rotate the CSRF token together with the session.
	middleware.SetCSRFCookie(w, middleware.NewCSRFToken(), h.SecureCookies, ttl)
	writeJSON(w, http.StatusOK, MeFromPrincipal(res.Principal))
}

// Logout implements POST /api/admin/v1/auth/logout.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request, _ api.LogoutParams) {
	info := httpctx.From(r.Context())
	p, _ := auth.PrincipalFrom(r.Context())
	if err := h.Auth.Logout(r.Context(), info.SessionToken, p.Actor(info.ClientIP)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure set from config (HTTPS base URL)
		Name: middleware.SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// GetMe implements GET /api/admin/v1/auth/me.
func (h *Handlers) GetMe(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		problem.Write(w, r, problem.Unauthorized)
		return
	}
	writeJSON(w, http.StatusOK, MeFromPrincipal(p))
}

// EnrollTotp implements POST /api/admin/v1/auth/totp/enroll.
func (h *Handlers) EnrollTotp(w http.ResponseWriter, r *http.Request, _ api.EnrollTotpParams) {
	p, _ := auth.PrincipalFrom(r.Context())
	enr, err := h.Auth.BeginTOTPEnrolment(r.Context(), p, httpctx.From(r.Context()).ClientIP)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.TotpEnrollment{Secret: enr.Secret, OtpauthUrl: enr.OTPAuthURL})
}

// ConfirmTotp implements POST /api/admin/v1/auth/totp/confirm.
func (h *Handlers) ConfirmTotp(w http.ResponseWriter, r *http.Request, _ api.ConfirmTotpParams) {
	var req api.TotpConfirmRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.Code) != 6 || strings.Trim(req.Code, "0123456789") != "" {
		problem.Write(w, r, problem.ValidationFailed, problem.Fields(api.FieldError{Field: "code", Message: "Inserisci le 6 cifre del codice."}))
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	if err := h.Auth.ConfirmTOTP(r.Context(), p, req.Code, httpctx.From(r.Context()).ClientIP); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListEstablishments implements GET /api/admin/v1/establishments.
func (h *Handlers) ListEstablishments(w http.ResponseWriter, r *http.Request, params api.ListEstablishmentsParams) {
	limit := establishment.DefaultPageSize
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > establishment.MaxPageSize {
			problem.Write(w, r, problem.ValidationFailed, problem.Fields(api.FieldError{Field: "limit", Message: "Deve essere tra 1 e 200."}))
			return
		}
		limit = *params.Limit
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	p, _ := auth.PrincipalFrom(r.Context())
	page, err := h.Establishments.List(r.Context(), p, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.EstablishmentList{Items: make([]api.Establishment, 0, len(page.Items))}
	for i := range page.Items {
		out.Items = append(out.Items, EstablishmentToAPI(&page.Items[i]))
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	writeJSON(w, http.StatusOK, out)
}

// GetEstablishment implements GET /api/admin/v1/establishments/{establishmentId}.
func (h *Handlers) GetEstablishment(w http.ResponseWriter, r *http.Request, id api.EstablishmentID) {
	p, _ := auth.PrincipalFrom(r.Context())
	e, err := h.Establishments.Get(r.Context(), p, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, e.Version, EstablishmentToAPI(e))
}

// StreamEvents implements GET /api/admin/v1/stream.
func (h *Handlers) StreamEvents(w http.ResponseWriter, r *http.Request, params api.StreamEventsParams) {
	p, _ := auth.PrincipalFrom(r.Context())
	e, err := h.Establishments.Get(r.Context(), p, params.Establishment)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Stream.Serve(w, r, e.ID); err != nil {
		h.Log.WarnContext(r.Context(), "sse stream", slog.String("error", err.Error()))
	}
}

// MeFromPrincipal maps a principal to the API representation.
func MeFromPrincipal(p *auth.Principal) api.Me {
	me := api.Me{Memberships: []api.Membership{}, TotpEnrolmentRequired: p.TOTPEnrolmentRequired}
	if p.Kind == auth.PrincipalAPIKey {
		me.PrincipalType = api.MePrincipalType("api_key")
		scopes := permissionsToAPI(p.Scopes)
		me.ApiKey = &api.ApiKeyInfo{Id: p.APIKeyID, Name: p.APIKeyName, Prefix: p.APIKeyPrefix, EstablishmentId: p.APIKeyEstablishmentID, Scopes: scopes}
		return me
	}
	me.PrincipalType = api.MePrincipalType("user")
	me.User = &api.User{Id: p.UserID, Email: p.Email, Name: p.Name, Locale: p.Locale, TotpEnabled: p.TOTPEnabled, LastLoginAt: p.LastLoginAt}
	for _, g := range p.Grants {
		m := api.Membership{OrganizationId: g.OrganizationID, Role: api.Role(g.Role), Permissions: permissionsToAPI(auth.RolePermissions(g.Role))}
		if g.EstablishmentID != "" {
			est := g.EstablishmentID
			m.EstablishmentId = &est
		}
		me.Memberships = append(me.Memberships, m)
	}
	return me
}

func permissionsToAPI(ps []auth.Permission) []api.Permission {
	out := make([]api.Permission, 0, len(ps))
	for _, p := range ps {
		out = append(out, api.Permission(p))
	}
	return out
}

// EstablishmentToAPI maps a stored establishment to the API representation.
func EstablishmentToAPI(e *store.Establishment) api.Establishment {
	out := api.Establishment{
		Id: e.ID, OrganizationId: e.OrganizationID, Slug: e.Slug, Name: e.Name, Timezone: e.Timezone,
		Currency: e.Currency, DefaultLocale: e.DefaultLocale, EnabledLocales: nonNil(e.EnabledLocales),
		ContactEmail: e.ContactEmail, ContactPhone: e.ContactPhone, Address: AddressToAPI(e.Address),
		ConcessionRef: e.ConcessionRef, BookingEnabled: e.BookingEnabled, AllowedEmbedOrigins: nonNil(e.AllowedEmbedOrigins),
		Theme: ThemeToAPI(e.Theme), ArchivedAt: utcPtr(e.ArchivedAt), CreatedAt: e.CreatedAt.UTC(), UpdatedAt: e.UpdatedAt.UTC(),
		Version: max(e.Version, 1),
	}
	// A malformed stored document falls back to the defaults (never fails a read).
	settings, _ := establishment.ParseSettings(e.Settings, e.Currency)
	out.Settings = SettingsToAPI(settings)
	if e.Geo != nil {
		out.Geo = &api.Geo{Lat: e.Geo.Lat, Lon: e.Geo.Lon}
	}
	return out
}

// SettingsToAPI maps the establishment settings document.
func SettingsToAPI(s establishment.Settings) api.EstablishmentSettings {
	out := api.EstablishmentSettings{
		Pricing: api.PricingSettings{
			HalfDayFactor:    s.Pricing.HalfDayFactor.String(),
			Rounding:         api.Rounding(s.Pricing.Rounding),
			DefaultVatRateBp: s.Pricing.DefaultVATRateBP,
		},
	}
	if len(s.Pricing.LocalHolidays) > 0 {
		lh := append([]api.Date(nil), s.Pricing.LocalHolidays...)
		out.Pricing.LocalHolidays = &lh
	}
	if len(s.Pricing.RecurringLocalHolidays) > 0 {
		rh := make([]api.MonthDay, len(s.Pricing.RecurringLocalHolidays))
		for i, md := range s.Pricing.RecurringLocalHolidays {
			rh[i] = md.String()
		}
		out.Pricing.RecurringLocalHolidays = &rh
	}
	// policy.Rules and api.PolicyRules share the JSON shape of spec 9.
	_ = ConvertJSON(s.DefaultPolicy, &out.DefaultPolicy)
	return out
}

// ConvertJSON copies src into dst through their JSON encoding. Use it between
// API models and engine types that share the same JSON shape (PolicyRules <->
// policy.Rules, PriceConditions <-> pricing.Conditions, API -> pricing.Action).
// Note: pricing.Decimal marshals as a JSON number while api.Decimal is a
// string, so engine -> API conversions of actions need Decimal.String().
func ConvertJSON(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("admin: convert: %w", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("admin: convert: %w", err)
	}
	return nil
}

// AddressToAPI maps an address.
func AddressToAPI(a store.Address) api.Address {
	return api.Address{Street: strPtr(a.Street), City: strPtr(a.City), Province: strPtr(a.Province), PostalCode: strPtr(a.PostalCode), Country: strPtr(a.Country)}
}

// ThemeToAPI maps a theme.
func ThemeToAPI(t store.Theme) api.Theme {
	out := api.Theme{
		LogoUrl: strPtr(t.LogoURL), PrimaryColor: strPtr(t.PrimaryColor), AccentColor: strPtr(t.AccentColor),
		ShowPricesOnMap: t.ShowPricesOnMap, HideHeaderInEmbed: t.HideHeaderInEmbed,
	}
	if t.FontFamily != "" {
		f := api.ThemeFontFamily(t.FontFamily)
		out.FontFamily = &f
	}
	if t.Radius != "" {
		rd := api.ThemeRadius(t.Radius)
		out.Radius = &rd
	}
	if len(t.CustomTexts) > 0 {
		ct := t.CustomTexts
		out.CustomTexts = &ct
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// decode reads a JSON body into v, writing a problem on failure.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta troppo grande."))
			return false
		}
		problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
