package booking

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
)

// Group, party, partner and partner share of a booking (M8, spec 30.2.3,
// 30.2.4, 30.3). Only the guest names are collected about the guests: no
// contacts, no documents (spec 23). bookings.partner_amount, the charge on
// the partner account, is resolved from the share and the total by the
// store on every write (store.PartnerAmountOf, INV-104).

// Limits of the group fields.
const (
	maxGroupName  = 100
	maxGroupOrig  = 200
	maxGuestNames = 50
	maxGuestName  = 100
	maxPartySize  = 999
)

// Partner share types of the API (none clears the share).
const (
	ShareNone    = "none"
	ShareFull    = store.PartnerShareFull
	SharePercent = store.PartnerSharePercent
	ShareAmount  = store.PartnerShareAmount
)

// PartnerShareInput is api.BookingPartnerShare: Value is basis points for
// percent (0..10000) and cents for amount.
type PartnerShareInput struct {
	Type  string
	Value int64
}

// GroupInput is the M8 part of api.BookingCreate / api.BookingPatch. On a
// patch nil fields are unchanged and PartnerID "" removes the partner.
type GroupInput struct {
	PartnerID     *string
	GroupName     *string
	GroupOrigin   *string
	PartyAdults   *int
	PartyChildren *int
	GuestNames    *[]string
	PartnerShare  *PartnerShareInput
	// PartnerStay is the stay of the guest at the partner's structure
	// (optional, informative); a zero range clears it.
	PartnerStay *DateRange
}

// DateRange is an inclusive range of calendar dates.
type DateRange struct {
	From, To calendar.Date
}

// M8Detail is the M8 part of a booking detail.
type M8Detail struct {
	Agreement *AgreementRef
	Partner   *PartnerRef
	// Verification is the latest requirement check of the booking.
	Verification *store.RequirementVerification
	// VerifiedByName is the operator of Verification.
	VerifiedByName string
}

// AgreementRef is api.AgreementRef.
type AgreementRef struct {
	ID          string
	Code        *string
	Name        string
	PartnerID   *string
	PartnerName string
	Requirement string
}

// PartnerRef is api.PartnerRef.
type PartnerRef struct {
	ID   string
	Name string
	Kind string
}

// applyGroup validates in and writes it on b (create: every field; patch:
// the non-nil ones). An agreement whose target is a partner sets the
// partner of a booking that has none. diff, when not nil, receives the
// names of the changed fields (never the guest names).
func applyGroup(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, in GroupInput, diff map[string]any) error {
	v := &configerr.ValidationError{}
	var changed []string
	if in.PartnerID != nil {
		id := strings.TrimSpace(*in.PartnerID)
		if id == "" {
			if b.PartnerID != nil {
				changed = append(changed, "partner_id")
			}
			b.PartnerID = nil
		} else {
			p, err := store.GetPartner(ctx, tx, es, id)
			if _, perr := uuid.Parse(id); perr != nil {
				err = store.ErrNotFound
			}
			switch {
			case errors.Is(err, store.ErrNotFound):
				v.Add("partner_id", "Partner inesistente.")
			case err != nil:
				return fmt.Errorf("booking: partner: %w", err)
			case !p.Active && (b.PartnerID == nil || *b.PartnerID != id):
				v.Add("partner_id", "Partner non attivo.")
			default:
				if b.PartnerID == nil || *b.PartnerID != id {
					changed = append(changed, "partner_id")
				}
				b.PartnerID = &id
			}
		}
	}
	if in.GroupName != nil {
		g := strings.Join(strings.Fields(*in.GroupName), " ")
		if len([]rune(g)) > maxGroupName {
			v.Add("group_name", fmt.Sprintf("Massimo %d caratteri.", maxGroupName))
		}
		if g != b.GroupName {
			changed = append(changed, "group_name")
		}
		b.GroupName = g
	}
	if in.GroupOrigin != nil {
		g := strings.Join(strings.Fields(*in.GroupOrigin), " ")
		if len([]rune(g)) > maxGroupOrig {
			v.Add("group_origin", fmt.Sprintf("Massimo %d caratteri.", maxGroupOrig))
		}
		if g != b.GroupOrigin {
			changed = append(changed, "group_origin")
		}
		b.GroupOrigin = g
	}
	party := func(field string, in *int, dst **int) {
		if in == nil {
			return
		}
		if *in < 0 || *in > maxPartySize {
			v.Add(field, fmt.Sprintf("Tra 0 e %d.", maxPartySize))
			return
		}
		n := *in
		if *dst == nil || **dst != n {
			changed = append(changed, field)
		}
		*dst = &n
	}
	party("party_adults", in.PartyAdults, &b.PartyAdults)
	party("party_children", in.PartyChildren, &b.PartyChildren)
	if in.GuestNames != nil {
		var names []string
		for _, n := range *in.GuestNames {
			n = strings.Join(strings.Fields(n), " ")
			if n == "" {
				continue
			}
			if len([]rune(n)) > maxGuestName {
				v.Add("guest_names", fmt.Sprintf("Ogni nome al massimo %d caratteri.", maxGuestName))
				break
			}
			names = append(names, n)
		}
		if len(names) > maxGuestNames {
			v.Add("guest_names", fmt.Sprintf("Al massimo %d nomi.", maxGuestNames))
		}
		changed = append(changed, "guest_names")
		b.GuestNames = names
	}
	if sh := in.PartnerShare; sh != nil {
		switch sh.Type {
		case ShareNone, "":
			b.PartnerShareType, b.PartnerShareValue = nil, nil
		case ShareFull:
			t := ShareFull
			b.PartnerShareType, b.PartnerShareValue = &t, nil
		case SharePercent:
			if sh.Value <= 0 || sh.Value > 10000 {
				v.Add("partner_share.percent", "Percentuale tra 0 e 100.")
			}
			t, val := SharePercent, sh.Value
			b.PartnerShareType, b.PartnerShareValue = &t, &val
		case ShareAmount:
			if sh.Value <= 0 {
				v.Add("partner_share.amount", "Importo maggiore di zero (in centesimi).")
			}
			t, val := ShareAmount, sh.Value
			b.PartnerShareType, b.PartnerShareValue = &t, &val
		default:
			v.Add("partner_share.type", "Usa none, full, percent o amount.")
		}
		changed = append(changed, "partner_share")
	}
	if r := in.PartnerStay; r != nil {
		if r.From.IsZero() && r.To.IsZero() {
			b.PartnerStayFrom, b.PartnerStayTo = nil, nil
		} else {
			if r.From.IsZero() || r.To.IsZero() || r.To.Before(r.From) {
				v.Add("partner_guest_stay", "Indica dal e al, con la fine non prima dell'inizio.")
			}
			f, t := r.From, r.To
			b.PartnerStayFrom, b.PartnerStayTo = &f, &t
		}
		changed = append(changed, "partner_guest_stay")
	}
	if b.PartnerShareType != nil && b.PartnerID == nil {
		v.Add("partner_share", "La quota a carico del partner richiede il partner.")
	}
	if err := v.OrNil(); err != nil {
		return err
	}
	if diff != nil && len(changed) > 0 {
		diff["group_fields"] = changed
	}
	return nil
}

// defaultPartnerFromAgreement sets the partner of b from its agreement
// when the agreement targets a partner and b has none.
func defaultPartnerFromAgreement(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) error {
	if b.AgreementID == nil || b.PartnerID != nil {
		return nil
	}
	ag, err := store.GetAgreement(ctx, tx, es, *b.AgreementID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("booking: agreement: %w", err)
	}
	if ag.PartnerID != nil && ag.Target != nil && *ag.Target == store.AgreementTargetPartner {
		id := *ag.PartnerID
		b.PartnerID = &id
	}
	return nil
}

// applyAgreementPatch changes the agreement of b (spec 30.2.3): "" removes
// it. The caller reprices the booking afterwards.
func applyAgreementPatch(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, id *string, diff map[string]any) error {
	if id == nil {
		return nil
	}
	next := strings.TrimSpace(*id)
	cur := ""
	if b.AgreementID != nil {
		cur = *b.AgreementID
	}
	if next == cur {
		return nil
	}
	if next == "" {
		b.AgreementID = nil
	} else {
		if _, err := uuid.Parse(next); err != nil {
			return configerr.Invalid("agreement_id", "Convenzione inesistente.")
		}
		if _, err := store.GetAgreement(ctx, tx, es, next); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return configerr.Invalid("agreement_id", "Convenzione inesistente.")
			}
			return fmt.Errorf("booking: agreement: %w", err)
		}
		b.AgreementID = &next
	}
	diff["agreement_changed"] = map[string]any{"from": cur, "to": next}
	return nil
}

// loadM8Detail fills d.M8 (agreement, partner, latest verification).
func loadM8Detail(ctx context.Context, db bun.IDB, es store.EstablishmentScope, d *Detail) error {
	b := &d.Booking
	if b.AgreementID != nil {
		ag, err := store.GetAgreement(ctx, db, es, *b.AgreementID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("booking: agreement: %w", err)
		}
		if err == nil {
			ref := &AgreementRef{ID: ag.ID, Code: ag.Code, Name: ag.Name, PartnerID: ag.PartnerID, PartnerName: ag.PartnerName, Requirement: ag.Requirement}
			if ag.PartnerID != nil {
				if p, err := store.GetPartner(ctx, db, es, *ag.PartnerID); err == nil {
					ref.PartnerName = p.Name
				}
			}
			d.M8.Agreement = ref
		}
	}
	if b.PartnerID != nil {
		p, err := store.GetPartner(ctx, db, es, *b.PartnerID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("booking: partner: %w", err)
		}
		if err == nil {
			d.M8.Partner = &PartnerRef{ID: p.ID, Name: p.Name, Kind: p.Kind}
		}
	}
	vs, err := store.LatestBookingVerifications(ctx, db, es, []string{b.ID})
	if err != nil {
		return fmt.Errorf("booking: verification: %w", err)
	}
	if v, ok := vs[b.ID]; ok {
		d.M8.Verification = &v
		if v.VerifiedByUserID != nil {
			names, err := store.UserNames(ctx, db, []string{*v.VerifiedByUserID})
			if err != nil {
				return fmt.Errorf("booking: verification: %w", err)
			}
			d.M8.VerifiedByName = names[*v.VerifiedByUserID]
		}
	}
	return nil
}

// applyCreateM8 writes the group fields of a new booking and the partner
// of its agreement (before the insert).
func (s *Service) applyCreateM8(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, in CreateInput) error {
	if in.Group.PartnerID == nil {
		if err := defaultPartnerFromAgreement(ctx, tx, es, b); err != nil {
			return err
		}
	}
	return applyGroup(ctx, tx, es, b, in.Group, nil)
}

// afterCreateM8 records the requirement check given with the creation.
func (s *Service) afterCreateM8(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, in CreateInput, by *string, diff map[string]any) error {
	if b.PartnerID != nil {
		diff["partner_id"] = *b.PartnerID
	}
	if b.GroupName != "" {
		diff["group"] = true
	}
	if !in.RequirementVerified || b.AgreementID == nil {
		return nil
	}
	a := *b.AgreementID
	v, err := insertBookingVerification(ctx, tx, es, b, by, partner.VerifyInput{AgreementID: &a}, s.now())
	if err != nil {
		return err
	}
	diff["requirement_verified"] = v.ID
	return nil
}

// applyPatchM8 applies the agreement and group changes of a modification
// before the booking is repriced.
func applyPatchM8(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, in PatchInput, diff map[string]any) error {
	if err := applyAgreementPatch(ctx, tx, es, b, in.AgreementID, diff); err != nil {
		return err
	}
	if in.AgreementID != nil && in.Group.PartnerID == nil {
		if err := defaultPartnerFromAgreement(ctx, tx, es, b); err != nil {
			return err
		}
	}
	return applyGroup(ctx, tx, es, b, in.Group, diff)
}

// applyOnlineAgreementCode resolves the agreement code of an online
// booking (active, valid today, sold online) into in.AgreementID.
func (s *Service) applyOnlineAgreementCode(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in *CreateInput) error {
	code := strings.TrimSpace(in.AgreementCode)
	if code == "" {
		return nil
	}
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return fmt.Errorf("booking: establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return fmt.Errorf("booking: timezone: %w", err)
	}
	a, err := pricingconfig.OnlineAgreement(ctx, tx, es, code, calendar.In(s.now(), loc))
	switch {
	case errors.Is(err, pricingconfig.ErrAgreementCodeUnknown):
		return ErrAgreementNotApplicable
	case err != nil:
		return err
	}
	in.AgreementID = a.ID
	return nil
}
