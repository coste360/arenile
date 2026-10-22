package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the establishment configuration (spec 6.1, 12.5): create
// (organization-wide owners), partial update with If-Match, archive and
// restore (owners). Owned by the establishment builder
// (internal/domain/establishment: Service.Create, Service.Update).

// CreateEstablishment implements POST /establishments.
func (h *Handlers) CreateEstablishment(w http.ResponseWriter, r *http.Request, _ api.CreateEstablishmentParams) {
	var req api.EstablishmentCreate
	if !decode(w, r, &req) {
		return
	}
	in := establishment.CreateInput{
		OrganizationID: estDeref(req.OrganizationId), Slug: req.Slug, Name: req.Name,
		Timezone: estDeref(req.Timezone), Currency: estDeref(req.Currency), DefaultLocale: estDeref(req.DefaultLocale),
		ContactEmail: estDeref(req.ContactEmail), ContactPhone: estDeref(req.ContactPhone), IP: estClientIP(r),
	}
	if req.EnabledLocales != nil {
		in.EnabledLocales = *req.EnabledLocales
	}
	if req.Address != nil {
		in.Address = AddressFromAPI(*req.Address)
	}
	e, err := h.Establishments.Create(r.Context(), principalOf(r), in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, e.Version, EstablishmentToAPI(e))
}

// UpdateEstablishment implements PATCH /establishments/{establishmentId}.
func (h *Handlers) UpdateEstablishment(w http.ResponseWriter, r *http.Request, id api.EstablishmentID, params api.UpdateEstablishmentParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.EstablishmentUpdate
	if !decode(w, r, &req) {
		return
	}
	in, err := establishmentUpdateFromAPI(req)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	in.IfMatch, in.IP = ifMatch, estClientIP(r)
	e, err := h.Establishments.Update(r.Context(), principalOf(r), id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, e.Version, EstablishmentToAPI(e))
}

func establishmentUpdateFromAPI(req api.EstablishmentUpdate) (establishment.UpdateInput, error) {
	in := establishment.UpdateInput{
		Name: req.Name, Slug: req.Slug, Timezone: req.Timezone, Currency: req.Currency,
		DefaultLocale: req.DefaultLocale, EnabledLocales: req.EnabledLocales,
		ContactEmail: req.ContactEmail, ContactPhone: req.ContactPhone, ConcessionRef: req.ConcessionRef,
		BookingEnabled: req.BookingEnabled, AllowedEmbedOrigins: req.AllowedEmbedOrigins, Archived: req.Archived,
	}
	if req.Address != nil {
		a := AddressFromAPI(*req.Address)
		in.Address = &a
	}
	if req.Geo != nil {
		in.Geo = &store.Geo{Lat: req.Geo.Lat, Lon: req.Geo.Lon}
	}
	if req.Theme != nil {
		t := ThemeFromAPI(*req.Theme)
		in.Theme = &t
	}
	if req.Settings != nil {
		var s establishment.Settings
		if err := ConvertJSON(*req.Settings, &s); err != nil {
			return in, configerr.Invalid("settings", "Impostazioni non valide: controlla numeri decimali, date (AAAA-MM-GG) e ricorrenze (MM-GG).")
		}
		in.Settings = &s
	}
	return in, nil
}

// AddressFromAPI maps an API address.
func AddressFromAPI(a api.Address) store.Address {
	return store.Address{
		Street: estDeref(a.Street), City: estDeref(a.City), Province: estDeref(a.Province),
		PostalCode: estDeref(a.PostalCode), Country: estDeref(a.Country),
	}
}

// ThemeFromAPI maps an API theme.
func ThemeFromAPI(t api.Theme) store.Theme {
	out := store.Theme{
		LogoURL: estDeref(t.LogoUrl), PrimaryColor: estDeref(t.PrimaryColor), AccentColor: estDeref(t.AccentColor),
		ShowPricesOnMap: t.ShowPricesOnMap, HideHeaderInEmbed: t.HideHeaderInEmbed,
	}
	if t.FontFamily != nil {
		out.FontFamily = string(*t.FontFamily)
	}
	if t.Radius != nil {
		out.Radius = string(*t.Radius)
	}
	if t.CustomTexts != nil {
		out.CustomTexts = *t.CustomTexts
	}
	return out
}

func estClientIP(r *http.Request) string { return reqctx.From(r.Context()).ClientIP }

func estDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
