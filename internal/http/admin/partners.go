package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Partners, customer categories, requirement verifications and agreement
// extensions (spec 30.2, ADR 0028).
//
//	ListPartners / CreatePartner / GetPartner / UpdatePartner / DeletePartner -> Partners
//	*CustomerCategor*                                                  -> Partners
//	ListCustomerRequirementVerifications / VerifyCustomerRequirement    -> Partners
//	VerifyBookingRequirement -> Bookings.VerifyRequirement (booking state changes stay in booking)
//	ListAgreementVersions / GetAgreementQuotaUsage -> Pricing (pricingconfig)

// partnersReady answers 501 when the M8 partner service is not wired.
func (h *Handlers) partnersReady(w http.ResponseWriter, r *http.Request) bool {
	if h.Partners == nil {
		notImplemented(w, r)
		return false
	}
	return true
}

// ---- partners -----------------------------------------------------------------

// ListPartners implements GET /partners.
func (h *Handlers) ListPartners(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPartnersParams) {
	if !h.partnersReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := partner.Filter{Q: customerStr(params.Q), Active: params.Active}
	if params.Kind != nil {
		f.Kind = string(*params.Kind)
	}
	items, next, err := h.Partners.List(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PartnerList{Items: make([]api.Partner, 0, len(items)), NextCursor: nextCursor(next)}
	for i := range items {
		out.Items = append(out.Items, PartnerToAPI(&items[i]))
	}
	customerNoStore(w)
	writeJSON(w, http.StatusOK, out)
}

// CreatePartner implements POST /partners.
func (h *Handlers) CreatePartner(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreatePartnerParams) {
	if !h.partnersReady(w, r) {
		return
	}
	var req api.PartnerInput
	if !decode(w, r, &req) {
		return
	}
	v, err := h.Partners.Create(r.Context(), principalOf(r), establishmentID, partnerInputFromAPI(&req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusCreated, v.Version, PartnerToAPI(v))
}

// GetPartner implements GET /partners/{partnerId}.
func (h *Handlers) GetPartner(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PartnerID) {
	if !h.partnersReady(w, r) {
		return
	}
	v, err := h.Partners.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, v.Version, PartnerToAPI(v))
}

// UpdatePartner implements PUT /partners/{partnerId}.
func (h *Handlers) UpdatePartner(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PartnerID, params api.UpdatePartnerParams) {
	if !h.partnersReady(w, r) {
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.PartnerInput
	if !decode(w, r, &req) {
		return
	}
	v, err := h.Partners.Update(r.Context(), principalOf(r), establishmentID, id, expected, partnerInputFromAPI(&req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, v.Version, PartnerToAPI(v))
}

// DeletePartner implements DELETE /partners/{partnerId}.
func (h *Handlers) DeletePartner(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PartnerID, params api.DeletePartnerParams) {
	if !h.partnersReady(w, r) {
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Partners.Delete(r.Context(), principalOf(r), establishmentID, id, expected); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- customer categories ------------------------------------------------------------

// ListCustomerCategories implements GET /customer-categories.
func (h *Handlers) ListCustomerCategories(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListCustomerCategoriesParams) {
	if !h.partnersReady(w, r) {
		return
	}
	items, err := h.Partners.ListCategories(r.Context(), principalOf(r), establishmentID, params.Active)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.CustomerCategoryList{Items: make([]api.CustomerCategory, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, CustomerCategoryToAPI(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateCustomerCategory implements POST /customer-categories.
func (h *Handlers) CreateCustomerCategory(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateCustomerCategoryParams) {
	if !h.partnersReady(w, r) {
		return
	}
	var req api.CustomerCategoryInput
	if !decode(w, r, &req) {
		return
	}
	v, err := h.Partners.CreateCategory(r.Context(), principalOf(r), establishmentID, categoryInputFromAPI(&req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, v.Version, CustomerCategoryToAPI(v))
}

// GetCustomerCategory implements GET /customer-categories/{customerCategoryId}.
func (h *Handlers) GetCustomerCategory(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerCategoryID) {
	if !h.partnersReady(w, r) {
		return
	}
	v, err := h.Partners.GetCategory(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Version, CustomerCategoryToAPI(v))
}

// UpdateCustomerCategory implements PUT /customer-categories/{customerCategoryId}.
func (h *Handlers) UpdateCustomerCategory(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerCategoryID, params api.UpdateCustomerCategoryParams) {
	if !h.partnersReady(w, r) {
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.CustomerCategoryInput
	if !decode(w, r, &req) {
		return
	}
	v, err := h.Partners.UpdateCategory(r.Context(), principalOf(r), establishmentID, id, expected, categoryInputFromAPI(&req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Version, CustomerCategoryToAPI(v))
}

// DeleteCustomerCategory implements DELETE /customer-categories/{customerCategoryId}.
func (h *Handlers) DeleteCustomerCategory(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerCategoryID, params api.DeleteCustomerCategoryParams) {
	if !h.partnersReady(w, r) {
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Partners.DeleteCategory(r.Context(), principalOf(r), establishmentID, id, expected); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- requirement verifications -----------------------------------------------------

// ListCustomerRequirementVerifications implements GET /customers/{customerId}/requirement-verifications.
func (h *Handlers) ListCustomerRequirementVerifications(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerID) {
	if !h.partnersReady(w, r) {
		return
	}
	rows, names, err := h.Partners.ListCustomerVerifications(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RequirementVerificationList{Items: make([]api.RequirementVerification, 0, len(rows))}
	for i := range rows {
		out.Items = append(out.Items, RequirementVerificationToAPI(&rows[i], names))
	}
	customerNoStore(w)
	writeJSON(w, http.StatusOK, out)
}

// VerifyCustomerRequirement implements POST /customers/{customerId}/requirement-verifications.
func (h *Handlers) VerifyCustomerRequirement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerID, _ api.VerifyCustomerRequirementParams) {
	if !h.partnersReady(w, r) {
		return
	}
	var req api.RequirementVerificationInput
	if !decode(w, r, &req) {
		return
	}
	row, names, err := h.Partners.VerifyCustomer(r.Context(), principalOf(r), establishmentID, id, partner.VerifyInput{
		AgreementID: req.AgreementId, CustomerCategoryID: req.CustomerCategoryId, Note: customerStr(req.Note),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeJSON(w, http.StatusCreated, RequirementVerificationToAPI(row, names))
}

// VerifyBookingRequirement implements POST /bookings/{bookingId}/requirement-verification.
func (h *Handlers) VerifyBookingRequirement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.BookingID, params api.VerifyBookingRequirementParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.RequirementVerificationInput
	if !decode(w, r, &req) {
		return
	}
	d, err := h.Bookings.VerifyRequirement(r.Context(), principalOf(r), establishmentID, id, booking.VerifyInput{
		ExpectedVersion: expected, AgreementID: req.AgreementId, CustomerCategoryID: req.CustomerCategoryId,
		Note: customerStr(req.Note),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Booking.Version, BookingDetailToAPI(d))
}

// ---- agreement versions and quota ----------------------------------------------------

// ListAgreementVersions implements GET /agreements/{agreementId}/versions.
func (h *Handlers) ListAgreementVersions(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.AgreementID) {
	items, err := h.Pricing.ListAgreementVersions(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.AgreementVersionList{Items: make([]api.AgreementVersion, 0, len(items))}
	for _, v := range items {
		av := api.AgreementVersion{
			Version: v.Version, ChangedAt: v.ChangedAt, ChangedByUserId: v.ChangedByUserID,
			Agreement: AgreementInputToAPI(v.Input),
		}
		if v.ChangedByName != "" {
			n := v.ChangedByName
			av.ChangedByName = &n
		}
		out.Items = append(out.Items, av)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetAgreementQuotaUsage implements GET /agreements/{agreementId}/quota.
func (h *Handlers) GetAgreementQuotaUsage(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.AgreementID, params api.GetAgreementQuotaUsageParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	u, err := h.Pricing.AgreementQuotaUsage(r.Context(), principalOf(r), establishmentID, id, p.From, p.To)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.AgreementQuotaUsage{AgreementId: id, Days: make([]api.AgreementQuotaDay, 0, len(u.Days))}
	if u.Quota != nil {
		q := agreementQuotaToAPI(*u.Quota)
		out.Quota = &q
	}
	for _, d := range u.Days {
		day := api.AgreementQuotaDay{
			Date: d.Date, Slot: api.Slot(d.Slot), Reserved: d.Reserved, Used: d.Used, Max: d.Max, Released: d.Released,
		}
		if !d.ReleaseOn.IsZero() {
			ro := d.ReleaseOn
			day.ReleaseOn = &ro
		}
		out.Days = append(out.Days, day)
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- mapping ----------------------------------------------------------------------

func partnerInputFromAPI(in *api.PartnerInput) partner.Input {
	out := partner.Input{
		Name: in.Name, Kind: string(in.Kind), Municipality: customerStr(in.Municipality), Address: customerStr(in.Address),
		ContactName: customerStr(in.ContactName), Phone: customerStr(in.Phone), VATNumber: customerStr(in.VatNumber),
		TaxCode: customerStr(in.TaxCode), Notes: customerStr(in.Notes), Active: pcBoolOr(in.Active, true),
	}
	if in.AccommodationType != nil {
		t := string(*in.AccommodationType)
		out.AccommodationType = &t
	}
	if in.Email != nil {
		out.Email = string(*in.Email)
	}
	return out
}

// PartnerToAPI maps a partner view.
func PartnerToAPI(v *partner.View) api.Partner {
	p := &v.Partner
	out := api.Partner{
		Id: p.ID, Name: p.Name, Kind: api.PartnerKind(p.Kind), Municipality: strPtr(p.Municipality),
		Address: strPtr(p.Address), ContactName: strPtr(p.ContactName), Email: p.Email, Phone: p.Phone,
		VatNumber: strPtr(p.VATNumber), TaxCode: strPtr(p.TaxCode), Notes: p.Notes, Active: p.Active,
		Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	if p.AccommodationType != nil {
		t := api.AccommodationType(*p.AccommodationType)
		out.AccommodationType = &t
	}
	n := v.AgreementCount
	out.AgreementCount = &n
	if v.Balance != nil {
		b := int64(*v.Balance)
		out.Balance = &b
	}
	return out
}

// PartnerRefToAPI maps a short partner reference (nil when absent).
func PartnerRefToAPI(r *partner.Ref) *api.PartnerRef {
	if r == nil {
		return nil
	}
	return &api.PartnerRef{Id: r.ID, Name: r.Name, Kind: api.PartnerKind(r.Kind)}
}

func categoryInputFromAPI(in *api.CustomerCategoryInput) partner.CategoryInput {
	out := partner.CategoryInput{
		Code: in.Code, Name: in.Name, Description: customerStr(in.Description), Requirement: customerStr(in.Requirement),
		Active: pcBoolOr(in.Active, true),
	}
	if in.RegisterCategory != nil {
		out.RegisterCategory = string(*in.RegisterCategory)
	}
	return out
}

// CustomerCategoryToAPI maps a category view.
func CustomerCategoryToAPI(v *partner.CategoryView) api.CustomerCategory {
	c := &v.CustomerCategory
	n := v.MemberCount
	return api.CustomerCategory{
		Id: c.ID, Code: c.Code, Name: c.Name, Description: c.Description, Requirement: c.Requirement,
		RegisterCategory: api.RegisterCategory(c.RegisterCategory), Active: c.Active, MemberCount: &n,
		Version: c.Version, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// RequirementVerificationToAPI maps a verification; names resolves the
// operator (may be nil).
func RequirementVerificationToAPI(v *store.RequirementVerification, names map[string]string) api.RequirementVerification {
	out := api.RequirementVerification{
		Id: v.ID, Subject: api.RequirementVerificationSubject(v.Subject), BookingId: v.BookingID,
		CustomerId: v.CustomerID, AgreementId: v.AgreementID, CustomerCategoryId: v.CustomerCategoryID,
		Requirement: v.Requirement, Note: strPtr(v.Note), VerifiedAt: v.VerifiedAt, VerifiedByUserId: v.VerifiedByUserID,
	}
	if v.VerifiedByUserID != nil {
		if n := names[*v.VerifiedByUserID]; n != "" {
			out.VerifiedByName = &n
		}
	}
	return out
}

// customerExportM8 adds the M8 personal data to the GDPR export: guest
// names and group origin of the bookings and the requirement verifications.
func customerExportM8(out *api.CustomerExport, x *customer.Export) {
	for i := range x.Bookings {
		if i >= len(out.Bookings) {
			break
		}
		if g := x.Bookings[i].GuestNames; len(g) > 0 {
			names := append([]string{}, g...)
			out.Bookings[i].GuestNames = &names
		}
		if o := x.Bookings[i].GroupOrigin; o != "" {
			out.Bookings[i].GroupOrigin = strPtr(o)
		}
	}
	vs := make([]api.RequirementVerification, 0, len(x.RequirementVerifications))
	for i := range x.RequirementVerifications {
		vs = append(vs, RequirementVerificationToAPI(&x.RequirementVerifications[i], nil))
	}
	out.RequirementVerifications = &vs
}
