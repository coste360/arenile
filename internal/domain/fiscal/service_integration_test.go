//go:build integration

package fiscal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app/builtin"
	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

var t0 = time.Date(2027, 7, 15, 8, 0, 0, 0, time.UTC)

// device is a fake telematic register: it numbers documents itself.
type device struct{ issued int }

func (*device) Code() string { return "rt" }
func (d *device) Issue(context.Context, ext.FiscalDocumentRequest) (*ext.FiscalDocumentResult, error) {
	d.issued++
	return &ext.FiscalDocumentResult{Ref: "rt-ref", Number: "0042-000" + string(rune('0'+d.issued)), IssuedAt: t0}, nil
}
func (*device) Void(context.Context, string) error { return nil }
func (*device) Status(context.Context) (ext.DeviceStatus, error) {
	return ext.DeviceStatus{Online: true}, nil
}

type drivers struct {
	c  clock.Clock
	rt *device
}

func (d drivers) FiscalDriver(code string) (ext.FiscalDriver, bool) {
	switch code {
	case builtin.NoneCode:
		return builtin.NoneFiscal{Clock: d.c}, true
	case "rt":
		return d.rt, true
	}
	return nil, false
}

func TestFiscalService_DocumentsAndExport(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", t0)
		if err != nil {
			t.Fatal(err)
		}
		es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
		if err != nil {
			t.Fatal(err)
		}
		orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
		if err != nil {
			t.Fatal(err)
		}
		esB, err := store.SystemEstablishmentScope(ctx, db, estB.ID)
		if err != nil {
			t.Fatal(err)
		}
		ownerB := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgB.ID, Role: auth.RoleOwner}}}
		c := clock.NewFake(t0)
		rt := &device{}
		svc := fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c), Drivers: drivers{c, rt}})
		owner := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, Role: auth.RoleOwner}}}
		cashier := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, EstablishmentID: est.ID, Role: auth.RoleCashier}}}

		recordIn := func(scope store.EstablishmentScope, day string, dir string, amount money.Cents) *store.Payment {
			t.Helper()
			taxable, tax := money.VATSplit(amount, 1000)
			pm := &store.Payment{
				ID: store.NewID(), Provider: "manual", Method: store.PaymentMethodCash, Direction: dir, Amount: amount,
				Status: store.PaymentSucceeded, BusinessDate: calendar.MustParse(day),
				VATBreakdown: []store.VATShare{{RateBP: 1000, Gross: amount, Taxable: taxable, Tax: tax}}, CreatedAt: t0, UpdatedAt: t0,
			}
			err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
				if err := store.InsertPayment(ctx, tx, scope, pm); err != nil {
					return err
				}
				return svc.RecordPayment(ctx, tx, scope, pm)
			})
			if err != nil {
				t.Fatal(err)
			}
			return pm
		}
		record := func(day string, dir string, amount money.Cents) *store.Payment { return recordIn(es, day, dir, amount) }
		record("2027-07-15", store.PaymentDirectionIn, 1100)
		record("2027-07-15", store.PaymentDirectionIn, 2200)
		record("2027-07-16", store.PaymentDirectionRefund, 550)
		// Another establishment of another organization: its documents and
		// takings never cross over (tenancy, INV-26).
		recordIn(esB, "2027-07-15", store.PaymentDirectionIn, 99900)
		page, err := svc.ListDocuments(ctx, owner, est.ID, store.FiscalDocumentFilter{}, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		nums := []string{}
		for _, d := range page.Items {
			nums = append(nums, *d.DocumentNumber)
		}
		// Newest first; the sequence restarts every business day.
		if len(nums) != 3 || nums[0] != "20270716-0001" || nums[1] != "20270715-0002" || nums[2] != "20270715-0001" {
			t.Fatalf("numbers %v", nums)
		}

		// A device driver: pending in the transaction, issued by the job.
		est.Settings = store.JSON(`{"fiscal":{"driver":"rt"}}`)
		if _, err := db.NewUpdate().Model(est).Column("settings").WherePK().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		record("2027-07-16", store.PaymentDirectionIn, 3300)
		pending, err := svc.ListDocuments(ctx, owner, est.ID, store.FiscalDocumentFilter{Status: store.FiscalPending}, "", 10)
		if err != nil || len(pending.Items) != 1 || rt.issued != 0 {
			t.Fatalf("pending: %+v %v issued %d", pending.Items, err, rt.issued)
		}
		if err := svc.IssuePending(ctx); err != nil {
			t.Fatal(err)
		}
		if err := svc.IssuePending(ctx); err != nil { // nothing left: no double issue
			t.Fatal(err)
		}
		issued, err := svc.ListDocuments(ctx, owner, est.ID, store.FiscalDocumentFilter{Status: store.FiscalIssued}, "", 10)
		if err != nil || len(issued.Items) != 4 || rt.issued != 1 || *issued.Items[0].DocumentNumber != "0042-0001" {
			t.Fatalf("issued: %d %v issued %d", len(issued.Items), err, rt.issued)
		}

		// Export: permissions, validation, totals.
		from, to := calendar.MustParse("2027-07-15"), calendar.MustParse("2027-07-16")
		if _, err := svc.Corrispettivi(ctx, cashier, est.ID, fiscal.CorrispettiviQuery{From: from, To: to}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier export: %v", err)
		}
		if _, err := svc.Corrispettivi(ctx, owner, est.ID, fiscal.CorrispettiviQuery{From: to, To: from}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("inverted range: %v", err)
		}
		if _, err := svc.Corrispettivi(ctx, owner, est.ID, fiscal.CorrispettiviQuery{From: from, To: from.AddDays(400)}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("range too long: %v", err)
		}
		rep, err := svc.Corrispettivi(ctx, owner, est.ID, fiscal.CorrispettiviQuery{From: from, To: to, GroupBy: fiscal.GroupByMonth})
		if err != nil {
			t.Fatal(err)
		}
		if rep.GrandTotal != 1100+2200-550+3300 || len(rep.Rows) != 1 || rep.Rows[0].Count != 4 || rep.Rows[0].Tax != 550 {
			t.Fatalf("report %+v", rep)
		}
		if n, _ := db.NewSelect().TableExpr("outbox_events").Where("type = ?", event.FiscalDocumentRecorded).Count(ctx); n != 6 {
			t.Fatalf("fiscal events %d", n)
		}
		// Cross-tenant: the owner of B gets not found on A and sees only B.
		if _, err := svc.ListDocuments(ctx, ownerB, est.ID, store.FiscalDocumentFilter{}, "", 10); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("documents of A for owner B: %v", err)
		}
		if _, err := svc.Corrispettivi(ctx, ownerB, est.ID, fiscal.CorrispettiviQuery{From: from, To: to}); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("corrispettivi of A for owner B: %v", err)
		}
		if _, err := svc.Corrispettivi(ctx, owner, estB.ID, fiscal.CorrispettiviQuery{From: from, To: to}); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("corrispettivi of B for owner A: %v", err)
		}
		pageB, err := svc.ListDocuments(ctx, ownerB, estB.ID, store.FiscalDocumentFilter{}, "", 10)
		if err != nil || len(pageB.Items) != 1 || pageB.Items[0].EstablishmentID != estB.ID {
			t.Fatalf("documents of B: %v %+v", err, pageB.Items)
		}
		repB, err := svc.Corrispettivi(ctx, ownerB, estB.ID, fiscal.CorrispettiviQuery{From: from, To: to})
		if err != nil || repB.GrandTotal != 99900 || len(repB.Rows) != 1 {
			t.Fatalf("corrispettivi of B: %v %+v", err, repB)
		}
		if len(page.Items) != 3 || len(issued.Items) != 4 {
			t.Fatal("documents of B leaked into A")
		}
	})
}
