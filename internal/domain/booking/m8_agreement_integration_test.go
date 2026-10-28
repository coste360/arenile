//go:build integration

package booking_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// M8 agreements on bookings (spec 30.2-30.3, ADR 0028): partner, group and
// partner share, the partner amount following the total (INV-104), fixed
// pack prices with versions (INV-21), channels and the requirement check.

func m8Partner(t *testing.T, f *bookingtest.Fixture) *partner.View {
	t.Helper()
	svc := partner.NewService(partner.Deps{DB: f.DB, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock)})
	p, err := svc.Create(f.Ctx, f.Owner, f.Est.ID, partner.Input{Name: "Hotel Miramare", Kind: store.PartnerKindAccommodation, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func m8Agreement(t *testing.T, f *bookingtest.Fixture, in pricingconfig.AgreementInput) *pricingconfig.Agreement {
	t.Helper()
	if in.Name == "" {
		in.Name = "Convenzione"
	}
	in.Active = true
	a, err := f.Pricing.CreateAgreement(f.Ctx, f.Owner, f.Est.ID, in)
	if err != nil {
		t.Fatalf("create agreement: %v", err)
	}
	return a
}

func pct(s string) pricing.Action {
	v := pricing.MustParseDecimal(s)
	return pricing.Action{Type: pricing.ActionPercentOff, Percent: &v}
}

func TestPartnerAmount_FollowsTotal(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		p := m8Partner(t, f)
		code := "HOTEL"
		ag := m8Agreement(t, f, pricingconfig.AgreementInput{
			Code: &code, Action: pct("10"),
			M8: pricingconfig.AgreementM8{Target: store.AgreementTargetPartner, PartnerID: &p.ID, Requirement: "conferma dell'hotel"},
		})
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.AgreementID = ag.ID
		adults, children := 2, 1
		in.Group = booking.GroupInput{
			GroupName: ptrTo("Famiglia Bianchi"), PartyAdults: &adults, PartyChildren: &children,
			GuestNames: &[]string{"  Anna  Bianchi ", ""}, PartnerShare: &booking.PartnerShareInput{Type: booking.ShareFull},
		}
		in.RequirementVerified = true
		d := f.MustCreate(in)
		b := d.Booking
		if b.PartnerID == nil || *b.PartnerID != p.ID {
			t.Fatalf("partner from the agreement target: %v", b.PartnerID)
		}
		if b.TotalAmount != 2*bookingtest.BasePerDay*9/10 || b.PartnerAmount != b.TotalAmount {
			t.Fatalf("total %d partner %d", b.TotalAmount, b.PartnerAmount)
		}
		if len(b.GuestNames) != 1 || b.GuestNames[0] != "Anna Bianchi" || b.GroupName != "Famiglia Bianchi" {
			t.Fatalf("group %+v %q", b.GuestNames, b.GroupName)
		}
		if d.M8.Verification == nil || d.M8.Verification.Requirement != "conferma dell'hotel" || d.M8.Agreement == nil || d.M8.Partner == nil {
			t.Fatalf("detail m8 %+v", d.M8)
		}
		// Extending the stay: the partner amount follows the total.
		ends := bookingtest.D("2027-07-12")
		res, err := f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, b.ID, &b.Version, booking.PatchInput{EndsOn: &ends})
		if err != nil {
			t.Fatal(err)
		}
		nb := res.Detail.Booking
		if nb.TotalAmount != 3*bookingtest.BasePerDay*9/10 || nb.PartnerAmount != nb.TotalAmount {
			t.Fatalf("after extend: total %d partner %d", nb.TotalAmount, nb.PartnerAmount)
		}
		// Half charged to the partner.
		res, err = f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, b.ID, &nb.Version, booking.PatchInput{
			Group: booking.GroupInput{PartnerShare: &booking.PartnerShareInput{Type: booking.SharePercent, Value: 5000}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Detail.Booking; got.PartnerAmount != got.TotalAmount/2 {
			t.Fatalf("half share: %d of %d", got.PartnerAmount, got.TotalAmount)
		}
		// Removing the agreement reprices at list price; the partner stays.
		none := ""
		v := res.Detail.Booking.Version
		res, err = f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, b.ID, &v, booking.PatchInput{AgreementID: &none})
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Detail.Booking; got.AgreementID != nil || got.TotalAmount != 3*bookingtest.BasePerDay || got.PartnerAmount != got.TotalAmount/2 {
			t.Fatalf("agreement removed: %+v", got)
		}
		// A cancellation never charges the partner.
		if _, err := f.Svc.Cancel(f.Ctx, f.Owner, f.Est.ID, b.ID, booking.CancelInput{Reason: "rinuncia"}); err != nil {
			t.Fatal(err)
		}
		got, err := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, b.ID)
		if err != nil || got.Booking.PartnerAmount != 0 {
			t.Fatalf("cancelled: %v %d", err, got.Booking.PartnerAmount)
		}
	})
}

func TestBooking_AgreementPackPriceAndVersions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		zero := pct("0")
		ag := m8Agreement(t, f, pricingconfig.AgreementInput{
			Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: zero.Percent},
			M8: pricingconfig.AgreementM8{
				Target: store.AgreementTargetCustomers, PackPrices: []pricing.AgreementPackPrice{{PackID: f.Base.ID, Amount: 1200}},
				RegisterCategory: ptrTo(store.RegisterResidents),
			},
		})
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.AgreementID = ag.ID
		first := f.MustCreate(in)
		if first.Booking.TotalAmount != 2400 {
			t.Fatalf("fixed price: %d", first.Booking.TotalAmount)
		}
		upd := pricingconfig.AgreementInput{
			Name: ag.Name, Action: ag.Action, Active: true,
			M8: pricingconfig.AgreementM8{Target: store.AgreementTargetCustomers, PackPrices: []pricing.AgreementPackPrice{{PackID: f.Base.ID, Amount: 1000}}},
		}
		if _, err := f.Pricing.UpdateAgreement(f.Ctx, f.Owner, f.Est.ID, ag.ID, &ag.Version, upd); err != nil {
			t.Fatal(err)
		}
		versions, err := f.Pricing.ListAgreementVersions(f.Ctx, f.Owner, f.Est.ID, ag.ID)
		if err != nil || len(versions) != 2 || versions[0].Version != 2 || versions[1].Input.M8.PackPrices[0].Amount != 1200 {
			t.Fatalf("versions: %v %+v", err, versions)
		}
		// INV-21: the booking priced with version 1 keeps its price.
		got, _ := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, first.Booking.ID)
		if got.Booking.TotalAmount != 2400 {
			t.Fatalf("existing booking repriced: %d", got.Booking.TotalAmount)
		}
		in = f.Input(f.Spots[1], "2027-07-10", "2027-07-11")
		in.AgreementID = ag.ID
		if second := f.MustCreate(in); second.Booking.TotalAmount != 2000 {
			t.Fatalf("new version: %d", second.Booking.TotalAmount)
		}
	})
}

func TestBooking_AgreementChannelsAndValidation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		code := "WEB10"
		online := m8Agreement(t, f, pricingconfig.AgreementInput{
			Code: &code, Action: pct("10"), M8: pricingconfig.AgreementM8{Channels: []string{pricingconfig.ChannelOnlineLink}},
		})
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-10")
		in.AgreementID = online.ID
		if _, err := f.Svc.Create(f.Ctx, f.Owner, f.Est.ID, in); !errors.Is(err, booking.ErrAgreementNotApplicable) {
			t.Fatalf("online-only agreement at the desk: %v", err)
		}
		in = f.Input(f.Spots[0], "2027-07-10", "2027-07-10")
		in.Group = booking.GroupInput{PartnerShare: &booking.PartnerShareInput{Type: booking.ShareFull}}
		if _, err := f.Svc.Create(f.Ctx, f.Owner, f.Est.ID, in); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("share without partner: %v", err)
		}
		// An online code without a code is refused when the agreement is saved.
		_, err := f.Pricing.CreateAgreement(f.Ctx, f.Owner, f.Est.ID, pricingconfig.AgreementInput{
			Name: "x", Action: pct("5"), Active: true, M8: pricingconfig.AgreementM8{Channels: []string{pricingconfig.ChannelOnlineCode}},
		})
		if !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("online channel without code: %v", err)
		}
	})
}

func TestBooking_VerifyRequirement(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		ag := m8Agreement(t, f, pricingconfig.AgreementInput{Action: pct("10"), M8: pricingconfig.AgreementM8{Requirement: "documento di residenza"}})
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-10")
		in.AgreementID = ag.ID
		d := f.MustCreate(in)
		if d.M8.Verification != nil {
			t.Fatal("not verified yet")
		}
		got, err := f.Svc.VerifyRequirement(f.Ctx, f.Cashier, f.Est.ID, d.Booking.ID, booking.VerifyInput{
			ExpectedVersion: &d.Booking.Version, Note: "carta d'identità vista",
		})
		if err != nil {
			t.Fatal(err)
		}
		v := got.M8.Verification
		if v == nil || v.Requirement != "documento di residenza" || v.VerifiedByUserID == nil || *v.VerifiedByUserID != f.Cashier.UserID || got.Booking.Version != d.Booking.Version+1 {
			t.Fatalf("verification %+v version %d", v, got.Booking.Version)
		}
		if _, err := f.Svc.VerifyRequirement(f.Ctx, f.Viewer, f.Est.ID, d.Booking.ID, booking.VerifyInput{}); err == nil {
			t.Fatal("viewer verified a requirement")
		}
	})
}

func ptrTo[T any](v T) *T { return &v }
