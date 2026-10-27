package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/catalog"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the catalog area (spec 6.3): components, packs and extras.
// Owned by the catalog builder (internal/domain/catalog). Deleting a pack or
// an extra that price rules (or, for packs, fixed prices, extras and
// policies) reference archives it (active = false) and still answers 204.

// ---- components -------------------------------------------------------------

// ListComponents implements GET /establishments/{establishmentId}/components.
func (h *Handlers) ListComponents(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListComponentsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Catalog.ListComponents(r.Context(), principalOf(r), establishmentID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.ComponentList{Items: make([]api.Component, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, ComponentToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateComponent implements POST /establishments/{establishmentId}/components.
func (h *Handlers) CreateComponent(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateComponentParams) {
	var req api.ComponentInput
	if !decode(w, r, &req) {
		return
	}
	c, err := h.Catalog.CreateComponent(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, componentInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, c.Version, ComponentToAPI(c))
}

// DeleteComponent implements DELETE /establishments/{establishmentId}/components/{componentId}.
func (h *Handlers) DeleteComponent(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ComponentID, params api.DeleteComponentParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Catalog.DeleteComponent(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, id, ifMatch); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetComponent implements GET /establishments/{establishmentId}/components/{componentId}.
func (h *Handlers) GetComponent(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ComponentID) {
	c, err := h.Catalog.GetComponent(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, c.Version, ComponentToAPI(c))
}

// UpdateComponent implements PUT /establishments/{establishmentId}/components/{componentId}.
func (h *Handlers) UpdateComponent(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ComponentID, params api.UpdateComponentParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.ComponentInput
	if !decode(w, r, &req) {
		return
	}
	c, err := h.Catalog.UpdateComponent(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, id, ifMatch, componentInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, c.Version, ComponentToAPI(c))
}

// ---- extras -----------------------------------------------------------------

// ListExtras implements GET /establishments/{establishmentId}/extras.
func (h *Handlers) ListExtras(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListExtrasParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Catalog.ListExtras(r.Context(), principalOf(r), establishmentID, params.Active, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.ExtraList{Items: make([]api.Extra, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, ExtraToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateExtra implements POST /establishments/{establishmentId}/extras.
func (h *Handlers) CreateExtra(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateExtraParams) {
	var req api.ExtraInput
	if !decode(w, r, &req) {
		return
	}
	x, err := h.Catalog.CreateExtra(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, extraInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, x.Version, ExtraToAPI(x))
}

// DeleteExtra implements DELETE /establishments/{establishmentId}/extras/{extraId}.
func (h *Handlers) DeleteExtra(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ExtraID, params api.DeleteExtraParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if _, err := h.Catalog.DeleteExtra(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, id, ifMatch); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetExtra implements GET /establishments/{establishmentId}/extras/{extraId}.
func (h *Handlers) GetExtra(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ExtraID) {
	x, err := h.Catalog.GetExtra(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, x.Version, ExtraToAPI(x))
}

// UpdateExtra implements PUT /establishments/{establishmentId}/extras/{extraId}.
func (h *Handlers) UpdateExtra(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ExtraID, params api.UpdateExtraParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.ExtraInput
	if !decode(w, r, &req) {
		return
	}
	x, err := h.Catalog.UpdateExtra(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, id, ifMatch, extraInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, x.Version, ExtraToAPI(x))
}

// ---- packs ------------------------------------------------------------------

// ListPacks implements GET /establishments/{establishmentId}/packs.
func (h *Handlers) ListPacks(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPacksParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Catalog.ListPacks(r.Context(), principalOf(r), establishmentID, params.Active, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PackList{Items: make([]api.Pack, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, PackToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePack implements POST /establishments/{establishmentId}/packs.
func (h *Handlers) CreatePack(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreatePackParams) {
	var req api.PackInput
	if !decode(w, r, &req) {
		return
	}
	pk, err := h.Catalog.CreatePack(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, packInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, pk.Version, PackToAPI(pk))
}

// DeletePack implements DELETE /establishments/{establishmentId}/packs/{packId}.
func (h *Handlers) DeletePack(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PackID, params api.DeletePackParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if _, err := h.Catalog.DeletePack(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, id, ifMatch); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPack implements GET /establishments/{establishmentId}/packs/{packId}.
func (h *Handlers) GetPack(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PackID) {
	pk, err := h.Catalog.GetPack(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, pk.Version, PackToAPI(pk))
}

// UpdatePack implements PUT /establishments/{establishmentId}/packs/{packId}.
func (h *Handlers) UpdatePack(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PackID, params api.UpdatePackParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.PackInput
	if !decode(w, r, &req) {
		return
	}
	pk, err := h.Catalog.UpdatePack(r.Context(), principalOf(r), catalogClientIP(r), establishmentID, id, ifMatch, packInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, pk.Version, PackToAPI(pk))
}

// ---- mapping ----------------------------------------------------------------

// clientIP is the caller's IP for the audit log.
func catalogClientIP(r *http.Request) string { return reqctx.From(r.Context()).ClientIP }

func catalogI18nFromAPI(t *api.I18nText) store.I18nText {
	if t == nil {
		return nil
	}
	return store.I18nText(*t)
}

func catalogI18nToAPI(t store.I18nText) api.I18nText {
	if t == nil {
		return api.I18nText{}
	}
	return api.I18nText(t)
}

func catalogOptList(v []string) *[]string {
	if len(v) == 0 {
		return nil
	}
	out := append([]string(nil), v...)
	return &out
}

func catalogDerefList(v *[]string) []string {
	if v == nil {
		return nil
	}
	return *v
}

func componentInputFromAPI(in api.ComponentInput) catalog.ComponentInput {
	return catalog.ComponentInput{Code: in.Code, Name: store.I18nText(in.Name), Tracked: in.Tracked, PoolID: in.PoolId}
}

// ComponentToAPI maps a component to its API representation.
func ComponentToAPI(c *store.Component) api.Component {
	return api.Component{
		Id: c.ID, Code: c.Code, Name: catalogI18nToAPI(c.Name), Tracked: c.Tracked, PoolId: c.PoolID,
		Version: c.Version, CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC(),
	}
}

func packInputFromAPI(in api.PackInput) catalog.PackInput {
	out := catalog.PackInput{
		Code: in.Code, Name: store.I18nText(in.Name), Description: catalogI18nFromAPI(in.Description), Image: in.Image,
		Composition: map[string]int(in.Composition), AllowedZoneIDs: catalogDerefList(in.AllowedZoneIds),
		MaxPerBooking: in.MaxPerBooking, Active: in.Active, VATRateBP: in.VatRateBp,
	}
	if in.RequiresSpotKind != nil {
		for _, k := range *in.RequiresSpotKind {
			out.RequiresSpotKind = append(out.RequiresSpotKind, string(k))
		}
	}
	for _, f := range in.AllowedFormulas {
		out.AllowedFormulas = append(out.AllowedFormulas, string(f))
	}
	for _, c := range in.Channels {
		out.Channels = append(out.Channels, string(c))
	}
	if in.RequiresApproval != nil {
		out.RequiresApproval = *in.RequiresApproval
	}
	if in.SortOrder != nil {
		out.SortOrder = *in.SortOrder
	}
	return out
}

// PackToAPI maps a pack to its API representation.
func PackToAPI(p *store.Pack) api.Pack {
	out := api.Pack{
		Id: p.ID, Code: p.Code, Name: catalogI18nToAPI(p.Name), Description: catalogI18nToAPI(p.Description), Image: p.Image,
		Composition: api.ComponentQuantities(p.Composition), AllowedZoneIds: catalogOptList(p.AllowedZoneIDs),
		AllowedFormulas: make([]api.Formula, 0, len(p.AllowedFormulas)), Channels: make([]api.Channel, 0, len(p.Channels)),
		MaxPerBooking: p.MaxPerBooking, RequiresApproval: p.RequiresApproval, SortOrder: p.SortOrder, Active: p.Active,
		VatRateBp: p.VATRateBP, Version: p.Version, CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC(),
	}
	if out.Composition == nil {
		out.Composition = api.ComponentQuantities{}
	}
	if len(p.RequiresSpotKind) > 0 {
		kinds := make([]api.SpotKind, 0, len(p.RequiresSpotKind))
		for _, k := range p.RequiresSpotKind {
			kinds = append(kinds, api.SpotKind(k))
		}
		out.RequiresSpotKind = &kinds
	}
	for _, f := range p.AllowedFormulas {
		out.AllowedFormulas = append(out.AllowedFormulas, api.Formula(f))
	}
	for _, c := range p.Channels {
		out.Channels = append(out.Channels, api.Channel(c))
	}
	return out
}

func extraInputFromAPI(in api.ExtraInput) catalog.ExtraInput {
	out := catalog.ExtraInput{
		Code: in.Code, Name: store.I18nText(in.Name), Description: catalogI18nFromAPI(in.Description),
		ComponentCode: in.ComponentCode, PricingUnit: string(in.PricingUnit), MaxQuantity: in.MaxQuantity,
		AttachTo: string(in.AttachTo), AllowedPackIDs: catalogDerefList(in.AllowedPackIds), Active: in.Active, VATRateBP: in.VatRateBp,
	}
	for _, c := range in.Channels {
		out.Channels = append(out.Channels, string(c))
	}
	if in.SortOrder != nil {
		out.SortOrder = *in.SortOrder
	}
	out.ShowOnMap, out.MapLabel = in.ShowOnMap, in.MapLabel
	return out
}

// ExtraToAPI maps an extra to its API representation.
func ExtraToAPI(x *store.Extra) api.Extra {
	out := api.Extra{
		Id: x.ID, Code: x.Code, Name: catalogI18nToAPI(x.Name), Description: catalogI18nToAPI(x.Description),
		ComponentCode: x.ComponentCode, PricingUnit: api.PricingUnit(x.PricingUnit), MaxQuantity: x.MaxQuantity,
		AttachTo: api.ExtraAttachTo(x.AttachTo), AllowedPackIds: catalogOptList(x.AllowedPackIDs),
		Channels: make([]api.Channel, 0, len(x.Channels)), SortOrder: x.SortOrder, Active: x.Active,
		VatRateBp: x.VATRateBP, Version: x.Version, CreatedAt: x.CreatedAt.UTC(), UpdatedAt: x.UpdatedAt.UTC(),
	}
	for _, c := range x.Channels {
		out.Channels = append(out.Channels, api.Channel(c))
	}
	out.ShowOnMap = &x.ShowOnMap
	if x.MapLabel != "" {
		out.MapLabel = &x.MapLabel
	}
	return out
}
