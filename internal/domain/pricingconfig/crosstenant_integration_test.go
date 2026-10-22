//go:build integration

package pricingconfig

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestCrossTenant_OtherResources: fixed prices, promo codes, agreements,
// booking policies and the season copy of establishment A are invisible to
// the owner of another organization, both through A's establishment id
// (store.ErrNotFound: the establishment itself is unknown to him) and
// through his own establishment with A's resource ids (configerr.ErrNotFound
// or a validation error for a foreign season).
func TestCrossTenant_OtherResources(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		code := "high"
		fp, err := f.svc.CreateFixedPrice(f.ctx, f.manager, f.est.ID, FixedPriceInput{SeasonID: f.season.ID, PackID: f.base.ID, Formula: pricing.FormulaWeek, PriceCategory: "front", RatePeriodCode: &code, Amount: 21000})
		if err != nil {
			t.Fatal(err)
		}
		pc, err := f.svc.CreatePromoCode(f.ctx, f.manager, f.est.ID, PromoCodeInput{Code: "ESTATE10", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("10")}, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		ag, err := f.svc.CreateAgreement(f.ctx, f.manager, f.est.ID, AgreementInput{Name: "Hotel", PartnerName: "Hotel Mare", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("15")}, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		pol, err := f.pol.Create(f.ctx, f.manager, f.est.ID, PolicyInput{SeasonID: f.season.ID, Name: "Agosto", Active: true, Rules: policy.Rules{MaxSpotsPerBooking: policy.Ptr(2)}})
		if err != nil {
			t.Fatal(err)
		}
		o, mine, theirs := f.outsider, f.other.ID, f.est.ID
		foreignEst := func(name string, err error) {
			t.Helper()
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("%s through A's establishment: %v", name, err)
			}
		}
		unknown := func(name string, err error) {
			t.Helper()
			if !errors.Is(err, configerr.ErrNotFound) {
				t.Fatalf("%s through own establishment: %v", name, err)
			}
		}
		// A's establishment id is unknown to the outsider: reads and writes.
		_, _, err = f.svc.ListFixedPrices(f.ctx, o, theirs, FixedPriceListFilter{}, "", 10)
		foreignEst("list fixed", err)
		_, _, err = f.svc.ListPromoCodes(f.ctx, o, theirs, nil, "", 10)
		foreignEst("list promo", err)
		_, _, err = f.svc.ListAgreements(f.ctx, o, theirs, nil, "", 10)
		foreignEst("list agreements", err)
		_, _, err = f.pol.List(f.ctx, o, theirs, "", "", 10)
		foreignEst("list policies", err)
		_, err = f.svc.GetFixedPrice(f.ctx, o, theirs, fp.ID)
		foreignEst("get fixed", err)
		_, err = f.svc.CopySeason(f.ctx, o, theirs, f.season.ID, CopySeasonInput{TargetYear: 2028, IncludePriceRules: true})
		foreignEst("copy season", err)
		foreignEst("delete promo", f.svc.DeletePromoCode(f.ctx, o, theirs, pc.ID, nil))

		// Through his own establishment, A's ids do not exist.
		_, err = f.svc.GetFixedPrice(f.ctx, o, mine, fp.ID)
		unknown("get fixed", err)
		_, err = f.svc.UpdateFixedPrice(f.ctx, o, mine, fp.ID, nil, FixedPriceInput{SeasonID: f.season.ID, PackID: f.base.ID, Formula: pricing.FormulaWeek, PriceCategory: "front", Amount: 1})
		if !errors.Is(err, configerr.ErrNotFound) && !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("update fixed through own establishment: %v", err)
		}
		unknown("delete fixed", f.svc.DeleteFixedPrice(f.ctx, o, mine, fp.ID, nil))
		_, err = f.svc.GetPromoCode(f.ctx, o, mine, pc.ID)
		unknown("get promo", err)
		_, err = f.svc.UpdatePromoCode(f.ctx, o, mine, pc.ID, nil, PromoCodeInput{Code: "ESTATE10", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("10")}, Active: true})
		unknown("update promo", err)
		unknown("delete promo", f.svc.DeletePromoCode(f.ctx, o, mine, pc.ID, nil))
		_, err = f.svc.GetAgreement(f.ctx, o, mine, ag.ID)
		unknown("get agreement", err)
		_, err = f.svc.UpdateAgreement(f.ctx, o, mine, ag.ID, nil, AgreementInput{Name: "Hotel", PartnerName: "X", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("15")}, Active: true})
		unknown("update agreement", err)
		unknown("delete agreement", f.svc.DeleteAgreement(f.ctx, o, mine, ag.ID, nil))
		_, err = f.pol.Get(f.ctx, o, mine, pol.ID)
		unknown("get policy", err)
		unknown("delete policy", f.pol.Delete(f.ctx, o, mine, pol.ID, nil))
		// Writes bound to A's season are a validation error (foreign season).
		_, err = f.svc.CreateFixedPrice(f.ctx, o, mine, FixedPriceInput{SeasonID: f.season.ID, PackID: f.base.ID, Formula: pricing.FormulaWeek, PriceCategory: "front", Amount: 1})
		if !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("fixed price on foreign season: %v", err)
		}
		_, err = f.pol.Create(f.ctx, o, mine, PolicyInput{SeasonID: f.season.ID, Name: "x", Active: true})
		if !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("policy on foreign season: %v", err)
		}
		_, err = f.pol.Update(f.ctx, o, mine, pol.ID, nil, PolicyInput{SeasonID: f.season.ID, Name: "x", Active: true})
		if !errors.Is(err, configerr.ErrNotFound) && !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("update policy through own establishment: %v", err)
		}
		// A's season cannot be the source of a copy into the outsider's lido.
		_, err = f.svc.CopySeason(f.ctx, o, mine, f.season.ID, CopySeasonInput{TargetYear: 2028, IncludePriceRules: true})
		unknown("copy foreign season", err)
		// Nothing of A changed, and the outsider's lists are empty.
		if items, _, err := f.svc.ListPromoCodes(f.ctx, f.viewer, theirs, nil, "", 10); err != nil || len(items) != 1 {
			t.Fatalf("A promo codes: %d %v", len(items), err)
		}
		if items, _, err := f.svc.ListFixedPrices(f.ctx, o, mine, FixedPriceListFilter{}, "", 10); err != nil || len(items) != 0 {
			t.Fatalf("outsider fixed prices: %d %v", len(items), err)
		}
		if items, _, err := f.pol.List(f.ctx, o, mine, "", "", 10); err != nil || len(items) != 0 {
			t.Fatalf("outsider policies: %d %v", len(items), err)
		}
		if n := f.countAudit("fixed_price.deleted") + f.countAudit("promo_code.deleted") + f.countAudit("agreement.deleted") + f.countAudit("booking_policy.deleted"); n != 0 {
			t.Fatalf("deletions audited for refused calls: %d", n)
		}
	})
}

// TestSimulate_DatesInsideSeason: with an explicit season, the stay must
// fall within its dates; otherwise another year's rules would price it.
func TestSimulate_DatesInsideSeason(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		in := SimulateInput{
			SeasonID: f.season.ID, Channel: pricing.ChannelDesk, Slots: calendar.SlotsFull, Formula: pricing.FormulaDay,
			Lines: []pricing.LineInput{{PackID: f.base.ID, ZoneID: f.back.ID}},
		}
		for _, tc := range []struct {
			name, from, to string
			ok             bool
		}{
			{"inside", "2027-07-05", "2027-07-06", true},
			{"edges", "2027-05-15", "2027-09-15", true},
			{"before opening", "2027-05-14", "2027-05-15", false},
			{"after closing", "2027-09-15", "2027-09-16", false},
			{"other year", "2028-07-05", "2028-07-05", false},
		} {
			in.StartsOn, in.EndsOn = d(tc.from), d(tc.to)
			_, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, in)
			var ve *configerr.ValidationError
			switch {
			case tc.ok && err != nil:
				t.Fatalf("%s: %v", tc.name, err)
			case !tc.ok && (!errors.As(err, &ve) || ve.Fields[0].Field != "starts_on"):
				t.Fatalf("%s: want starts_on field error, got %v", tc.name, err)
			}
		}
	})
}
