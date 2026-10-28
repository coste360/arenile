//go:build integration

package partner_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type env struct {
	f   *bookingtest.Fixture
	svc *partner.Service
	cus *customer.Service
	rep *report.Service
}

func newEnv(t *testing.T, db *store.DB) *env {
	f := bookingtest.New(t, db)
	aw, ob := audit.NewWriter(f.Clock), event.NewOutbox(f.Clock)
	return &env{
		f:   f,
		svc: partner.NewService(partner.Deps{DB: db, Clock: f.Clock, Audit: aw, Outbox: ob}),
		cus: customer.NewService(customer.Deps{DB: db, Clock: f.Clock, Audit: aw, Outbox: ob}),
		rep: report.NewService(report.Deps{DB: db, Clock: f.Clock}),
	}
}

func TestPartners_CRUDScopeAndInUse(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f := e.f
		hotel := "hotel"
		p, err := e.svc.Create(f.Ctx, f.Owner, f.Est.ID, partner.Input{
			Name: " Hotel Miramare ", Kind: store.PartnerKindAccommodation, AccommodationType: &hotel,
			Email: "Info@Miramare.IT", Phone: "333 1234567", Municipality: "Gaeta", Active: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if p.Name != "Hotel Miramare" || *p.Email != "info@miramare.it" || *p.Phone != "+393331234567" || p.Version != 1 {
			t.Fatalf("normalised %+v", p.Partner)
		}
		if _, err := e.svc.Create(f.Ctx, f.Owner, f.Est.ID, partner.Input{Name: "x", Kind: "ente", AccommodationType: &hotel}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("accommodation type on a public body: %v", err)
		}
		if _, err := e.svc.Create(f.Ctx, f.Cashier, f.Est.ID, partner.Input{Name: "x", Kind: "ente"}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier creates a partner: %v", err)
		}
		list, _, err := e.svc.List(f.Ctx, f.Cashier, f.Est.ID, partner.Filter{Q: "mira"}, "", 50)
		if err != nil || len(list) != 1 || list[0].Balance != nil {
			t.Fatalf("search as cashier (no balance): %v %+v", err, list)
		}
		if _, err := e.svc.Get(f.Ctx, f.Outsider, f.Est.ID, p.ID); err == nil {
			t.Fatal("outsider read the partner")
		}
		// In use by an agreement: refused, deactivation allowed.
		_, err = f.Pricing.CreateAgreement(f.Ctx, f.Owner, f.Est.ID, pricingconfig.AgreementInput{
			Name: "Hotel", Action: pricing.Action{Type: pricing.ActionAmountOff, Amount: 100}, Active: true,
			M8: pricingconfig.AgreementM8{Target: store.AgreementTargetPartner, PartnerID: &p.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Delete(f.Ctx, f.Owner, f.Est.ID, p.ID, nil); !errors.Is(err, partner.ErrPartnerInUse) {
			t.Fatalf("delete in use: %v", err)
		}
		v := p.Version
		upd, err := e.svc.Update(f.Ctx, f.Owner, f.Est.ID, p.ID, &v, partner.Input{Name: "Hotel Miramare", Kind: store.PartnerKindAccommodation, Active: false})
		if err != nil || upd.Active || upd.Version != 2 || upd.AgreementCount != 1 {
			t.Fatalf("deactivate: %v %+v", err, upd)
		}
		if _, err := e.svc.Update(f.Ctx, f.Owner, f.Est.ID, p.ID, &v, partner.Input{Name: "y", Kind: "ente"}); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale update: %v", err)
		}
	})
}

func TestCustomerCategories_MembersVerificationsAndGDPR(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f := e.f
		cat, err := e.svc.CreateCategory(f.Ctx, f.Owner, f.Est.ID, partner.CategoryInput{
			Code: "Residenti", Name: "Residenti", Requirement: "documento di residenza", RegisterCategory: store.RegisterResidents, Active: true,
		})
		if err != nil || cat.Code != "residenti" {
			t.Fatalf("create category: %v %+v", err, cat)
		}
		if _, err := e.svc.CreateCategory(f.Ctx, f.Owner, f.Est.ID, partner.CategoryInput{Code: "residenti", Name: "Altro", Active: true}); !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("duplicate code: %v", err)
		}
		ids := []string{cat.ID}
		c, err := e.cus.Create(f.Ctx, f.Owner, "", f.Est.ID, customer.Input{FirstName: "Anna", LastName: "Verdi", CategoryIDs: &ids}, false)
		if err != nil || len(c.CategoryIDs) != 1 {
			t.Fatalf("customer with category: %v %+v", err, c)
		}
		bad := []string{"nope"}
		if _, err := e.cus.Create(f.Ctx, f.Owner, "", f.Est.ID, customer.Input{LastName: "X", CategoryIDs: &bad}, false); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("unknown category: %v", err)
		}
		page, err := e.cus.List(f.Ctx, f.Owner, f.Est.ID, customer.ListFilter{CategoryID: cat.ID}, "", 50)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != c.ID {
			t.Fatalf("filter by category: %v %+v", err, page.Items)
		}
		got, err := e.svc.GetCategory(f.Ctx, f.Owner, f.Est.ID, cat.ID)
		if err != nil || got.MemberCount != 1 {
			t.Fatalf("member count: %v %+v", err, got)
		}
		v, names, err := e.svc.VerifyCustomer(f.Ctx, f.Cashier, f.Est.ID, c.ID, partner.VerifyInput{CustomerCategoryID: &cat.ID, Note: "tessera 2027"})
		if err != nil || v.Requirement != "documento di residenza" || names == nil {
			t.Fatalf("verify: %v %+v", err, v)
		}
		if _, _, err := e.svc.VerifyCustomer(f.Ctx, f.Cashier, f.Est.ID, c.ID, partner.VerifyInput{}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("verification without subject: %v", err)
		}
		// Category in use by an agreement cannot be deleted.
		_, err = f.Pricing.CreateAgreement(f.Ctx, f.Owner, f.Est.ID, pricingconfig.AgreementInput{
			Name: "Residenti", Action: pricing.Action{Type: pricing.ActionAmountOff, Amount: 100}, Active: true,
			M8: pricingconfig.AgreementM8{Target: store.AgreementTargetCategory, CustomerCategoryID: &cat.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.DeleteCategory(f.Ctx, f.Owner, f.Est.ID, cat.ID, nil); !errors.Is(err, partner.ErrCategoryInUse) {
			t.Fatalf("delete category in use: %v", err)
		}
		// GDPR: the export carries the categories and verifications; the
		// anonymization removes them.
		x, err := e.cus.Export(f.Ctx, f.Owner, "", f.Est.ID, c.ID)
		if err != nil || len(x.Customer.CategoryIDs) != 1 || len(x.RequirementVerifications) != 1 {
			t.Fatalf("export: %v %+v", err, x)
		}
		if _, err := e.cus.Anonymize(f.Ctx, f.Owner, "", f.Est.ID, c.ID, nil); err != nil {
			t.Fatal(err)
		}
		es, _ := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
		left, err := store.CustomerM8Data(f.Ctx, db, es, c.ID)
		if err != nil || len(left.CategoryIDs) != 0 || len(left.Verifications) != 0 {
			t.Fatalf("after anonymization: %v %+v", err, left)
		}
	})
}

// TestPartnerAccount_StatementAndPayments: charged follows the partner
// share of the bookings, payments (voided excluded) are subtracted and the
// closing balance is opening + charged - paid (INV-104); the report per
// partner and the agreements register count the same bookings.
func TestPartnerAccount_StatementAndPayments(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f := e.f
		p, err := e.svc.Create(f.Ctx, f.Owner, f.Est.ID, partner.Input{Name: "Hotel Miramare", Kind: store.PartnerKindAccommodation, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		ten := pricing.MustParseDecimal("10")
		ag, err := f.Pricing.CreateAgreement(f.Ctx, f.Owner, f.Est.ID, pricingconfig.AgreementInput{
			Name: "Ospiti Miramare", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: &ten}, Active: true,
			M8: pricingconfig.AgreementM8{Target: store.AgreementTargetPartner, PartnerID: &p.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		book := func(spot int, from, to string, share booking.PartnerShareInput, adults int) *booking.Detail {
			in := f.Input(f.Spots[spot], from, to)
			in.AgreementID = ag.ID
			in.Group = booking.GroupInput{PartnerShare: &share, PartyAdults: &adults}
			return f.MustCreate(in)
		}
		b1 := book(0, "2027-06-25", "2027-06-26", booking.PartnerShareInput{Type: booking.ShareFull}, 2) // before July
		b2 := book(1, "2027-07-01", "2027-07-03", booking.PartnerShareInput{Type: booking.ShareFull}, 3)
		b3 := book(2, "2027-07-10", "2027-07-10", booking.PartnerShareInput{Type: booking.ShareAmount, Value: 500}, 1)
		if b2.Booking.PartnerAmount != 5400 || b3.Booking.PartnerAmount != 500 {
			t.Fatalf("partner amounts %d %d", b2.Booking.PartnerAmount, b3.Booking.PartnerAmount)
		}
		pay := func(amount money.Cents, on string) *store.PartnerPayment {
			x, err := e.svc.RecordPayment(f.Ctx, f.Cashier, f.Est.ID, p.ID, partner.PaymentInput{Amount: amount, ReceivedOn: calendar.MustParse(on), Method: "bank_transfer", Reference: "CRO 1"})
			if err != nil {
				t.Fatal(err)
			}
			return x
		}
		pay(1000, "2027-06-28")
		pay(2000, "2027-07-05")
		wrong := pay(9999, "2027-07-06")
		if _, err := e.svc.VoidPayment(f.Ctx, f.Cashier, f.Est.ID, p.ID, wrong.ID, "importo errato"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.VoidPayment(f.Ctx, f.Cashier, f.Est.ID, p.ID, wrong.ID, "di nuovo"); !errors.Is(err, partner.ErrPaymentVoided) {
			t.Fatalf("void twice: %v", err)
		}
		july, _ := report.NewPeriod(calendar.MustParse("2027-07-01"), calendar.MustParse("2027-07-31"))
		st, err := e.rep.PartnerStatement(f.Ctx, f.Owner, f.Est.ID, p.ID, july)
		if err != nil {
			t.Fatal(err)
		}
		opening := b1.Booking.PartnerAmount - 1000
		if st.OpeningBalance != opening || st.Charged != 5900 || st.Paid != 2000 || st.ClosingBalance != opening+5900-2000 {
			t.Fatalf("statement: opening %d charged %d paid %d closing %d", st.OpeningBalance, st.Charged, st.Paid, st.ClosingBalance)
		}
		if len(st.Bookings) != 2 || st.Totals.SpotDays != 4 || st.Totals.People != 4 || len(st.Payments) != 2 {
			t.Fatalf("statement rows %+v", st)
		}
		if st.Totals.Discount != 600+200 {
			t.Fatalf("discount %d", st.Totals.Discount)
		}
		// The balance on the partner (all time) for reports.read.
		v, err := e.svc.Get(f.Ctx, f.Owner, f.Est.ID, p.ID)
		if err != nil || v.Balance == nil || *v.Balance != b1.Booking.PartnerAmount+5900-3000 {
			t.Fatalf("balance %v %v", err, v.Balance)
		}
		rep, err := e.rep.Partners(f.Ctx, f.Owner, f.Est.ID, july, "")
		if err != nil || len(rep.Rows) != 1 || rep.Rows[0].Charged != st.Charged || rep.Rows[0].Paid != st.Paid || rep.Rows[0].Bookings != 2 {
			t.Fatalf("partners report: %v %+v", err, rep)
		}
		reg, err := e.rep.AgreementsRegister(f.Ctx, f.Owner, f.Est.ID, july, f.Season.ID)
		if err != nil || len(reg.Entries) != 1 || reg.Entries[0].RegisterCategory != store.RegisterAccommodation ||
			reg.Totals.Bookings != 2 || reg.Totals.Discount != st.Totals.Discount || reg.SeasonYear != 2027 {
			t.Fatalf("register: %v %+v", err, reg)
		}
		if _, err := e.rep.PartnerStatement(f.Ctx, f.Cashier, f.Est.ID, p.ID, july); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier reads the statement: %v", err)
		}
	})
}
