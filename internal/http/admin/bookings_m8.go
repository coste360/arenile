package admin

import (
	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/http/api"
)

// M8 fields of bookings (spec 30.2.3-30.3, ADR 0028): agreement, partner,
// group, party, guest names and the partner share, on creation, patch and
// detail. The booking service owns their validation (booking.GroupInput).

// bkShareFromAPI converts the partner share; a malformed percent is a
// validation error on partner_share.percent.
func bkShareFromAPI(sh *api.BookingPartnerShare) (*booking.PartnerShareInput, error) {
	if sh == nil {
		return nil, nil
	}
	out := &booking.PartnerShareInput{Type: string(sh.Type)}
	switch out.Type {
	case booking.SharePercent:
		if sh.Percent == nil {
			return nil, configerr.Invalid("partner_share.percent", "Campo obbligatorio.")
		}
		d, err := pricing.ParseDecimal(*sh.Percent)
		bp, ok := d.BasisPoints()
		if err != nil || !ok {
			return nil, configerr.Invalid("partner_share.percent", "Percentuale non valida (al massimo due decimali).")
		}
		out.Value = bp
	case booking.ShareAmount:
		if sh.Amount == nil {
			return nil, configerr.Invalid("partner_share.amount", "Campo obbligatorio.")
		}
		out.Value = *sh.Amount
	}
	return out, nil
}

func bkGroupFromAPI(partnerID, groupName, groupOrigin *string, adults, children *int, guests *[]string,
	share *api.BookingPartnerShare, stay *api.DateRange,
) (booking.GroupInput, error) {
	g := booking.GroupInput{
		PartnerID: partnerID, GroupName: groupName, GroupOrigin: groupOrigin,
		PartyAdults: adults, PartyChildren: children, GuestNames: guests,
	}
	sh, err := bkShareFromAPI(share)
	if err != nil {
		return g, err
	}
	g.PartnerShare = sh
	if stay != nil {
		g.PartnerStay = &booking.DateRange{From: stay.From, To: stay.To}
	}
	return g, nil
}

// bkCreateM8 fills the M8 fields of a creation.
func bkCreateM8(in *booking.CreateInput, req *api.BookingCreate) error {
	g, err := bkGroupFromAPI(req.PartnerId, req.GroupName, req.GroupOrigin, req.PartyAdults, req.PartyChildren,
		req.GuestNames, req.PartnerShare, req.PartnerGuestStay)
	if err != nil {
		return err
	}
	in.Group = g
	in.RequirementVerified = pcBoolOr(req.RequirementVerified, false)
	return nil
}

// bkPatchM8 fills the M8 fields of a modification.
func bkPatchM8(in *booking.PatchInput, req *api.BookingPatch) error {
	g, err := bkGroupFromAPI(req.PartnerId, req.GroupName, req.GroupOrigin, req.PartyAdults, req.PartyChildren,
		req.GuestNames, req.PartnerShare, req.PartnerGuestStay)
	if err != nil {
		return err
	}
	in.Group = g
	in.AgreementID = req.AgreementId
	return nil
}

// bookingDetailM8 adds the M8 fields to a booking detail.
func bookingDetailM8(out *api.BookingDetail, d *booking.Detail) {
	b := &d.Booking
	out.GroupName, out.GroupOrigin = strPtr(b.GroupName), strPtr(b.GroupOrigin)
	out.PartyAdults, out.PartyChildren = b.PartyAdults, b.PartyChildren
	if len(b.GuestNames) > 0 {
		g := append([]string{}, b.GuestNames...)
		out.GuestNames = &g
	}
	if b.PartnerID != nil {
		amt := int64(b.PartnerAmount)
		out.PartnerAmount = &amt
	}
	if b.PartnerShareType != nil {
		sh := &api.BookingPartnerShare{Type: api.BookingPartnerShareType(*b.PartnerShareType)}
		if b.PartnerShareValue != nil {
			switch *b.PartnerShareType {
			case booking.SharePercent:
				p := pricing.NewDecimal(*b.PartnerShareValue, 2).String()
				sh.Percent = &p
			case booking.ShareAmount:
				a := *b.PartnerShareValue
				sh.Amount = &a
			}
		}
		out.PartnerShare = sh
	}
	if b.PartnerStayFrom != nil && b.PartnerStayTo != nil {
		out.PartnerGuestStay = &api.DateRange{From: *b.PartnerStayFrom, To: *b.PartnerStayTo}
	}
	if a := d.M8.Agreement; a != nil {
		out.Agreement = &api.AgreementRef{
			Id: a.ID, Code: a.Code, Name: a.Name, PartnerId: a.PartnerID, PartnerName: strPtr(a.PartnerName),
			Requirement: strPtr(a.Requirement),
		}
	}
	if p := d.M8.Partner; p != nil {
		out.Partner = &api.PartnerRef{Id: p.ID, Name: p.Name, Kind: api.PartnerKind(p.Kind)}
	}
	if v := d.M8.Verification; v != nil {
		names := map[string]string{}
		if v.VerifiedByUserID != nil && d.M8.VerifiedByName != "" {
			names[*v.VerifiedByUserID] = d.M8.VerifiedByName
		}
		rv := RequirementVerificationToAPI(v, names)
		out.RequirementVerification = &rv
	}
}

// uuidParams checks optional id query parameters (a malformed id is a 400,
// never a database error on PostgreSQL).
func uuidParams(params map[string]*string) error {
	v := &configerr.ValidationError{}
	for field, p := range params {
		if p != nil && *p != "" {
			if _, err := uuid.Parse(*p); err != nil {
				v.Add(field, "Identificativo non valido.")
			}
		}
	}
	return v.OrNil()
}
