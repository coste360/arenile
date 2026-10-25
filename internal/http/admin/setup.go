package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/setup"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the guided setup (spec 15.2), owned by the setup builder
// (internal/domain/setup). Permission checks live in setup.Service.

// ListSetupTemplates implements GET /establishments/{establishmentId}/setup/templates.
func (h *Handlers) ListSetupTemplates(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	ts, err := h.Setup.ListTemplates(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.SetupTemplateList{Items: make([]api.SetupTemplate, 0, len(ts))}
	for _, t := range ts {
		out.Items = append(out.Items, SetupTemplateToAPI(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetSetupStatus implements GET /establishments/{establishmentId}/setup/status.
func (h *Handlers) GetSetupStatus(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	st, err := h.Setup.Status(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.SetupStatus{
		HasSeason: st.HasSeason, HasRatePeriods: st.HasRatePeriods, HasMap: st.HasMap, SpotCount: st.SpotCount,
		HasCatalog: st.HasCatalog, HasPrices: st.HasPrices, BookingEnabled: st.BookingEnabled,
		PriceCategories: append([]string{}, st.PriceCategories...), HasOperations: st.HasOperations,
	})
}

// ResetEstablishmentSetup implements POST /establishments/{establishmentId}/setup/reset.
func (h *Handlers) ResetEstablishmentSetup(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.ResetEstablishmentSetupParams) {
	var req api.SetupResetRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.Setup.Reset(r.Context(), principalOf(r), establishmentID, setup.ResetInput{
		Scope: setup.ResetScope(req.Scope), ConfirmName: req.ConfirmName, IP: estClientIP(r),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.SetupResetResult{Scope: api.SetupResetResultScope(res.Scope), Deleted: res.Deleted})
}

// ApplySetupTemplate implements POST /establishments/{establishmentId}/setup/apply-template.
// The API has no "force" flag: the template is refused (409) when the
// establishment already has maps or zones, except "empty".
func (h *Handlers) ApplySetupTemplate(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.ApplySetupTemplateParams) {
	var req api.ApplyTemplateRequest
	if !decode(w, r, &req) {
		return
	}
	in := setup.ApplyInput{
		Template: setup.Code(req.Template), SeasonYear: req.SeasonYear, OpensOn: req.OpensOn, ClosesOn: req.ClosesOn,
		Rows: req.Rows, SpotsPerRow: req.SpotsPerRow, ClientIP: reqctx.From(r.Context()).ClientIP,
	}
	if req.MapName != nil {
		in.MapName = store.I18nText(*req.MapName)
	}
	res, err := h.Setup.Apply(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.ApplyTemplateResult{
		Template: api.SetupTemplateCode(res.Template), ZoneIds: nonNil(res.ZoneIDs), PoolIds: nonNil(res.PoolIDs),
		ComponentIds: nonNil(res.ComponentIDs), PackIds: nonNil(res.PackIDs), ExtraIds: nonNil(res.ExtraIDs),
		RatePeriodIds: nonNil(res.RatePeriodIDs), SpotsCreated: res.SpotsCreated,
	}
	if res.SeasonID != "" {
		out.SeasonId = &res.SeasonID
	}
	if res.MapID != "" {
		out.MapId = &res.MapID
	}
	writeJSON(w, http.StatusCreated, out)
}

// CreateQuickPricelist implements POST /establishments/{establishmentId}/setup/quick-pricelist.
func (h *Handlers) CreateQuickPricelist(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateQuickPricelistParams) {
	var req api.QuickPricelistRequest
	if !decode(w, r, &req) {
		return
	}
	in := setup.QuickPricelistInput{SeasonID: req.SeasonId, ClientIP: reqctx.From(r.Context()).ClientIP}
	if req.PackIds != nil {
		in.PackIDs = *req.PackIds
	}
	if req.ReplaceExisting != nil {
		in.ReplaceExisting = *req.ReplaceExisting
	}
	in.Prices = make([]setup.QuickPrice, len(req.Prices))
	for i, p := range req.Prices {
		in.Prices[i] = setup.QuickPrice{PriceCategory: p.PriceCategory, RatePeriodCode: p.RatePeriodCode, Amount: money.Cents(p.Amount)}
	}
	res, err := h.Setup.QuickPricelist(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.QuickPricelistResult{Replaced: res.Replaced, Rules: make([]api.PriceRule, 0, len(res.Rules))}
	for i := range res.Rules {
		pr, err := quickRuleToAPI(&res.Rules[i])
		if err != nil {
			problem.Error(w, r, h.Log, err)
			return
		}
		out.Rules = append(out.Rules, pr)
	}
	writeJSON(w, http.StatusCreated, out)
}

// SetupTemplateToAPI maps a setup template.
func SetupTemplateToAPI(t setup.Template) api.SetupTemplate {
	out := api.SetupTemplate{
		Code: api.SetupTemplateCode(t.Code), Name: api.I18nText(t.Name), Description: api.I18nText(t.Description),
		Rows: t.Rows, SpotsPerRow: t.SpotsPerRow, Zones: make([]api.SetupTemplateZone, 0, len(t.Zones)),
		Packs: make([]api.SetupTemplatePack, 0, len(t.Packs)), RatePeriods: t.RatePeriodCodes(),
	}
	for _, z := range t.Zones {
		out.Zones = append(out.Zones, api.SetupTemplateZone{
			Code: z.Code, Name: api.I18nText(z.Name), AssignmentMode: api.AssignmentMode(z.AssignmentMode), PriceCategory: z.PriceCategory,
		})
	}
	for _, p := range t.Packs {
		out.Packs = append(out.Packs, api.SetupTemplatePack{Code: p.Code, Name: api.I18nText(p.Name), Composition: api.ComponentQuantities(p.Composition)})
	}
	return out
}

// quickRuleToAPI maps a rule generated by the quick price list. Its
// conditions and action hold only codes, slots and an amount, so the JSON
// shapes of the engine and of the API coincide (no decimals involved).
func quickRuleToAPI(r *store.PriceRule) (api.PriceRule, error) {
	out := api.PriceRule{
		Id: r.ID, SeasonId: r.SeasonID, Name: r.Name, TargetType: api.PriceTargetType(r.TargetType),
		Priority: r.Priority, Stage: api.PriceStage(r.Stage), Stackable: r.Stackable, Active: r.Active,
		Seq: r.Seq, Version: r.Version, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.TargetIDs != nil {
		ids := append([]string(nil), r.TargetIDs...)
		out.TargetIds = &ids
	}
	if err := r.Conditions.Unmarshal(&out.Conditions); err != nil {
		return out, err
	}
	if err := r.Action.Unmarshal(&out.Action); err != nil {
		return out, err
	}
	return out, nil
}
