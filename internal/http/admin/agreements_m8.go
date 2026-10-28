package admin

import (
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/money"
)

// M8 fields of agreements (spec 30.2.2, ADR 0028) on the agreement API.

// agreementM8FromAPI converts the M8 fields of an agreement input.
func agreementM8FromAPI(in api.AgreementInput) (pricingconfig.AgreementM8, error) {
	m := pricingconfig.AgreementM8{
		PartnerID: in.PartnerId, CustomerCategoryID: in.CustomerCategoryId, Requirement: pcStrOr(in.Requirement),
		SeasonIDs: pcSliceOf(in.SeasonIds),
	}
	if in.Target != nil {
		m.Target = string(*in.Target)
	}
	if in.RegisterCategory != nil {
		rc := string(*in.RegisterCategory)
		m.RegisterCategory = &rc
	}
	if in.Channels != nil {
		for _, c := range *in.Channels {
			m.Channels = append(m.Channels, string(c))
		}
	}
	if in.PackPrices != nil {
		for _, pp := range *in.PackPrices {
			x := pricing.AgreementPackPrice{PackID: pp.PackId, Amount: money.Cents(pp.Amount)}
			if pp.Per != nil {
				x.Per = pricing.AmountPer(*pp.Per)
			}
			m.PackPrices = append(m.PackPrices, x)
		}
	}
	if sc := in.Scope; sc != nil {
		m.Scope = &pricing.AgreementScope{
			PackIDs: pcSliceOf(sc.PackIds), ExtraIDs: pcSliceOf(sc.ExtraIds), ApplyToExtras: pcBoolOr(sc.ApplyToExtras, false),
		}
	}
	if q := in.Quota; q != nil {
		m.Quota = &pricingconfig.AgreementQuota{
			MaxPerDay: q.MaxPerDay, ReservedPerDay: q.ReservedPerDay, ReleaseDaysBefore: q.ReleaseDaysBefore,
			ZoneIDs: pcSliceOf(q.ZoneIds),
		}
	}
	if m.Target == "" && (m.PartnerID != nil || m.CustomerCategoryID != nil) {
		return m, configerr.Invalid("target", "Indica il tipo di destinatari.")
	}
	return m, nil
}

func agreementQuotaToAPI(q pricingconfig.AgreementQuota) api.AgreementQuota {
	out := api.AgreementQuota{MaxPerDay: q.MaxPerDay, ReservedPerDay: q.ReservedPerDay}
	rd := q.ReleaseDays()
	out.ReleaseDaysBefore = &rd
	if len(q.ZoneIDs) > 0 {
		z := append([]string{}, q.ZoneIDs...)
		out.ZoneIds = &z
	}
	return out
}

// agreementM8Fields maps the M8 fields shared by api.Agreement and
// api.AgreementInput.
func agreementM8Fields(m pricingconfig.AgreementM8) (target *api.AgreementTarget, channels *[]api.AgreementChannel,
	packPrices *[]api.AgreementPackPrice, scope *api.AgreementScope, quota *api.AgreementQuota, seasons *[]string,
	register *api.RegisterCategory,
) {
	if m.Target != "" {
		t := api.AgreementTarget(m.Target)
		target = &t
	}
	if len(m.Channels) > 0 {
		cs := make([]api.AgreementChannel, 0, len(m.Channels))
		for _, c := range m.Channels {
			cs = append(cs, api.AgreementChannel(c))
		}
		channels = &cs
	}
	if len(m.PackPrices) > 0 {
		ps := make([]api.AgreementPackPrice, 0, len(m.PackPrices))
		for _, p := range m.PackPrices {
			x := api.AgreementPackPrice{PackId: p.PackID, Amount: int64(p.Amount)}
			if p.Per != "" {
				per := api.AmountPer(p.Per)
				x.Per = &per
			}
			ps = append(ps, x)
		}
		packPrices = &ps
	}
	if s := m.Scope; s != nil {
		sc := api.AgreementScope{}
		if len(s.PackIDs) > 0 {
			ids := append([]string{}, s.PackIDs...)
			sc.PackIds = &ids
		}
		if len(s.ExtraIDs) > 0 {
			ids := append([]string{}, s.ExtraIDs...)
			sc.ExtraIds = &ids
		}
		ate := s.ApplyToExtras
		sc.ApplyToExtras = &ate
		scope = &sc
	}
	if m.Quota != nil {
		q := agreementQuotaToAPI(*m.Quota)
		quota = &q
	}
	if len(m.SeasonIDs) > 0 {
		ids := append([]string{}, m.SeasonIDs...)
		seasons = &ids
	}
	if m.RegisterCategory != nil {
		rc := api.RegisterCategory(*m.RegisterCategory)
		register = &rc
	}
	return
}

// agreementM8ToAPI adds the M8 fields to an agreement.
func agreementM8ToAPI(out *api.Agreement, a pricingconfig.Agreement) {
	m := a.M8
	out.Target, out.Channels, out.PackPrices, out.Scope, out.Quota, out.SeasonIds, out.RegisterCategory = agreementM8Fields(m)
	out.PartnerId, out.CustomerCategoryId, out.Requirement = m.PartnerID, m.CustomerCategoryID, strPtr(m.Requirement)
	if p := m.Partner; p != nil {
		out.Partner = &api.PartnerRef{Id: p.ID, Name: p.Name, Kind: api.PartnerKind(p.Kind)}
	}
	out.PublicLink = strPtr(m.PublicLink)
}

// AgreementInputToAPI maps a stored agreement version back to its input.
func AgreementInputToAPI(in pricingconfig.AgreementInput) api.AgreementInput {
	cond := pcConditionsToAPI(in.Conditions)
	active, stackable := in.Active, in.Stackable
	out := api.AgreementInput{
		Code: in.Code, Name: in.Name, PartnerName: strPtr(in.PartnerName), Notes: strPtr(in.Notes),
		Action: pcActionToAPI(in.Action), Conditions: &cond, ValidFrom: in.ValidFrom, ValidTo: in.ValidTo,
		Active: &active, Stackable: &stackable,
	}
	m := in.M8
	out.Target, out.Channels, out.PackPrices, out.Scope, out.Quota, out.SeasonIds, out.RegisterCategory = agreementM8Fields(m)
	out.PartnerId, out.CustomerCategoryId, out.Requirement = m.PartnerID, m.CustomerCategoryID, strPtr(m.Requirement)
	return out
}
