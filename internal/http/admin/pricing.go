package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of this file are owned by the pricing builder
// (internal/domain/pricingconfig Service): price rules, fixed prices, promo
// codes, agreements, the panel tools (simulate, matrix, coverage) and the
// season copy. They decode, call the service (which checks permissions) and
// encode.

func (h *Handlers) pricingFail(w http.ResponseWriter, r *http.Request, err error) {
	problem.Error(w, r, h.Log, err)
}

// ---- conversions ------------------------------------------------------------

func pcConditionsFromAPI(c *api.PriceConditions) (pricing.Conditions, error) {
	var out pricing.Conditions
	if c == nil {
		return out, nil
	}
	if err := ConvertJSON(c, &out); err != nil {
		return out, configerr.Invalid("conditions", "Condizioni non valide.")
	}
	return out, nil
}

func pcConditionsToAPI(c pricing.Conditions) api.PriceConditions {
	var out api.PriceConditions
	_ = ConvertJSON(c, &out) // same JSON shape, no decimals
	return out
}

func pcDecimalFromAPI(field string, s *api.Decimal) (*pricing.Decimal, error) {
	if s == nil {
		return nil, nil
	}
	d, err := pricing.ParseDecimal(*s)
	if err != nil {
		return nil, configerr.Invalid(field, "Numero decimale non valido (es. \"1.2\").")
	}
	return &d, nil
}

func pcActionFromAPI(a api.PriceAction) (pricing.Action, error) {
	out := pricing.Action{Type: pricing.ActionType(a.Type)}
	if a.Amount != nil {
		out.Amount = money.Cents(*a.Amount)
	}
	var err error
	if out.Factor, err = pcDecimalFromAPI("action.factor", a.Factor); err != nil {
		return out, err
	}
	if out.Percent, err = pcDecimalFromAPI("action.percent", a.Percent); err != nil {
		return out, err
	}
	if a.Per != nil {
		out.Per = pricing.AmountPer(*a.Per)
	}
	if a.Every != nil {
		out.Every = *a.Every
	}
	if a.Free != nil {
		out.Free = *a.Free
	}
	return out, nil
}

func pcActionToAPI(a pricing.Action) api.PriceAction {
	out := api.PriceAction{Type: api.PriceActionType(a.Type)}
	if a.Amount != 0 {
		v := int64(a.Amount)
		out.Amount = &v
	}
	if a.Factor != nil {
		v := a.Factor.String()
		out.Factor = &v
	}
	if a.Percent != nil {
		v := a.Percent.String()
		out.Percent = &v
	}
	if a.Per != "" {
		v := api.AmountPer(a.Per)
		out.Per = &v
	}
	if a.Every != 0 {
		v := a.Every
		out.Every = &v
	}
	if a.Free != 0 {
		v := a.Free
		out.Free = &v
	}
	return out
}

func pcBoolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func pcIntOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func pcStrOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func pcSliceOf[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

func pcPtrSlice[T any](s []T) *[]T {
	if len(s) == 0 {
		return nil
	}
	return &s
}

func pcRuleInputFromAPI(in api.PriceRuleInput) (pricingconfig.RuleInput, error) {
	cond, err := pcConditionsFromAPI(&in.Conditions)
	if err != nil {
		return pricingconfig.RuleInput{}, err
	}
	act, err := pcActionFromAPI(in.Action)
	if err != nil {
		return pricingconfig.RuleInput{}, err
	}
	return pricingconfig.RuleInput{
		SeasonID: in.SeasonId, Name: in.Name, TargetType: pricing.TargetType(in.TargetType), TargetIDs: pcSliceOf(in.TargetIds),
		Conditions: cond, Action: act, Priority: pcIntOr(in.Priority, 0), Stage: pricing.Stage(in.Stage),
		Stackable: pcBoolOr(in.Stackable, false), Active: pcBoolOr(in.Active, true),
	}, nil
}

// PriceRuleToAPI maps a price rule.
func PriceRuleToAPI(r pricingconfig.PriceRule) api.PriceRule {
	return api.PriceRule{
		Id: r.ID, SeasonId: r.SeasonID, Name: r.Name, TargetType: api.PriceTargetType(r.TargetType),
		TargetIds: pcPtrSlice(r.TargetIDs), Conditions: pcConditionsToAPI(r.Conditions), Action: pcActionToAPI(r.Action),
		Priority: r.Priority, Stage: api.PriceStage(r.Stage), Stackable: r.Stackable, Active: r.Active, Seq: r.Seq,
		Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// FixedPriceToAPI maps a fixed duration price.
func FixedPriceToAPI(fp *store.FixedDurationPrice) api.FixedPrice {
	return api.FixedPrice{
		Id: fp.ID, SeasonId: fp.SeasonID, PackId: fp.PackID, Formula: api.FixedFormula(fp.Formula),
		PriceCategory: fp.PriceCategory, RatePeriodCode: fp.RatePeriodCode, Amount: fp.Amount, Version: fp.Version,
		CreatedAt: fp.CreatedAt.UTC(), UpdatedAt: fp.UpdatedAt.UTC(),
	}
}

func pcPromoInputFromAPI(in api.PromoCodeInput) (pricingconfig.PromoCodeInput, error) {
	cond, err := pcConditionsFromAPI(in.Conditions)
	if err != nil {
		return pricingconfig.PromoCodeInput{}, err
	}
	act, err := pcActionFromAPI(in.Action)
	if err != nil {
		return pricingconfig.PromoCodeInput{}, err
	}
	return pricingconfig.PromoCodeInput{
		Code: in.Code, Description: pcStrOr(in.Description), Action: act, Conditions: cond, ValidFrom: in.ValidFrom,
		ValidTo: in.ValidTo, MaxUses: in.MaxUses, MaxUsesPerCustomer: in.MaxUsesPerCustomer,
		Stackable: pcBoolOr(in.Stackable, false), Active: pcBoolOr(in.Active, true),
	}, nil
}

// PromoCodeToAPI maps a promo code.
func PromoCodeToAPI(pc pricingconfig.PromoCode) api.PromoCode {
	return api.PromoCode{
		Id: pc.ID, Code: pc.Code, Description: pc.Description, Action: pcActionToAPI(pc.Action),
		Conditions: pcConditionsToAPI(pc.Conditions), ValidFrom: pc.ValidFrom, ValidTo: pc.ValidTo, MaxUses: pc.MaxUses,
		MaxUsesPerCustomer: pc.MaxUsesPerCustomer, Uses: pc.Uses, Stackable: pc.Stackable, Active: pc.Active,
		Version: pc.Version, CreatedAt: pc.CreatedAt, UpdatedAt: pc.UpdatedAt,
	}
}

func pcAgreementInputFromAPI(in api.AgreementInput) (pricingconfig.AgreementInput, error) {
	cond, err := pcConditionsFromAPI(in.Conditions)
	if err != nil {
		return pricingconfig.AgreementInput{}, err
	}
	act, err := pcActionFromAPI(in.Action)
	if err != nil {
		return pricingconfig.AgreementInput{}, err
	}
	m8, err := agreementM8FromAPI(in)
	if err != nil {
		return pricingconfig.AgreementInput{}, err
	}
	return pricingconfig.AgreementInput{
		Code: in.Code, Name: in.Name, PartnerName: pcStrOr(in.PartnerName), Notes: pcStrOr(in.Notes), Action: act,
		Conditions: cond, ValidFrom: in.ValidFrom, ValidTo: in.ValidTo, Stackable: pcBoolOr(in.Stackable, false),
		Active: pcBoolOr(in.Active, true), M8: m8,
	}, nil
}

// AgreementToAPI maps an agreement.
func AgreementToAPI(a pricingconfig.Agreement) api.Agreement {
	out := api.Agreement{
		Id: a.ID, Code: a.Code, Name: a.Name, PartnerName: a.PartnerName, Notes: a.Notes, Action: pcActionToAPI(a.Action),
		Conditions: pcConditionsToAPI(a.Conditions), ValidFrom: a.ValidFrom, ValidTo: a.ValidTo, Stackable: a.Stackable,
		Active: a.Active, Version: a.Version, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
	agreementM8ToAPI(&out, a)
	return out
}

// BreakdownToAPI maps an engine breakdown (VAT rates as decimal strings).
func BreakdownToAPI(bd pricing.PriceBreakdown) api.PriceBreakdown {
	out := api.PriceBreakdown{
		EngineVersion: bd.EngineVersion, Currency: bd.Currency, Rounding: int64(bd.Rounding), Total: int64(bd.Total),
		Lines: make([]api.LineBreakdown, 0, len(bd.Lines)), Discounts: make([]api.DiscountBreakdown, 0, len(bd.Discounts)),
		Vat: make([]api.VatBreakdown, 0, len(bd.VAT)),
	}
	extras := func(xs []pricing.ExtraBreakdown) []api.ExtraBreakdown {
		o := make([]api.ExtraBreakdown, 0, len(xs))
		for _, x := range xs {
			o = append(o, api.ExtraBreakdown{Code: x.Code, Qty: x.Qty, Total: int64(x.Total)})
		}
		return o
	}
	for _, l := range bd.Lines {
		al := api.LineBreakdown{
			Pack: l.Pack, Zone: l.Zone, Spot: strPtr(l.Spot), Total: int64(l.Total),
			Days: make([]api.DayBreakdown, 0, len(l.Days)), Extras: extras(l.Extras),
		}
		if l.Fixed != nil {
			al.Fixed = &api.FixedBreakdown{Formula: api.FixedFormula(l.Fixed.Formula), RatePeriod: strPtr(l.Fixed.RatePeriod), Amount: int64(l.Fixed.Amount)}
		}
		for _, d := range l.Days {
			ad := api.DayBreakdown{
				Date: d.Date, Slots: api.Slots(d.Slots), Base: int64(d.Base), Total: int64(d.Total),
				Applied: make([]api.AppliedRule, 0, len(d.Applied)),
			}
			for _, a := range d.Applied {
				ad.Applied = append(ad.Applied, api.AppliedRule{Rule: a.Rule, Stage: api.PriceStage(a.Stage), Effect: a.Effect})
			}
			al.Days = append(al.Days, ad)
		}
		out.Lines = append(out.Lines, al)
	}
	if len(bd.Extras) > 0 {
		x := extras(bd.Extras)
		out.Extras = &x
	}
	for _, d := range bd.Discounts {
		out.Discounts = append(out.Discounts, api.DiscountBreakdown{Source: d.Source, Amount: int64(d.Amount)})
	}
	for _, v := range bd.VAT {
		out.Vat = append(out.Vat, api.VatBreakdown{Rate: v.Rate.String(), Taxable: int64(v.Taxable), Tax: int64(v.Tax)})
	}
	return out
}

// PolicyRulesToAPI maps policy rules (same JSON shape).
func PolicyRulesToAPI(r policy.Rules) api.PolicyRules {
	var out api.PolicyRules
	_ = ConvertJSON(r, &out)
	return out
}

// ---- price rules --------------------------------------------------------------

// ListPriceRules implements GET /establishments/{id}/price-rules.
func (h *Handlers) ListPriceRules(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPriceRulesParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	f := pricingconfig.RuleListFilter{SeasonID: pcStrOr(params.SeasonId)}
	if params.Stage != nil {
		f.Stage = string(*params.Stage)
	}
	if params.TargetType != nil {
		f.TargetType = string(*params.TargetType)
	}
	items, next, err := h.Pricing.ListRules(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	out := api.PriceRuleList{Items: make([]api.PriceRule, 0, len(items)), NextCursor: nextCursor(next)}
	for _, it := range items {
		out.Items = append(out.Items, PriceRuleToAPI(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePriceRule implements POST /establishments/{id}/price-rules.
func (h *Handlers) CreatePriceRule(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreatePriceRuleParams) {
	var req api.PriceRuleInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcRuleInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	rule, err := h.Pricing.CreateRule(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusCreated, rule.Version, PriceRuleToAPI(*rule))
}

// GetPriceRule implements GET /establishments/{id}/price-rules/{ruleId}.
func (h *Handlers) GetPriceRule(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PriceRuleID) {
	rule, err := h.Pricing.GetRule(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, rule.Version, PriceRuleToAPI(*rule))
}

// UpdatePriceRule implements PUT /establishments/{id}/price-rules/{ruleId}.
func (h *Handlers) UpdatePriceRule(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PriceRuleID, params api.UpdatePriceRuleParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	var req api.PriceRuleInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcRuleInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	rule, err := h.Pricing.UpdateRule(r.Context(), principalOf(r), establishmentID, id, expected, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, rule.Version, PriceRuleToAPI(*rule))
}

// DeletePriceRule implements DELETE /establishments/{id}/price-rules/{ruleId}.
func (h *Handlers) DeletePriceRule(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PriceRuleID, params api.DeletePriceRuleParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	if err := h.Pricing.DeleteRule(r.Context(), principalOf(r), establishmentID, id, expected); err != nil {
		h.pricingFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- fixed prices -------------------------------------------------------------

func pcFixedInputFromAPI(in api.FixedPriceInput) pricingconfig.FixedPriceInput {
	return pricingconfig.FixedPriceInput{
		SeasonID: in.SeasonId, PackID: in.PackId, Formula: pricing.Formula(in.Formula), PriceCategory: in.PriceCategory,
		RatePeriodCode: in.RatePeriodCode, Amount: money.Cents(in.Amount),
	}
}

// ListFixedPrices implements GET /establishments/{id}/fixed-prices.
func (h *Handlers) ListFixedPrices(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListFixedPricesParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	f := pricingconfig.FixedPriceListFilter{SeasonID: pcStrOr(params.SeasonId), PackID: pcStrOr(params.PackId)}
	items, next, err := h.Pricing.ListFixedPrices(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	out := api.FixedPriceList{Items: make([]api.FixedPrice, 0, len(items)), NextCursor: nextCursor(next)}
	for i := range items {
		out.Items = append(out.Items, FixedPriceToAPI(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateFixedPrice implements POST /establishments/{id}/fixed-prices.
func (h *Handlers) CreateFixedPrice(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateFixedPriceParams) {
	var req api.FixedPriceInput
	if !decode(w, r, &req) {
		return
	}
	fp, err := h.Pricing.CreateFixedPrice(r.Context(), principalOf(r), establishmentID, pcFixedInputFromAPI(req))
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusCreated, fp.Version, FixedPriceToAPI(fp))
}

// GetFixedPrice implements GET /establishments/{id}/fixed-prices/{fixedPriceId}.
func (h *Handlers) GetFixedPrice(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.FixedPriceID) {
	fp, err := h.Pricing.GetFixedPrice(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, fp.Version, FixedPriceToAPI(fp))
}

// UpdateFixedPrice implements PUT /establishments/{id}/fixed-prices/{fixedPriceId}.
func (h *Handlers) UpdateFixedPrice(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.FixedPriceID, params api.UpdateFixedPriceParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	var req api.FixedPriceInput
	if !decode(w, r, &req) {
		return
	}
	fp, err := h.Pricing.UpdateFixedPrice(r.Context(), principalOf(r), establishmentID, id, expected, pcFixedInputFromAPI(req))
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, fp.Version, FixedPriceToAPI(fp))
}

// DeleteFixedPrice implements DELETE /establishments/{id}/fixed-prices/{fixedPriceId}.
func (h *Handlers) DeleteFixedPrice(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.FixedPriceID, params api.DeleteFixedPriceParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	if err := h.Pricing.DeleteFixedPrice(r.Context(), principalOf(r), establishmentID, id, expected); err != nil {
		h.pricingFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- promo codes --------------------------------------------------------------

// ListPromoCodes implements GET /establishments/{id}/promo-codes.
func (h *Handlers) ListPromoCodes(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPromoCodesParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	items, next, err := h.Pricing.ListPromoCodes(r.Context(), principalOf(r), establishmentID, params.Active, cursor, limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	out := api.PromoCodeList{Items: make([]api.PromoCode, 0, len(items)), NextCursor: nextCursor(next)}
	for _, it := range items {
		out.Items = append(out.Items, PromoCodeToAPI(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePromoCode implements POST /establishments/{id}/promo-codes.
func (h *Handlers) CreatePromoCode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreatePromoCodeParams) {
	var req api.PromoCodeInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcPromoInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	pc, err := h.Pricing.CreatePromoCode(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusCreated, pc.Version, PromoCodeToAPI(*pc))
}

// GetPromoCode implements GET /establishments/{id}/promo-codes/{promoCodeId}.
func (h *Handlers) GetPromoCode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PromoCodeID) {
	pc, err := h.Pricing.GetPromoCode(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, pc.Version, PromoCodeToAPI(*pc))
}

// UpdatePromoCode implements PUT /establishments/{id}/promo-codes/{promoCodeId}.
func (h *Handlers) UpdatePromoCode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PromoCodeID, params api.UpdatePromoCodeParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	var req api.PromoCodeInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcPromoInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	pc, err := h.Pricing.UpdatePromoCode(r.Context(), principalOf(r), establishmentID, id, expected, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, pc.Version, PromoCodeToAPI(*pc))
}

// DeletePromoCode implements DELETE /establishments/{id}/promo-codes/{promoCodeId}.
func (h *Handlers) DeletePromoCode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PromoCodeID, params api.DeletePromoCodeParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	if err := h.Pricing.DeletePromoCode(r.Context(), principalOf(r), establishmentID, id, expected); err != nil {
		h.pricingFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- agreements ---------------------------------------------------------------

// ListAgreements implements GET /establishments/{id}/agreements.
func (h *Handlers) ListAgreements(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListAgreementsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	items, next, err := h.Pricing.ListAgreements(r.Context(), principalOf(r), establishmentID, params.Active, cursor, limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	out := api.AgreementList{Items: make([]api.Agreement, 0, len(items)), NextCursor: nextCursor(next)}
	for _, it := range items {
		out.Items = append(out.Items, AgreementToAPI(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateAgreement implements POST /establishments/{id}/agreements.
func (h *Handlers) CreateAgreement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateAgreementParams) {
	var req api.AgreementInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcAgreementInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	a, err := h.Pricing.CreateAgreement(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusCreated, a.Version, AgreementToAPI(*a))
}

// GetAgreement implements GET /establishments/{id}/agreements/{agreementId}.
func (h *Handlers) GetAgreement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.AgreementID) {
	a, err := h.Pricing.GetAgreement(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, a.Version, AgreementToAPI(*a))
}

// UpdateAgreement implements PUT /establishments/{id}/agreements/{agreementId}.
func (h *Handlers) UpdateAgreement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.AgreementID, params api.UpdateAgreementParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	var req api.AgreementInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcAgreementInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	a, err := h.Pricing.UpdateAgreement(r.Context(), principalOf(r), establishmentID, id, expected, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, a.Version, AgreementToAPI(*a))
}

// DeleteAgreement implements DELETE /establishments/{id}/agreements/{agreementId}.
func (h *Handlers) DeleteAgreement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.AgreementID, params api.DeleteAgreementParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	if err := h.Pricing.DeleteAgreement(r.Context(), principalOf(r), establishmentID, id, expected); err != nil {
		h.pricingFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- tools --------------------------------------------------------------------

func pcExtrasFromAPI(xs *[]api.SimulateExtra) []pricing.ExtraInput {
	if xs == nil {
		return nil
	}
	out := make([]pricing.ExtraInput, 0, len(*xs))
	for _, x := range *xs {
		out = append(out, pricing.ExtraInput{ExtraID: x.ExtraId, Quantity: x.Quantity})
	}
	return out
}

// SimulatePrice implements POST /establishments/{id}/pricing/simulate.
func (h *Handlers) SimulatePrice(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.SimulatePriceParams) {
	var req api.SimulateRequest
	if !decode(w, r, &req) {
		return
	}
	in := pricingconfig.SimulateInput{
		SeasonID: pcStrOr(req.SeasonId), Channel: pricing.Channel(req.Channel), StartsOn: req.StartsOn, EndsOn: req.EndsOn,
		Slots: calendar.Slots(req.Slots), Formula: pricing.Formula(req.Formula), Today: req.Today,
		Extras: pcExtrasFromAPI(req.Extras), CustomerTags: pcSliceOf(req.CustomerTags), AgreementID: pcStrOr(req.AgreementId),
		PromoCode: pcStrOr(req.PromoCode),
	}
	for _, l := range req.Lines {
		in.Lines = append(in.Lines, pricing.LineInput{
			PackID: l.PackId, ZoneID: pcStrOr(l.ZoneId), SpotID: pcStrOr(l.SpotId), Extras: pcExtrasFromAPI(l.Extras),
		})
	}
	res, err := h.Pricing.Simulate(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	ev := api.PolicyEvaluation{Rules: PolicyRulesToAPI(res.Policy.Rules), Violations: make([]api.PolicyViolation, 0, len(res.Violations))}
	if res.Policy.PolicyID != "" {
		id := res.Policy.PolicyID
		ev.PolicyId = &id
	}
	for _, v := range res.Violations {
		pv := api.PolicyViolation{Code: string(v.Code)}
		if len(v.Params) > 0 {
			params := v.Params
			pv.Params = &params
		}
		ev.Violations = append(ev.Violations, pv)
	}
	writeJSON(w, http.StatusOK, api.SimulateResult{
		Breakdown: BreakdownToAPI(res.Breakdown), DepositDue: int64(res.DepositDue), Policy: ev,
	})
}

// GetPriceMatrix implements GET /establishments/{id}/pricing/matrix.
func (h *Handlers) GetPriceMatrix(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetPriceMatrixParams) {
	in := pricingconfig.MatrixInput{SeasonID: params.SeasonId, PackIDs: pcSliceOf(params.PackIds)}
	if params.Channel != nil {
		in.Channel = pricing.Channel(*params.Channel)
	}
	if params.Slots != nil {
		in.Slots = calendar.Slots(*params.Slots)
	}
	if params.Formula != nil {
		in.Formula = pricing.Formula(*params.Formula)
	}
	res, err := h.Pricing.Matrix(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	out := api.PriceMatrix{
		SeasonId: res.SeasonID, Channel: api.Channel(res.Channel), Slots: api.Slots(res.Slots), Formula: api.Formula(res.Formula),
		PackIds: res.PackIDs, Categories: res.Categories, RatePeriods: res.RatePeriods,
		Cells: make([]api.PriceMatrixCell, 0, len(res.Cells)),
	}
	for _, c := range res.Cells {
		out.Cells = append(out.Cells, api.PriceMatrixCell{
			PackId: c.PackID, PriceCategory: c.PriceCategory, RatePeriod: c.RatePeriod, ZoneId: c.ZoneID, Date: c.Date,
			Priced: c.Priced, Price: int64(c.Price),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GetPriceCoverage implements GET /establishments/{id}/pricing/coverage.
func (h *Handlers) GetPriceCoverage(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetPriceCoverageParams) {
	in := pricingconfig.CoverageInput{SeasonID: params.SeasonId, PackIDs: pcSliceOf(params.PackIds)}
	for _, ch := range pcSliceOf(params.Channels) {
		in.Channels = append(in.Channels, pricing.Channel(ch))
	}
	if params.Slots != nil {
		in.Slots = calendar.Slots(*params.Slots)
	}
	res, err := h.Pricing.Coverage(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	out := api.CoverageReport{
		SeasonId: res.SeasonID, From: res.From, To: res.To, Complete: res.Complete(),
		Gaps: make([]api.CoverageGap, 0, len(res.Gaps)),
	}
	for _, g := range res.Gaps {
		out.Gaps = append(out.Gaps, api.CoverageGap{PackId: g.PackID, PriceCategory: g.PriceCategory, Channel: api.Channel(g.Channel), From: g.From, To: g.To})
	}
	writeJSON(w, http.StatusOK, out)
}

// CopySeason implements POST /establishments/{id}/seasons/{seasonId}/copy.
func (h *Handlers) CopySeason(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, seasonID api.SeasonID, _ api.CopySeasonParams) {
	var req api.SeasonCopyRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.Pricing.CopySeason(r.Context(), principalOf(r), establishmentID, seasonID, pricingconfig.CopySeasonInput{
		TargetYear: req.TargetYear, OpensOn: req.OpensOn, ClosesOn: req.ClosesOn,
		IncludeRatePeriods: pcBoolOr(req.IncludeRatePeriods, true), IncludePriceRules: pcBoolOr(req.IncludePriceRules, true),
		IncludeFixedPrices: pcBoolOr(req.IncludeFixedPrices, true), IncludePolicies: pcBoolOr(req.IncludePolicies, true),
	})
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	sn := res.Season
	writeJSON(w, http.StatusCreated, api.SeasonCopyResult{
		Season: api.Season{
			Id: sn.ID, EstablishmentId: sn.EstablishmentID, Year: sn.Year, OpensOn: sn.OpensOn, ClosesOn: sn.ClosesOn,
			Status: api.SeasonStatus(sn.Status), Version: sn.Version, CreatedAt: sn.CreatedAt.UTC(), UpdatedAt: sn.UpdatedAt.UTC(),
		},
		RatePeriods: res.RatePeriods, PriceRules: res.PriceRules, FixedPrices: res.FixedPrices, Policies: res.Policies,
		Warnings: res.Warnings,
	})
}
