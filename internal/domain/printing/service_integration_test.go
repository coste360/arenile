//go:build integration

package printing_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/printing"
	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/printing/printertest"
	"github.com/coste360/arenile/internal/printing/templates"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

const ticketID = "0192f1a2-7b3c-7def-8123-456789abcdef"

var fastArgon = auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

type fixture struct {
	ctx     context.Context
	db      *store.DB
	clock   *clock.Fake
	svc     *printing.Service
	est     *store.Establishment
	estB    *store.Establishment
	owner   *auth.Principal
	cashier *auth.Principal
	ownerB  *auth.Principal
	logos   *fakeLogos
	tickets *fakeTickets
}

type fakeLogos struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (f *fakeLogos) LoadLogo(context.Context, string) (image.Image, error) {
	f.mu.Lock()
	f.calls++
	fail := f.fail
	f.mu.Unlock()
	if fail {
		return nil, errors.New("dial tcp: no route to host")
	}
	img := image.NewGray(image.Rect(0, 0, 100, 40))
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	img.Set(0, 0, color.White)
	return img, nil
}

type fakeTickets struct{}

func (fakeTickets) PrintableTicket(_ context.Context, _ store.EstablishmentScope, id string) (templates.TicketDoc, bool, error) {
	if id != ticketID {
		return templates.TicketDoc{}, false, store.ErrNotFound
	}
	return templates.TicketDoc{
		Number: "0042", Date: calendar.New(2027, 7, 1), Zone: "Prima fila", Formula: "Giornata intera",
		Pack: "Base", PackAmount: 2500, Total: 2500, BookingCode: "ARN-7F3K", PaymentMethod: "Contanti",
	}, true, nil
}

func newFixture(t *testing.T, db *store.DB, available bool) *fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	aw := audit.NewWriter(c)
	as, err := auth.NewService(db, c, nil, aw, nil, auth.Options{Argon2: &fastArgon})
	if err != nil {
		t.Fatal(err)
	}
	res, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-a.it", Name: "Owner A", Password: "password-owner-1", OrganizationName: "Org A"})
	if err != nil {
		t.Fatal(err)
	}
	est := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a", "Lido Azzurro", c.Now())
	est.Theme.LogoURL = "https://lido.example/logo.png"
	if err := store.InsertEstablishment(ctx, db, est); err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{ctx: ctx, db: db, clock: c, est: est, estB: estB, logos: &fakeLogos{}, tickets: &fakeTickets{}}
	f.svc = printing.NewService(printing.Deps{
		DB: db, Clock: c, Audit: aw, Available: available, Logos: f.logos, Tickets: f.tickets, AllowLoopback: true,
		Drivers: map[string]printing.Driver{printing.DriverNetwork: printing.NetworkDriver{
			DialTimeout: time.Second, WriteTimeout: 2 * time.Second, StatusTimeout: 100 * time.Millisecond, AllowLoopback: true,
		}},
	})
	f.owner = principal(ctx, t, db, res.User.ID)
	f.cashier = f.member(t, "cassa@lido-a.it", res.Organization.ID, &est.ID, auth.RoleCashier)
	f.ownerB = f.member(t, "owner@lido-b.it", orgB.ID, nil, auth.RoleOwner)
	return f
}

func (f *fixture) member(t *testing.T, email, orgID string, estID *string, role auth.Role) *auth.Principal {
	t.Helper()
	now := f.clock.Now()
	u := &store.User{ID: store.NewID(), Email: email, PasswordHash: "x", Name: strings.Split(email, "@")[0], Locale: "it", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := store.InsertUser(f.ctx, f.db, u); err != nil {
		t.Fatal(err)
	}
	m := &store.Membership{ID: store.NewID(), UserID: u.ID, OrganizationID: orgID, EstablishmentID: estID, Role: string(role), CreatedAt: now, UpdatedAt: now}
	if err := store.InsertMembership(f.ctx, f.db, m); err != nil {
		t.Fatal(err)
	}
	return principal(f.ctx, t, f.db, u.ID)
}

func principal(ctx context.Context, t *testing.T, db *store.DB, userID string) *auth.Principal {
	t.Helper()
	ms, err := store.ListUserMemberships(ctx, db, userID)
	if err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{Kind: auth.PrincipalUser, UserID: userID}
	for _, m := range ms {
		g := auth.Grant{OrganizationID: m.OrganizationID, Role: auth.Role(m.Role)}
		if m.EstablishmentID != nil {
			g.EstablishmentID = *m.EstablishmentID
		}
		p.Grants = append(p.Grants, g)
	}
	return p
}

func (f *fixture) printer(t *testing.T, name string, fp *printertest.Printer, mut ...func(*printing.PrinterInput)) *store.Printer {
	t.Helper()
	in := printing.PrinterInput{Name: name, Host: fp.Host(), Port: fp.Port(), PaperWidthMM: 80}
	for _, m := range mut {
		m(&in)
	}
	pr, err := f.svc.CreatePrinter(f.ctx, f.owner, f.est.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	return pr
}

func (f *fixture) scope(t *testing.T) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(f.ctx, f.db, f.est.ID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func (f *fixture) register(t *testing.T, code string) string {
	t.Helper()
	now := f.clock.Now()
	reg := &store.CashRegister{ID: store.NewID(), EstablishmentID: f.est.ID, Code: code, Name: "Cassa " + code, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := f.db.NewInsert().Model(reg).Exec(f.ctx); err != nil {
		t.Fatal(err)
	}
	return reg.ID
}

func (f *fixture) deliver(t *testing.T) int {
	t.Helper()
	n, err := f.svc.DeliverDue(f.ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestService_PrinterCRUD(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pin := 5
		pr := f.printer(t, "Banco bar", fp, func(in *printing.PrinterInput) { in.PaperWidthMM, in.DrawerPin, in.PrintLogo = 58, &pin, true })
		if pr.CharsPerLine != 32 || pr.Codepage != "cp858" || pr.CutMode != "partial" || pr.FeedLines != 3 || !pr.Active || pr.Version != 1 || pr.Status != "unknown" {
			t.Fatalf("defaults: %+v", pr)
		}
		// Name unique, public address refused, validation.
		if _, err := f.svc.CreatePrinter(f.ctx, f.owner, f.est.ID, printing.PrinterInput{Name: "Banco bar", Host: "192.168.1.9", PaperWidthMM: 80}); !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("duplicate name: %v", err)
		}
		if _, err := f.svc.CreatePrinter(f.ctx, f.owner, f.est.ID, printing.PrinterInput{Name: "X", Host: "8.8.8.8", PaperWidthMM: 80}); !errors.Is(err, printing.ErrAddressNotAllowed) {
			t.Fatalf("public: %v", err)
		}
		_, err := f.svc.CreatePrinter(f.ctx, f.owner, f.est.ID, printing.PrinterInput{Name: "", Host: "192.168.1.9", PaperWidthMM: 60, Port: 70000})
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) != 3 {
			t.Fatalf("validation: %v", err)
		}
		// Permissions and tenancy.
		if _, err := f.svc.CreatePrinter(f.ctx, f.cashier, f.est.ID, printing.PrinterInput{Name: "C", Host: "192.168.1.9", PaperWidthMM: 80}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier create: %v", err)
		}
		if _, err := f.svc.ListPrinters(f.ctx, f.cashier, f.est.ID, false); err != nil {
			t.Fatalf("cashier list: %v", err)
		}
		if _, err := f.svc.GetPrinter(f.ctx, f.ownerB, f.est.ID, pr.ID); err == nil {
			t.Fatal("other organization reads the printer")
		}
		if _, err := f.svc.GetPrinter(f.ctx, f.ownerB, f.estB.ID, pr.ID); !errors.Is(err, printing.ErrNotFound) {
			t.Fatalf("cross-establishment get: %v", err)
		}
		// Logo cache is invalidated by a paper width change.
		if err := store.SetPrinterLogo(f.ctx, db, pr.ID, []byte{0, 1, 0, 1, 0xff}, ptr("x")); err != nil {
			t.Fatal(err)
		}
		v := int64(1)
		inactive := false
		up, err := f.svc.UpdatePrinter(f.ctx, f.owner, f.est.ID, pr.ID, &v, printing.PrinterInput{Name: "Banco", Host: fp.Host(), Port: fp.Port(), PaperWidthMM: 80, PrintLogo: true, Active: &inactive})
		if err != nil {
			t.Fatal(err)
		}
		if up.Version != 2 || up.CharsPerLine != 48 || up.Active || up.DrawerPin != nil || len(up.LogoRaster) != 0 {
			t.Fatalf("update: %+v", up)
		}
		if _, err := f.svc.UpdatePrinter(f.ctx, f.owner, f.est.ID, pr.ID, &v, printing.PrinterInput{Name: "Banco", Host: fp.Host(), PaperWidthMM: 80}); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale update: %v", err)
		}
		list, _ := f.svc.ListPrinters(f.ctx, f.owner, f.est.ID, false)
		all, _ := f.svc.ListPrinters(f.ctx, f.owner, f.est.ID, true)
		if len(list) != 0 || len(all) != 1 {
			t.Fatalf("list active %d, all %d", len(list), len(all))
		}
		if err := f.svc.DeletePrinter(f.ctx, f.owner, f.est.ID, pr.ID, &v); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale delete: %v", err)
		}
		if err := f.svc.DeletePrinter(f.ctx, f.owner, f.est.ID, pr.ID, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.GetPrinter(f.ctx, f.owner, f.est.ID, pr.ID); !errors.Is(err, printing.ErrNotFound) {
			t.Fatalf("deleted: %v", err)
		}
	})
}

func TestService_RoutesAndResolve(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		bar := f.printer(t, "Bar", fp)
		cassa := f.printer(t, "Cassa", fp)
		regBar := f.register(t, "bar")
		regBeach := f.register(t, "spiaggia")

		rs, err := f.svc.GetRoutes(f.ctx, f.cashier, f.est.ID)
		if err != nil || len(rs.Routes) != 0 || rs.Version < 1 {
			t.Fatalf("empty routes: %+v %v", rs, err)
		}
		res, err := f.svc.ResolveRoute(f.ctx, f.cashier, f.est.ID, store.PrintDocPaymentReceipt, nil)
		if err != nil || res.Ready || res.Reason != printing.ReasonNoRoute {
			t.Fatalf("no route: %+v %v", res, err)
		}
		routes := []printing.Route{
			{DocumentKind: store.PrintDocPaymentReceipt, PrinterID: cassa.ID, Copies: 2, KickDrawer: true},
			{DocumentKind: store.PrintDocPaymentReceipt, RegisterID: &regBar, PrinterID: bar.ID},
			{DocumentKind: store.PrintDocCashZ, PrinterID: cassa.ID},
		}
		if _, err := f.svc.UpdateRoutes(f.ctx, f.cashier, f.est.ID, nil, routes); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier update routes: %v", err)
		}
		stale := rs.Version + 1
		if _, err := f.svc.UpdateRoutes(f.ctx, f.owner, f.est.ID, &stale, routes); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale routes: %v", err)
		}
		saved, err := f.svc.UpdateRoutes(f.ctx, f.owner, f.est.ID, &rs.Version, routes)
		if err != nil || len(saved.Routes) != 3 || saved.Version == rs.Version {
			t.Fatalf("saved: %+v %v", saved, err)
		}
		dup := append(slices.Clone(routes), printing.Route{DocumentKind: store.PrintDocCashZ, PrinterID: bar.ID})
		if _, err := f.svc.UpdateRoutes(f.ctx, f.owner, f.est.ID, nil, dup); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("duplicate rule: %v", err)
		}
		foreign := "00000000-0000-0000-0000-000000000000"
		if _, err := f.svc.UpdateRoutes(f.ctx, f.owner, f.est.ID, nil, []printing.Route{{DocumentKind: store.PrintDocCashZ, PrinterID: foreign}}); !errors.Is(err, printing.ErrNotFound) {
			t.Fatalf("unknown printer: %v", err)
		}
		// Register rule wins; other registers use the default rule.
		res, _ = f.svc.ResolveRoute(f.ctx, f.cashier, f.est.ID, store.PrintDocPaymentReceipt, &regBar)
		if !res.Ready || res.Printer.ID != bar.ID || res.Copies != 1 || res.KickDrawer {
			t.Fatalf("register rule: %+v", res)
		}
		res, _ = f.svc.ResolveRoute(f.ctx, f.cashier, f.est.ID, store.PrintDocPaymentReceipt, &regBeach)
		if !res.Ready || res.Printer.ID != cassa.ID || res.Copies != 2 || !res.KickDrawer {
			t.Fatalf("default rule: %+v", res)
		}
		// Unreachable printer: not ready (live re-check after the cool-down).
		fp.Close()
		f.clock.Advance(time.Minute)
		res, _ = f.svc.ResolveRoute(f.ctx, f.cashier, f.est.ID, store.PrintDocCashZ, nil)
		if res.Ready || res.Reason != printing.ReasonPrinterUnreachable {
			t.Fatalf("unreachable: %+v", res)
		}
		// Server edition: never ready.
		srv := printing.NewService(printing.Deps{DB: db, Clock: f.clock})
		res, _ = srv.ResolveRoute(f.ctx, f.cashier, f.est.ID, store.PrintDocCashZ, nil)
		if res.Ready || res.Reason != printing.ReasonServerEdition || res.Printer == nil {
			t.Fatalf("server edition: %+v", res)
		}
		// Deleting a printer removes its routes.
		if err := f.svc.DeletePrinter(f.ctx, f.owner, f.est.ID, cassa.ID, nil); err != nil {
			t.Fatal(err)
		}
		left, _ := f.svc.GetRoutes(f.ctx, f.owner, f.est.ID)
		if len(left.Routes) != 1 || left.Routes[0].PrinterID != bar.ID {
			t.Fatalf("routes after delete: %+v", left.Routes)
		}
	})
}

// TestService_JobRoutingEndToEnd queues each document kind through the
// routing table and checks the bytes received by a fake TCP printer.
func TestService_JobRoutingEndToEnd(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pin := 2
		pr := f.printer(t, "Cassa", fp, func(in *printing.PrinterInput) { in.DrawerPin, in.PrintLogo = &pin, true })
		es := f.scope(t)
		now := f.clock.Now()
		cs := &store.CashSession{
			ID: store.NewID(), Register: "main", OpenedAt: now, OpeningFloat: 10000,
			ExpectedTotals: store.MethodTotals{"cash": 45000, "card": 12000}, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertCashSession(f.ctx, db, es, cs); err != nil {
			t.Fatal(err)
		}
		pm := &store.Payment{
			ID: store.NewID(), Provider: "manual", Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn,
			Amount: 3500, Status: "succeeded", Description: "Ombrellone A12", BusinessDate: calendar.New(2027, 7, 1),
			VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: 3500, Taxable: 2869, Tax: 631}}, Metadata: map[string]any{},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertPayment(f.ctx, db, es, pm); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.UpdateRoutes(f.ctx, f.owner, f.est.ID, nil, []printing.Route{
			{DocumentKind: store.PrintDocPaymentReceipt, PrinterID: pr.ID, KickDrawer: true},
			{DocumentKind: store.PrintDocCashZ, PrinterID: pr.ID},
			{DocumentKind: store.PrintDocDailyTicket, PrinterID: pr.ID, Copies: 2},
		}); err != nil {
			t.Fatal(err)
		}

		// No route for test pages through createPrintJob without printer_id.
		if _, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocTestPage}, ""); !errors.Is(err, printing.ErrRouteMissing) {
			t.Fatalf("route missing: %v", err)
		}
		if _, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocPaymentReceipt}, ""); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("missing document id: %v", err)
		}
		if _, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocPaymentReceipt, DocumentID: store.NewID()}, ""); !errors.Is(err, printing.ErrNotFound) {
			t.Fatalf("unknown payment: %v", err)
		}

		receipt, created, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocPaymentReceipt, DocumentID: pm.ID}, "key-receipt")
		if err != nil || !created || receipt.Status != store.PrintJobQueued {
			t.Fatalf("receipt: %+v %v", receipt, err)
		}
		again, created, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocPaymentReceipt, DocumentID: pm.ID}, "key-receipt")
		if err != nil || created || again.ID != receipt.ID {
			t.Fatalf("same key must return the same job: %+v %v", again, err)
		}
		// The key is bound to the request: another document with it is refused.
		if _, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocCashZ, DocumentID: cs.ID}, "key-receipt"); !errors.Is(err, printing.ErrIdempotencyKeyReused) {
			t.Fatalf("key reused for another document: %v", err)
		}
		z, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocCashZ, DocumentID: cs.ID}, "")
		if err != nil {
			t.Fatal(err)
		}
		ticket, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocDailyTicket, DocumentID: ticketID}, "key-ticket")
		if err != nil || ticket.Copies != 2 {
			t.Fatalf("ticket: %+v %v", ticket, err)
		}
		test, _, err := f.svc.TestPrint(f.ctx, f.cashier, f.est.ID, pr.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		if n := f.deliver(t); n != 4 {
			t.Fatalf("delivered %d, want 4", n)
		}
		jobs := fp.WaitJobs(4, 5*time.Second)
		if len(jobs) != 4 {
			t.Fatalf("printer received %d jobs", len(jobs))
		}
		find := func(sub string) []byte {
			for _, j := range jobs {
				if bytes.Contains(j, []byte(sub)) {
					return j
				}
			}
			t.Fatalf("no job contains %q", sub)
			return nil
		}
		r := find("RICEVUTA NON FISCALE")
		for _, want := range [][]byte{[]byte("Ombrellone A12"), {0xd5, ' ', '3', '5', ',', '0', '0'}, []byte("Contanti"), {escpos.ESC, 'p', 0, 25, 250}, {escpos.GS, 'V', 1}, {escpos.GS, 'v', '0'}} {
			if !bytes.Contains(r, want) {
				t.Errorf("receipt misses %s", escpos.Dump(want))
			}
		}
		zj := find("CHIUSURA DI CASSA")
		if !bytes.Contains(zj, []byte("NON CHIUSA")) || bytes.Contains(zj, []byte{escpos.ESC, 'p'}) {
			t.Error("open session: draft Z without drawer kick")
		}
		tk := find("BIGLIETTO 0042")
		if bytes.Count(tk, []byte{escpos.GS, 'V', 1}) != 2 || !bytes.Contains(tk, []byte("1P0ARN-7F3K")) {
			t.Error("ticket: 2 copies with the QR of the booking code")
		}
		if !bytes.Contains(find("STAMPA DI PROVA"), []byte("1P0ARENILE-TEST")) {
			t.Error("test page QR")
		}
		for _, id := range []string{receipt.ID, z.ID, ticket.ID, test.ID} {
			j, err := f.svc.GetJob(f.ctx, f.cashier, f.est.ID, id)
			if err != nil || j.Status != store.PrintJobPrinted || j.PrintedAt == nil || j.Attempts != 1 {
				t.Fatalf("job %s: %+v %v", id, j, err)
			}
		}
		got, _ := f.svc.GetPrinter(f.ctx, f.owner, f.est.ID, pr.ID)
		if got.Status != store.PrinterStatusReachable || got.LastSuccessAt == nil || len(got.LogoRaster) == 0 {
			t.Fatalf("printer after jobs: status %s logo %d", got.Status, len(got.LogoRaster))
		}
		if f.logos.calls != 1 {
			t.Fatalf("logo fetched %d times, want 1 (cached)", f.logos.calls)
		}
		page, err := f.svc.ListJobs(f.ctx, f.cashier, f.est.ID, store.PrintJobFilter{Status: store.PrintJobPrinted}, "", 3)
		if err != nil || len(page.Items) != 3 || page.NextCursor == "" || page.PrinterNames[pr.ID] != "Cassa" || page.Items[0].Payload != nil {
			t.Fatalf("list: %d items next %q names %v", len(page.Items), page.NextCursor, page.PrinterNames)
		}
		if _, err := f.svc.ListJobs(f.ctx, f.cashier, f.est.ID, store.PrintJobFilter{PrinterID: "not-a-uuid"}, "", 3); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("malformed printer filter: %v", err)
		}
		// Without a key every request is a new job: a deliberate reprint.
		r1, c1, err1 := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocPaymentReceipt, DocumentID: pm.ID}, "")
		r2, c2, err2 := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocPaymentReceipt, DocumentID: pm.ID}, "")
		if err1 != nil || err2 != nil || !c1 || !c2 || r1.ID == r2.ID {
			t.Fatalf("reprint without key: %v %v %v %v", c1, c2, err1, err2)
		}
	})
}

func TestService_RetryBackoffAndFailure(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pr := f.printer(t, "Cassa", fp)
		port := fp.Port()
		fp.Close()
		j, _, err := f.svc.TestPrint(f.ctx, f.cashier, f.est.ID, pr.ID, "test-1")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.RetryJob(f.ctx, f.cashier, f.est.ID, j.ID); !errors.Is(err, printing.ErrInvalidTransition) {
			t.Fatalf("retry of a queued job: %v", err)
		}
		waits := []time.Duration{5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute}
		for i := range 5 {
			if n := f.deliver(t); n != 0 {
				t.Fatalf("attempt %d printed", i+1)
			}
			cur, _ := f.svc.GetJob(f.ctx, f.cashier, f.est.ID, j.ID)
			if cur.Attempts != i+1 || cur.LastError == nil {
				t.Fatalf("attempt %d: %+v", i+1, cur)
			}
			if i < 4 {
				if cur.Status != store.PrintJobQueued || !cur.NextAttemptAt.Equal(f.clock.Now().Add(waits[i])) {
					t.Fatalf("attempt %d: status %s next %v", i+1, cur.Status, cur.NextAttemptAt)
				}
				if n := f.deliver(t); n != 0 {
					t.Fatal("delivered before the backoff")
				}
				f.clock.Advance(waits[i])
			} else if cur.Status != store.PrintJobFailed || cur.NextAttemptAt != nil {
				t.Fatalf("after max attempts: %+v", cur)
			}
		}
		prn, _ := f.svc.GetPrinter(f.ctx, f.owner, f.est.ID, pr.ID)
		if prn.Status != store.PrinterStatusUnreachable || prn.LastError == nil {
			t.Fatalf("printer: %+v", prn)
		}
		// Printer back on the same port: manual retry prints the same payload.
		back := listenOn(t, port)
		r, err := f.svc.RetryJob(f.ctx, f.cashier, f.est.ID, j.ID)
		if err != nil || r.Status != store.PrintJobQueued || r.Attempts != 0 {
			t.Fatalf("retry: %+v %v", r, err)
		}
		if n := f.deliver(t); n != 1 {
			t.Fatalf("retry delivered %d", n)
		}
		if js := back.WaitJobs(1, 5*time.Second); len(js) != 1 || !bytes.Contains(js[0], []byte("STAMPA DI PROVA")) {
			t.Fatal("retried job not received")
		}
	})
}

// listenOn restarts a fake printer on a given port (retrying briefly while
// the OS releases it).
func listenOn(t *testing.T, port int) *printertest.Printer {
	t.Helper()
	for range 50 {
		if p, err := printertest.StartOn(t, port); err == nil {
			return p
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("port %d not available", port)
	return nil
}

func TestService_ServerEditionAndDelete(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pr := f.printer(t, "Cassa", fp)
		srv := printing.NewService(printing.Deps{DB: db, Clock: f.clock})
		if _, err := srv.CheckPrinter(f.ctx, f.cashier, f.est.ID, pr.ID); !errors.Is(err, printing.ErrUnavailable) {
			t.Fatalf("check: %v", err)
		}
		if _, _, err := srv.TestPrint(f.ctx, f.cashier, f.est.ID, pr.ID, ""); !errors.Is(err, printing.ErrUnavailable) {
			t.Fatalf("test print: %v", err)
		}
		if _, _, err := srv.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocTestPage, PrinterID: &pr.ID}, ""); !errors.Is(err, printing.ErrUnavailable) {
			t.Fatalf("enqueue: %v", err)
		}
		if len(srv.Jobs()) != 0 || len(f.svc.Jobs()) != 2 {
			t.Fatal("delivery and retention jobs only on the desktop edition")
		}
		// No printers on the server edition (nothing could print there); the
		// existing configuration stays readable and removable.
		if _, err := srv.CreatePrinter(f.ctx, f.owner, f.est.ID, printing.PrinterInput{Name: "Bar", Host: "10.0.0.20", PaperWidthMM: 58}); !errors.Is(err, printing.ErrUnavailable) {
			t.Fatalf("server create: %v", err)
		}
		if _, err := srv.UpdatePrinter(f.ctx, f.owner, f.est.ID, pr.ID, nil, printing.PrinterInput{Name: "Bar", Host: "10.0.0.20", PaperWidthMM: 58}); !errors.Is(err, printing.ErrUnavailable) {
			t.Fatalf("server update: %v", err)
		}
		if got, err := srv.ListPrinters(f.ctx, f.cashier, f.est.ID, true); err != nil || len(got) != 1 {
			t.Fatalf("server list: %d %v", len(got), err)
		}
		// Check on desktop.
		res, err := f.svc.CheckPrinter(f.ctx, f.cashier, f.est.ID, pr.ID)
		if err != nil || res.Status != store.PrinterStatusReachable {
			t.Fatalf("check: %+v %v", res, err)
		}
		// Delete: queued jobs become cancelled and keep their history.
		j, _, err := f.svc.TestPrint(f.ctx, f.cashier, f.est.ID, pr.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := f.svc.DeletePrinter(f.ctx, f.owner, f.est.ID, pr.ID, nil); err != nil {
			t.Fatal(err)
		}
		got, err := f.svc.GetJob(f.ctx, f.cashier, f.est.ID, j.ID)
		if err != nil || got.Status != store.PrintJobCancelled || got.PrinterID != nil {
			t.Fatalf("job after delete: %+v %v", got, err)
		}
		if n := f.deliver(t); n != 0 || len(fp.Jobs()) != 0 {
			t.Fatal("cancelled job printed")
		}
		// Inactive printer refuses jobs.
		inactive := false
		pr2, err := f.svc.CreatePrinter(f.ctx, f.owner, f.est.ID, printing.PrinterInput{Name: "Off", Host: fp.Host(), Port: fp.Port(), PaperWidthMM: 80, Active: &inactive})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.svc.TestPrint(f.ctx, f.cashier, f.est.ID, pr2.ID, ""); !errors.Is(err, printing.ErrPrinterInactive) {
			t.Fatalf("inactive: %v", err)
		}
	})
}

// TestConcurrency_PrintJobOnce: 100 parallel enqueues of the same document
// create one job (unique idempotency key) and 100 parallel deliveries print
// it exactly once (conditional claim with a lease).
func TestConcurrency_PrintJobOnce(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pr := f.printer(t, "Cassa", fp)
		const n = 100
		var wg sync.WaitGroup
		ids := make([]string, n)
		errs := make([]error, n)
		created := make([]bool, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				j, c, err := f.svc.TestPrint(f.ctx, f.cashier, f.est.ID, pr.ID, "same-key")
				errs[i], created[i] = err, c
				if j != nil {
					ids[i] = j.ID
				}
			}()
		}
		wg.Wait()
		nCreated := 0
		for i := range n {
			if errs[i] != nil {
				t.Fatalf("enqueue %d: %v", i, errs[i])
			}
			if ids[i] != ids[0] {
				t.Fatalf("enqueue %d returned job %s, want %s", i, ids[i], ids[0])
			}
			if created[i] {
				nCreated++
			}
		}
		if nCreated != 1 {
			t.Fatalf("%d enqueues created a job, want 1", nCreated)
		}
		rows, err := db.NewSelect().Model((*store.PrintJob)(nil)).Where("establishment_id = ?", f.est.ID).Count(f.ctx)
		if err != nil || rows != 1 {
			t.Fatalf("rows = %d, %v", rows, err)
		}
		var printed sync.WaitGroup
		total := make(chan int, n)
		for range n {
			printed.Add(1)
			go func() {
				defer printed.Done()
				c, err := f.svc.DeliverDue(f.ctx, 10)
				if err != nil {
					t.Error(err)
				}
				total <- c
			}()
		}
		printed.Wait()
		close(total)
		sum := 0
		for c := range total {
			sum += c
		}
		time.Sleep(100 * time.Millisecond)
		if sum != 1 || len(fp.Jobs()) != 1 {
			t.Fatalf("printed %d times, printer received %d jobs", sum, len(fp.Jobs()))
		}
	})
}

func ptr[T any](v T) *T { return &v }

// TestService_DraftZLiveTotals opens a session through the cash service,
// records a card and a cash payment and a cash movement, then prints the
// draft Z: "Atteso per metodo" must carry the live totals, not the ones
// frozen at open time (opening float only).
func TestService_DraftZLiveTotals(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pr := f.printer(t, "Cassa", fp)
		cashSvc := cash.NewService(cash.Deps{DB: db, Clock: f.clock, Audit: audit.NewWriter(f.clock), Outbox: event.NewOutbox(f.clock)})
		cs, err := cashSvc.Open(f.ctx, f.cashier, "127.0.0.1", f.est.ID, cash.OpenInput{OpeningFloat: 10000})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cashSvc.AddMovement(f.ctx, f.cashier, "127.0.0.1", f.est.ID, cs.ID, cash.MovementInput{Kind: store.CashMovementIn, Amount: 2000, Reason: "Resto dal bar"}); err != nil {
			t.Fatal(err)
		}
		es := f.scope(t)
		for _, p := range []struct {
			method string
			amount money.Cents
		}{{store.PaymentMethodCard, 12000}, {store.PaymentMethodCash, 3500}} {
			taxable, tax := money.VATSplit(p.amount, 2200)
			pm := &store.Payment{
				ID: store.NewID(), Provider: "manual", Method: p.method, Direction: store.PaymentDirectionIn, Amount: p.amount,
				Status: store.PaymentSucceeded, CashSessionID: &cs.ID, BusinessDate: calendar.New(2027, 7, 1), Metadata: map[string]any{},
				VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: p.amount, Taxable: taxable, Tax: tax}},
				CreatedAt:    f.clock.Now(), UpdatedAt: f.clock.Now(),
			}
			if err := store.InsertPayment(f.ctx, db, es, pm); err != nil {
				t.Fatal(err)
			}
		}
		// The browser Z report and the slip must agree.
		detail, err := cashSvc.Get(f.ctx, f.cashier, f.est.ID, cs.ID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Summary.Expected[store.PaymentMethodCash] != 15500 || detail.Summary.Expected[store.PaymentMethodCard] != 12000 {
			t.Fatalf("cash detail: %+v", detail.Summary.Expected)
		}
		if _, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocCashZ, DocumentID: cs.ID, PrinterID: &pr.ID}, ""); err != nil {
			t.Fatal(err)
		}
		if n := f.deliver(t); n != 1 {
			t.Fatalf("delivered %d", n)
		}
		jobs := fp.WaitJobs(1, 5*time.Second)
		if len(jobs) != 1 {
			t.Fatalf("printer received %d jobs", len(jobs))
		}
		slip := jobs[0]
		euro := func(s string) []byte { return append([]byte{0xd5, ' '}, []byte(s)...) }
		for _, want := range [][]byte{[]byte("NON CHIUSA"), []byte("Contanti"), euro("155,00"), []byte("Carta"), euro("120,00"), []byte("Totale atteso"), euro("275,00"), []byte("Resto dal bar"), euro("20,00")} {
			if !bytes.Contains(slip, want) {
				t.Errorf("draft Z misses %q", want)
			}
		}
		if bytes.Contains(slip, euro("100,00")) && !bytes.Contains(slip, []byte("Fondo cassa")) {
			t.Error("opening float printed as the expected cash")
		}
	})
}

// TestService_LeaseReclaimAndRetention: a job whose lease expired mid-send
// (worker crash) is reclaimed as a new attempt and resent without the
// drawer kick (at-least-once delivery, INV-86); payloads of finished jobs
// are nulled after PayloadRetention.
func TestService_LeaseReclaimAndRetention(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pin := 2
		pr := f.printer(t, "Cassa", fp, func(in *printing.PrinterInput) { in.DrawerPin = &pin })
		es := f.scope(t)
		now := f.clock.Now()
		pm := &store.Payment{
			ID: store.NewID(), Provider: "manual", Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn,
			Amount: 1000, Status: "succeeded", Description: "Lettino", BusinessDate: calendar.New(2027, 7, 1), Metadata: map[string]any{},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertPayment(f.ctx, db, es, pm); err != nil {
			t.Fatal(err)
		}
		kick := true
		j, _, err := f.svc.Enqueue(f.ctx, f.cashier, f.est.ID, printing.JobInput{DocumentKind: store.PrintDocPaymentReceipt, DocumentID: pm.ID, PrinterID: &pr.ID, KickDrawer: &kick}, "")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(j.Payload, []byte{escpos.ESC, 'p'}) {
			t.Fatal("receipt rendered without the drawer kick")
		}
		// Simulate a worker that claimed the job and died: lease in the past.
		expired := now.Add(-time.Minute)
		if _, err := db.NewUpdate().Model((*store.PrintJob)(nil)).Set("status = ?", store.PrintJobPrinting).Set("locked_until = ?", expired).
			Where("id = ?", j.ID).Exec(f.ctx); err != nil {
			t.Fatal(err)
		}
		if n := f.deliver(t); n != 1 {
			t.Fatalf("reclaimed delivery printed %d", n)
		}
		got, _ := f.svc.GetJob(f.ctx, f.cashier, f.est.ID, j.ID)
		if got.Status != store.PrintJobPrinted || got.Attempts != 2 {
			t.Fatalf("reclaimed job: status %s attempts %d", got.Status, got.Attempts)
		}
		jobs := fp.WaitJobs(1, 5*time.Second)
		if len(jobs) != 1 || !bytes.Contains(jobs[0], []byte("RICEVUTA")) || bytes.Contains(jobs[0], []byte{escpos.ESC, 'p'}) {
			t.Fatal("the resent slip must print without opening the drawer again")
		}

		// Retention: the payload goes after PayloadRetention, the row stays.
		if n, err := f.svc.PurgePayloads(f.ctx); err != nil || n != 0 {
			t.Fatalf("purge too early: %d %v", n, err)
		}
		f.clock.Advance(printing.PayloadRetention + time.Hour)
		if n, err := f.svc.PurgePayloads(f.ctx); err != nil || n != 1 {
			t.Fatalf("purge: %d %v", n, err)
		}
		got, err = f.svc.GetJob(f.ctx, f.cashier, f.est.ID, j.ID)
		if err != nil || got.Status != store.PrintJobPrinted || got.Payload != nil {
			t.Fatalf("purged job: %+v %v", got, err)
		}
	})
}

// TestService_LogoFailureCached: a logo that cannot be fetched is not
// retried on every print (negative cache), and prints resume with it once
// the cache expires.
func TestService_LogoFailureCached(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db, true)
		fp := printertest.Start(t)
		pr := f.printer(t, "Cassa", fp, func(in *printing.PrinterInput) { in.PrintLogo = true })
		f.logos.fail = true
		for range 3 {
			if _, _, err := f.svc.TestPrint(f.ctx, f.cashier, f.est.ID, pr.ID, ""); err != nil {
				t.Fatal(err)
			}
		}
		if f.logos.calls != 1 {
			t.Fatalf("logo fetched %d times while failing, want 1", f.logos.calls)
		}
		f.logos.fail = false
		f.clock.Advance(11 * time.Minute)
		if _, _, err := f.svc.TestPrint(f.ctx, f.cashier, f.est.ID, pr.ID, ""); err != nil {
			t.Fatal(err)
		}
		got, _ := f.svc.GetPrinter(f.ctx, f.owner, f.est.ID, pr.ID)
		if f.logos.calls != 2 || len(got.LogoRaster) == 0 {
			t.Fatalf("after the cache expired: calls %d, raster %d", f.logos.calls, len(got.LogoRaster))
		}
	})
}
