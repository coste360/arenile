//go:build integration

package customer_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var t0 = time.Date(2027, 6, 1, 8, 0, 0, 0, time.UTC)

type env struct {
	ctx        context.Context
	db         *store.DB
	clock      *clock.Fake
	svc        *customer.Service
	estA, estB *store.Establishment
	owner      *auth.Principal
	cashier    *auth.Principal
	viewer     *auth.Principal
	ownerB     *auth.Principal
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", t0)
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(t0)
	user := func(org, est string, role auth.Role) *auth.Principal {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org, EstablishmentID: est, Role: role}}}
	}
	return &env{
		ctx: ctx, db: db, clock: c, estA: estA, estB: estB,
		svc:     customer.NewService(customer.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)}),
		owner:   user(orgA.ID, "", auth.RoleOwner),
		cashier: user(orgA.ID, estA.ID, auth.RoleCashier),
		viewer:  user(orgA.ID, estA.ID, auth.RoleViewer),
		ownerB:  user(orgB.ID, "", auth.RoleOwner),
	}
}

func (e *env) insert(t *testing.T, m any) {
	t.Helper()
	if _, err := e.db.NewInsert().Model(m).Exec(e.ctx); err != nil {
		t.Fatalf("insert %T: %v", m, err)
	}
}

func (e *env) count(t *testing.T, table, where string, args ...any) int {
	t.Helper()
	n, err := e.db.NewSelect().TableExpr(table).Where(where, args...).Count(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func (e *env) create(t *testing.T, in customer.Input) *store.Customer {
	t.Helper()
	c, err := e.svc.Create(e.ctx, e.cashier, "10.0.0.1", e.estA.ID, in, false)
	if err != nil {
		t.Fatalf("create %s: %v", in.LastName, err)
	}
	return c
}

// accounting inserts a confirmed booking with a line and a cash payment for
// customerID, as the booking and payment services would.
func (e *env) accounting(t *testing.T, customerID string) (*store.Booking, *store.Payment) {
	t.Helper()
	est := e.estA.ID
	m := &store.Map{ID: store.NewID(), EstablishmentID: est, Name: store.I18nText{"it": "Spiaggia"}, Width: 10, Height: 10, GridSize: 1, SeaSide: "bottom", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	e.insert(t, m)
	z := &store.Zone{ID: store.NewID(), EstablishmentID: est, MapID: &m.ID, Code: "row1", Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: "seat", PriceCategory: "row1", Version: 1, CreatedAt: t0, UpdatedAt: t0}
	e.insert(t, z)
	sp := &store.Spot{
		ID: store.NewID(), EstablishmentID: est, MapID: m.ID, ZoneID: z.ID, Label: "A1", RowLabel: "A", Number: 1, Kind: "umbrella",
		Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, sp)
	pk := &store.Pack{
		ID: store.NewID(), EstablishmentID: est, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{},
		Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk"}, MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, pk)
	token := "hash"
	b := &store.Booking{
		ID: store.NewID(), EstablishmentID: est, Code: "AR-TEST1", CustomerID: customerID, Channel: store.ChannelDesk, Status: "confirmed",
		StartsOn: calendar.MustParse("2027-07-01"), EndsOn: calendar.MustParse("2027-07-02"), Slots: calendar.SlotsFull, Formula: "multi_day",
		PriceSnapshot: store.JSON(`{"total":7000}`), TotalAmount: 7000, PaidAmount: 5000, PolicySnapshot: store.JSON(`{"rules":{}}`),
		NotesCustomer: "chiamare al 3331234567", NotesInternal: "cliente abituale", ManageTokenHash: &token, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, b)
	e.insert(t, &store.BookingLine{
		ID: store.NewID(), EstablishmentID: est, BookingID: b.ID, PackID: pk.ID, ZoneID: z.ID, SpotID: &sp.ID,
		AssignmentStatus: store.AssignmentAssigned, Quantity: 1, PriceCategory: "row1", LineTotal: 7000, CreatedAt: t0, UpdatedAt: t0,
	})
	p := &store.Payment{
		ID: store.NewID(), EstablishmentID: est, BookingID: &b.ID, Provider: "manual", Method: store.PaymentMethodCash,
		Direction: store.PaymentDirectionIn, Amount: 5000, Status: store.PaymentSucceeded, BusinessDate: calendar.MustParse("2027-06-01"),
		VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: 5000, Taxable: 4098, Tax: 902}}, Metadata: map[string]any{}, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, p)
	return b, p
}

func mustField(t *testing.T, err error, field string) {
	t.Helper()
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error on %q, got %v", field, err)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("no error on %q: %+v", field, ve.Fields)
}

func TestCustomerService_CreateNormalisesAndDedupes(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		c := e.create(t, customer.Input{
			FirstName: "Mario", LastName: "Rossi", Email: " Mario.Rossi@Example.IT", Phone: "333 123 4567", Tags: []string{"VIP"},
			MarketingConsent: &store.MarketingConsent{Source: "desk", TextVersion: "v1"},
		})
		if *c.Email != "mario.rossi@example.it" || *c.Phone != "+393331234567" || c.Version != 1 || c.Locale != "it" ||
			c.Tags[0] != "vip" || !c.MarketingConsent.GivenAt.Equal(t0) {
			t.Fatalf("created %+v", c)
		}
		if n := e.count(t, "outbox_events", "type = ? AND establishment_id = ?", event.CustomerCreated, e.estA.ID); n != 1 {
			t.Fatalf("customer.created events: %d", n)
		}
		// The audit log keeps field names, never personal data (INV-30, spec 23).
		var diffs []string
		if err := db.NewSelect().TableExpr("audit_log").Column("diff").Where("entity_id = ?", c.ID).Scan(e.ctx, &diffs); err != nil {
			t.Fatal(err)
		}
		if len(diffs) != 1 || strings.Contains(diffs[0], "mario") || strings.Contains(diffs[0], "333") || !strings.Contains(diffs[0], "email") {
			t.Fatalf("audit diff %q", diffs)
		}

		// Dedupe hint: same email or same phone (other format) -> conflict with the existing id.
		for _, in := range []customer.Input{
			{LastName: "Rossi", Email: "MARIO.ROSSI@example.it"},
			{LastName: "Bianchi", Phone: "+39 333 1234567"},
		} {
			_, err := e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, in, false)
			var dup *customer.DuplicateError
			if !errors.As(err, &dup) || dup.ExistingID != c.ID || !errors.Is(err, configerr.ErrConflict) {
				t.Fatalf("duplicate %+v: %v", in, err)
			}
		}
		forced, err := e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, customer.Input{LastName: "Rossi", FirstName: "Lucia", Phone: "3331234567"}, true)
		if err != nil || forced.ID == c.ID {
			t.Fatalf("forced create: %v", err)
		}
		// Another establishment never sees the duplicate.
		if _, err := e.svc.Create(e.ctx, e.ownerB, "", e.estB.ID, customer.Input{LastName: "Rossi", Email: "mario.rossi@example.it"}, false); err != nil {
			t.Fatalf("create in B: %v", err)
		}

		_, err = e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, customer.Input{LastName: "X", Email: "nope", Phone: "12"}, false)
		mustField(t, err, "email")
		mustField(t, err, "phone")
		_, err = e.svc.Create(e.ctx, e.cashier, "", e.estA.ID, customer.Input{LastName: "X", AgreementID: store.NewID()}, false)
		mustField(t, err, "agreement_id")
	})
}

func TestCustomerService_SearchAndPagination(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		rossi := e.create(t, customer.Input{FirstName: "Mario", LastName: "Rossi", Phone: "333 1234567", Tags: []string{"vip"}})
		e.create(t, customer.Input{FirstName: "Nicolò", LastName: "D'Amico", Email: "nicolo@example.it"})
		e.create(t, customer.Input{FirstName: "Anna", LastName: "Bianchi", Tags: []string{"cliente storico", "vip"}})
		e.create(t, customer.Input{FirstName: "Luca", LastName: "Verdi", Tags: []string{"vipp"}})
		if _, err := e.svc.Create(e.ctx, e.ownerB, "", e.estB.ID, customer.Input{LastName: "Rossi"}, false); err != nil {
			t.Fatal(err)
		}
		search := func(f customer.ListFilter) []string {
			t.Helper()
			page, err := e.svc.List(e.ctx, e.viewer, e.estA.ID, f, "", 50)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, c := range page.Items {
				out = append(out, c.LastName)
			}
			return out
		}
		tests := []struct {
			name string
			f    customer.ListFilter
			want string
		}{
			{"all ordered by last name", customer.ListFilter{}, "Bianchi,D'Amico,Rossi,Verdi"},
			{"case insensitive", customer.ListFilter{Q: "ROSSI"}, "Rossi"},
			{"accent free", customer.ListFilter{Q: "nicolo"}, "D'Amico"},
			{"accent in query", customer.ListFilter{Q: "Nicolò d'amico"}, "D'Amico"},
			{"email", customer.ListFilter{Q: "@example.it"}, "D'Amico"},
			{"national phone", customer.ListFilter{Q: "333 123"}, "Rossi"},
			{"international phone", customer.ListFilter{Q: "+39 333 1234567"}, "Rossi"},
			{"tag exact", customer.ListFilter{Tag: "VIP"}, "Bianchi,Rossi"},
			{"tag with spaces", customer.ListFilter{Tag: "cliente storico"}, "Bianchi"},
			{"tag by text", customer.ListFilter{Q: "storico"}, "Bianchi"},
			{"like wildcards escaped", customer.ListFilter{Q: "%"}, ""},
		}
		for _, tt := range tests {
			if got := strings.Join(search(tt.f), ","); got != tt.want {
				t.Fatalf("%s: got %q, want %q", tt.name, got, tt.want)
			}
		}

		// Keyset pagination over (search_text, id).
		var seen []string
		cursor := ""
		for range 10 {
			page, err := e.svc.List(e.ctx, e.viewer, e.estA.ID, customer.ListFilter{}, cursor, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range page.Items {
				seen = append(seen, c.LastName)
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		if strings.Join(seen, ",") != "Bianchi,D'Amico,Rossi,Verdi" {
			t.Fatalf("pages %q", seen)
		}
		if _, err := e.svc.List(e.ctx, e.viewer, e.estA.ID, customer.ListFilter{}, "garbage!", 10); !errors.Is(err, store.ErrInvalidCursor) {
			t.Fatalf("bad cursor: %v", err)
		}

		// Anonymized customers are hidden unless asked for.
		if _, err := e.svc.Anonymize(e.ctx, e.owner, "", e.estA.ID, rossi.ID, nil); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(search(customer.ListFilter{Q: "rossi"}), ","); got != "" {
			t.Fatalf("anonymized still searchable: %q", got)
		}
		if got := search(customer.ListFilter{IncludeAnonymized: true}); len(got) != 4 {
			t.Fatalf("include anonymized: %q", got)
		}
	})
}

func TestCustomerService_UpdateOptimisticAndPermissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		c := e.create(t, customer.Input{LastName: "Rossi", MarketingConsent: &store.MarketingConsent{Source: "desk", TextVersion: "v1"}})

		// Viewer reads but cannot write; cross-tenant access is not found.
		if _, err := e.svc.Get(e.ctx, e.viewer, e.estA.ID, c.ID); err != nil {
			t.Fatalf("viewer get: %v", err)
		}
		if _, err := e.svc.Create(e.ctx, e.viewer, "", e.estA.ID, customer.Input{LastName: "X"}, false); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer create: %v", err)
		}
		if _, err := e.svc.Export(e.ctx, e.viewer, "", e.estA.ID, c.ID); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer export: %v", err)
		}
		if _, err := e.svc.Get(e.ctx, e.ownerB, e.estA.ID, c.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("cross-tenant get: %v", err)
		}
		if _, err := e.svc.Get(e.ctx, e.ownerB, e.estB.ID, c.ID); !errors.Is(err, customer.ErrNotFound) {
			t.Fatalf("customer of A through B: %v", err)
		}
		if _, err := e.svc.Anonymize(e.ctx, e.ownerB, "", e.estB.ID, c.ID, nil); !errors.Is(err, customer.ErrNotFound) {
			t.Fatalf("anonymize through B: %v", err)
		}

		e.clock.Advance(time.Hour)
		v1 := int64(1)
		up, err := e.svc.Update(e.ctx, e.cashier, "", e.estA.ID, c.ID, &v1, customer.Input{
			LastName: "Rossi", FirstName: "Mario", Phone: "+41 79 123 45 67",
			MarketingConsent: &store.MarketingConsent{Source: "desk", TextVersion: "v1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if up.Version != 2 || *up.Phone != "+41791234567" || !up.MarketingConsent.GivenAt.Equal(t0) {
			t.Fatalf("updated %+v (consent %v)", up, up.MarketingConsent)
		}
		var vc *configerr.VersionConflictError
		if _, err := e.svc.Update(e.ctx, e.cashier, "", e.estA.ID, c.ID, &v1, customer.Input{LastName: "X"}); !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale update: %v", err)
		}
		// No change: no new version, no event.
		same, err := e.svc.Update(e.ctx, e.cashier, "", e.estA.ID, c.ID, nil, customer.Input{
			LastName: "Rossi", FirstName: "Mario", Phone: "+41791234567", MarketingConsent: &store.MarketingConsent{Source: "desk", TextVersion: "v1"},
		})
		if err != nil || same.Version != 2 {
			t.Fatalf("no-op update: %v %+v", err, same)
		}
		// Withdrawing the consent (PUT without it).
		w, err := e.svc.Update(e.ctx, e.cashier, "", e.estA.ID, c.ID, nil, customer.Input{LastName: "Rossi", FirstName: "Mario", Phone: "+41791234567"})
		if err != nil || w.MarketingConsent != nil || w.Version != 3 {
			t.Fatalf("withdraw consent: %v %+v", err, w)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.CustomerUpdated); n != 2 {
			t.Fatalf("customer.updated events: %d", n)
		}
	})
}

func TestCustomerService_AnonymizeKeepsAccounting(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		c := e.create(t, customer.Input{
			FirstName: "Mario", LastName: "Rossi", Email: "mario@example.it", Phone: "3331234567", Notes: "allergico", Tags: []string{"vip"},
			MarketingConsent: &store.MarketingConsent{Source: "desk", TextVersion: "v1"},
		})
		b, p := e.accounting(t, c.ID)
		if err := e.svc.GrantCredit(e.ctx, db, mustScope(t, e), &store.CustomerCredit{CustomerID: c.ID, Amount: 1500, Source: store.CreditSourceManual}); err != nil {
			t.Fatal(err)
		}

		x, err := e.svc.Export(e.ctx, e.owner, "", e.estA.ID, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(x.Bookings) != 1 || len(x.Payments) != 1 || len(x.Credits) != 1 || x.BookingCodes[b.ID] != "AR-TEST1" ||
			x.SpotLabels[b.ID][0] != "A1" || *x.Customer.Email != "mario@example.it" {
			t.Fatalf("export %+v", x)
		}
		if n := e.count(t, "audit_log", "action = ? AND entity_id = ?", "customer.exported", c.ID); n != 1 {
			t.Fatalf("export audit: %d", n)
		}

		v := int64(1)
		an, err := e.svc.Anonymize(e.ctx, e.owner, "", e.estA.ID, c.ID, &v)
		if err != nil {
			t.Fatal(err)
		}
		if an.Email != nil || an.Phone != nil || an.FirstName != "" || an.LastName != customer.AnonymizedLastName ||
			an.Notes != "" || len(an.Tags) != 0 || an.MarketingConsent != nil || an.AnonymizedAt == nil || an.SearchText != "" {
			t.Fatalf("anonymized %+v", an)
		}
		// Bookings, payments and credits are untouched for accounting; the
		// free-text notes and the manage token are wiped.
		gotB, err := store.PassGetBooking(e.ctx, db, mustScope(t, e), b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotB.CustomerID != c.ID || gotB.TotalAmount != 7000 || gotB.PaidAmount != 5000 || gotB.Status != "confirmed" ||
			gotB.NotesCustomer != "" || gotB.NotesInternal != "" || gotB.ManageTokenHash != nil || gotB.Version != 2 {
			t.Fatalf("booking after anonymize %+v", gotB)
		}
		if n := e.count(t, "payments", "id = ? AND amount = ? AND booking_id = ?", p.ID, 5000, b.ID); n != 1 {
			t.Fatal("payment changed by anonymization")
		}
		if n := e.count(t, "booking_lines", "booking_id = ?", b.ID); n != 1 {
			t.Fatal("booking lines changed by anonymization")
		}
		bal, err := e.svc.CreditBalance(e.ctx, db, mustScope(t, e), c.ID, calendar.MustParse("2027-06-01"))
		if err != nil || bal != 1500 {
			t.Fatalf("credit balance after anonymize: %v %v", bal, err)
		}
		// Personal data is gone from every table that held it, audit included.
		for _, table := range []string{"customers", "bookings", "audit_log", "outbox_events"} {
			if n := e.count(t, table, "CAST("+textOf(table)+" AS TEXT) LIKE ?", "%mario@example.it%"); n != 0 {
				t.Fatalf("email still in %s", table)
			}
		}
		if n := e.count(t, "outbox_events", "type = ?", event.CustomerAnonymized); n != 1 {
			t.Fatalf("customer.anonymized events: %d", n)
		}
		if _, err := e.svc.Update(e.ctx, e.owner, "", e.estA.ID, c.ID, nil, customer.Input{LastName: "Rossi"}); !errors.Is(err, customer.ErrAnonymized) {
			t.Fatalf("update anonymized: %v", err)
		}
		if _, err := e.svc.Anonymize(e.ctx, e.owner, "", e.estA.ID, c.ID, nil); !errors.Is(err, customer.ErrAnonymized) {
			t.Fatalf("anonymize twice: %v", err)
		}
		d, err := e.svc.Get(e.ctx, e.viewer, e.estA.ID, c.ID)
		if err != nil || d.Stats.BookingsCount != 1 || d.Stats.TotalSpent != 5000 || d.Stats.BalanceDue != 2000 ||
			d.Stats.LastBookingOn == nil || d.Stats.LastBookingOn.String() != "2027-07-01" || d.CreditBalance != 1500 || len(d.RecentBookings) != 1 {
			t.Fatalf("detail after anonymize: %v %+v", err, d)
		}
	})
}

// textOf is the column whose text may contain personal data.
func textOf(table string) string {
	switch table {
	case "customers":
		return "search_text || COALESCE(email, '') || notes"
	case "bookings":
		return "notes_customer || notes_internal"
	case "audit_log":
		return "diff"
	}
	return "payload"
}

func TestCustomerService_CreditsAndResolve(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		es := mustScope(t, e)
		c := e.create(t, customer.Input{LastName: "Rossi"})
		exp := calendar.MustParse("2027-06-30")
		for _, cr := range []store.CustomerCredit{
			{CustomerID: c.ID, Amount: 2000, Source: store.CreditSourceSeasonRelease},
			{CustomerID: c.ID, Amount: 1000, Source: store.CreditSourceWeather, ExpiresOn: &exp},
			{CustomerID: c.ID, Amount: -500, Source: store.CreditSourceRedemption},
		} {
			if err := e.svc.GrantCredit(e.ctx, db, es, &cr); err != nil {
				t.Fatal(err)
			}
		}
		for day, want := range map[string]money.Cents{"2027-06-30": 2500, "2027-07-01": 1500} {
			got, err := e.svc.CreditBalance(e.ctx, db, es, c.ID, calendar.MustParse(day))
			if err != nil || got != want {
				t.Fatalf("balance on %s = %v (%v), want %v", day, got, err, want)
			}
		}
		if err := e.svc.GrantCredit(e.ctx, db, es, &store.CustomerCredit{CustomerID: c.ID, Source: store.CreditSourceManual}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("zero credit: %v", err)
		}
		esB, err := store.SystemEstablishmentScope(e.ctx, db, e.estB.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.GrantCredit(e.ctx, db, esB, &store.CustomerCredit{CustomerID: c.ID, Amount: 1, Source: store.CreditSourceManual}); !errors.Is(err, customer.ErrNotFound) {
			t.Fatalf("credit through another establishment: %v", err)
		}

		// Resolve inside a caller transaction (booking service port).
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			got, err := e.svc.Resolve(ctx, tx, e.cashier, es, c.ID, nil)
			if err != nil || got.ID != c.ID {
				t.Fatalf("resolve existing: %v", err)
			}
			created, err := e.svc.Resolve(ctx, tx, e.cashier, es, "", &customer.Input{LastName: "Nuovo", Email: "Nuovo@Example.it"})
			if err != nil || *created.Email != "nuovo@example.it" {
				t.Fatalf("resolve inline: %v", err)
			}
			_, err = e.svc.Resolve(ctx, tx, e.cashier, es, "", &customer.Input{LastName: ""})
			mustField(t, err, "customer.last_name")
			if _, err := e.svc.Resolve(ctx, tx, e.viewer, es, "", &customer.Input{LastName: "X"}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("viewer inline: %v", err)
			}
			if _, err := e.svc.Resolve(ctx, tx, e.cashier, es, store.NewID(), nil); !errors.Is(err, customer.ErrNotFound) {
				t.Fatalf("resolve unknown: %v", err)
			}
			if _, err := e.svc.Resolve(ctx, tx, e.cashier, es, c.ID, &customer.Input{LastName: "X"}); !errors.Is(err, configerr.ErrValidation) {
				t.Fatalf("resolve both: %v", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Anonymize(e.ctx, e.owner, "", e.estA.ID, c.ID, nil); err != nil {
			t.Fatal(err)
		}
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			_, err := e.svc.Resolve(ctx, tx, e.cashier, es, c.ID, nil)
			return err
		})
		if !errors.Is(err, customer.ErrAnonymized) {
			t.Fatalf("resolve anonymized: %v", err)
		}
	})
}

func mustScope(t *testing.T, e *env) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(e.ctx, e.db, e.estA.ID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}
