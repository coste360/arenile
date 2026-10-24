//go:build integration

package importer_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/importer"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type env struct {
	*bookingtest.Fixture
	svc       *importer.Service
	customers *customer.Service
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	f := bookingtest.New(t, db)
	customers := customer.NewService(customer.Deps{DB: db, Clock: f.Clock})
	passes := pass.NewService(pass.Deps{DB: db, Clock: f.Clock, Bookings: f.Svc, Credits: customers})
	svc := importer.NewService(importer.Deps{
		DB: db, Clock: f.Clock,
		Customers: func(db bun.IDB) importer.CustomerWriter { return customers.WithDB(db) },
		Passes:    func(db bun.IDB) importer.PassWriter { return passes.WithDB(db) },
	})
	return &env{Fixture: f, svc: svc, customers: customers}
}

func (e *env) upload(kind, body string) *store.ImportJob {
	e.T.Helper()
	j, err := e.svc.Create(e.Ctx, e.Owner, e.Est.ID, kind, "clienti.csv", []byte(body))
	if err != nil {
		e.T.Fatalf("upload: %v", err)
	}
	return j
}

func (e *env) preview(j *store.ImportJob, opts importer.Options) (*store.ImportJob, importer.Report) {
	e.T.Helper()
	out, err := e.svc.Preview(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version, opts)
	if err != nil {
		e.T.Fatalf("preview: %v", err)
	}
	return out, report(e.T, out)
}

func report(t testing.TB, j *store.ImportJob) importer.Report {
	t.Helper()
	var r importer.Report
	if err := remarshal(j.Report, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func hasIssue(issues []importer.Issue, row int, column, code string) bool {
	for _, i := range issues {
		if i.Row == row && i.Column == column && i.Code == code {
			return true
		}
	}
	return false
}

// TestImport_Customers_CreateSkipUpdate covers the customer flow: preview
// writes nothing, apply creates the new rows, skips the existing ones
// (matched by email), is idempotent and refuses the same file twice;
// update_existing merges only the given columns.
func TestImport_Customers_CreateSkipUpdate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		existing, err := e.customers.Create(e.Ctx, e.Owner, "", e.Est.ID, customer.Input{LastName: "Rossi", FirstName: "Mario", Email: "mario@example.org", Notes: "storico"}, false)
		if err != nil {
			t.Fatal(err)
		}
		csv := "\xef\xbb\xbfCognome;Nome;E-mail;Cellulare;Consenso marketing;Etichette;Colonna strana\n" +
			"Rossi;Mario;MARIO@example.org;;sì;vip;x\n" +
			"Bianchi;Anna;anna@example.org;333 1234567;no;;\n" +
			"Verdi;Luca;;+39 347 7654321;;famiglia|abbonato;\n" +
			";;;;;;\n"
		j := e.upload(store.ImportKindCustomers, csv)
		if j.Status != store.ImportStatusUploaded || report(t, j).Rows != 3 {
			t.Fatalf("uploaded job %+v report %+v", j, report(t, j))
		}
		if !hasIssue(report(t, j).Warnings, 1, "Colonna strana", "unknown_column") {
			t.Fatalf("unknown column warning missing: %+v", report(t, j).Warnings)
		}

		j, rep := e.preview(j, importer.Options{})
		if rep.ToCreate != 2 || rep.ToSkip != 1 || rep.ToUpdate != 0 || rep.Invalid != 0 || rep.Valid != 3 {
			t.Fatalf("preview report %+v", rep)
		}
		if !hasIssue(rep.Warnings, 4, "email", "no_match_key") {
			t.Fatalf("no_match_key warning missing: %+v", rep.Warnings)
		}
		if n := e.Count((*store.Customer)(nil), ""); n != 1 {
			t.Fatalf("preview wrote customers: %d", n)
		}
		if j.Status != store.ImportStatusValidated {
			t.Fatalf("status %s", j.Status)
		}

		applied, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		rep = report(t, applied)
		if applied.Status != store.ImportStatusApplied || applied.Content != nil || rep.Created == nil || *rep.Created != 2 || *rep.Skipped != 1 {
			t.Fatalf("applied %+v report %+v", applied, rep)
		}
		if n := e.Count((*store.Customer)(nil), ""); n != 3 {
			t.Fatalf("customers after apply: %d", n)
		}
		var anna store.Customer
		if err := db.NewSelect().Model(&anna).Where("email = ?", "anna@example.org").Scan(e.Ctx); err != nil {
			t.Fatal(err)
		}
		if anna.MarketingConsent != nil || anna.Phone == nil || *anna.Phone != "+393331234567" {
			t.Fatalf("anna %+v", anna)
		}
		var luca store.Customer
		if err := db.NewSelect().Model(&luca).Where("last_name = ?", "Verdi").Scan(e.Ctx); err != nil {
			t.Fatal(err)
		}
		if len(luca.Tags) != 2 || luca.MarketingConsent != nil {
			t.Fatalf("luca %+v", luca)
		}

		// Idempotent apply; the same file again is refused.
		again, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, j.ID, nil)
		if err != nil || again.Version != applied.Version {
			t.Fatalf("second apply: %v %+v", err, again)
		}
		dup := e.upload(store.ImportKindCustomers, csv)
		dup, _ = e.preview(dup, importer.Options{})
		if _, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, dup.ID, &dup.Version); !errors.Is(err, importer.ErrAlreadyApplied) {
			t.Fatalf("same file: %v", err)
		}

		// update_existing: only the given columns change, consent given.
		upd := e.upload(store.ImportKindCustomers, "cognome,nome,email,consenso_marketing,data_consenso\nRossi,Marco,mario@example.org,si,2027-05-02\n")
		upd, rep = e.preview(upd, importer.Options{UpdateExisting: true})
		if rep.ToUpdate != 1 {
			t.Fatalf("update preview %+v", rep)
		}
		if _, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, upd.ID, &upd.Version); err != nil {
			t.Fatal(err)
		}
		got, err := store.GetCustomer(e.Ctx, db, scope(t, e), existing.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.FirstName != "Marco" || got.Notes != "storico" || got.MarketingConsent == nil ||
			got.MarketingConsent.Source != "import" || got.MarketingConsent.GivenAt.Format("2006-01-02") != "2027-05-02" {
			t.Fatalf("updated customer %+v consent %+v", got, got.MarketingConsent)
		}
	})
}

// TestImport_Customers_ValidationReport: invalid rows are reported by row,
// column and code; apply refuses the file and writes nothing.
func TestImport_Customers_ValidationReport(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		if _, err := e.svc.Create(e.Ctx, e.Owner, e.Est.ID, store.ImportKindCustomers, "x.csv", []byte("nome;email\nMario;a@b.it\n")); !isInvalid(err, "missing_column") {
			t.Fatalf("missing column: %v", err)
		}
		csv := "cognome;email;telefono;lingua;consenso_marketing\n" +
			"Rossi;non-una-email;;it;\n" +
			"Bianchi;b@example.org;12;;\n" +
			";c@example.org;;;\n" +
			"Neri;b@example.org;;es;forse\n" +
			"Gialli;g@example.org;;;\n"
		j := e.upload(store.ImportKindCustomers, csv)
		j, rep := e.preview(j, importer.Options{})
		if rep.Invalid != 4 || rep.Valid != 1 || rep.ToCreate != 1 {
			t.Fatalf("report %+v", rep)
		}
		for _, want := range []struct {
			row       int
			col, code string
		}{
			{2, "email", "invalid_email"},
			{3, "telefono", "invalid_phone"},
			{4, "cognome", "required"},
			{5, "email", "duplicate_in_file"},
			{5, "lingua", "invalid_locale"},
			{5, "consenso_marketing", "invalid_boolean"},
		} {
			if !hasIssue(rep.Errors, want.row, want.col, want.code) {
				t.Errorf("missing %+v in %+v", want, rep.Errors)
			}
		}
		_, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version)
		if !isInvalid(err, "invalid_rows") {
			t.Fatalf("apply: %v", err)
		}
		if n := e.Count((*store.Customer)(nil), ""); n != 0 {
			t.Fatalf("customers written: %d", n)
		}
		got, err := e.svc.Get(e.Ctx, e.Owner, e.Est.ID, j.ID)
		if err != nil || got.Status != store.ImportStatusFailed {
			t.Fatalf("job after failed apply: %v %+v", err, got)
		}
		// Permission: establishment.configure.
		if _, err := e.svc.Get(e.Ctx, e.Cashier, e.Est.ID, j.ID); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier: %v", err)
		}
		if _, err := e.svc.Get(e.Ctx, e.Outsider, e.Est.ID, j.ID); err == nil {
			t.Fatal("outsider read the import")
		}
	})
}

// TestImport_SeasonPasses_RollbackAndIdempotence: a row refused while
// applying (spot already booked) rolls back the whole file, customers
// included; a valid file creates the passes (one customer for two rows)
// and importing it again skips them.
func TestImport_SeasonPasses_RollbackAndIdempotence(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.Base.AllowedFormulas = append(e.Base.AllowedFormulas, "season")
		if _, err := db.NewUpdate().Model(e.Base).Column("allowed_formulas").WherePK().Exec(e.Ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Pricing.CreateFixedPrice(e.Ctx, e.Owner, e.Est.ID, pricingconfig.FixedPriceInput{
			SeasonID: e.Season.ID, PackID: e.Base.ID, Formula: pricing.FormulaSeason, PriceCategory: e.SeatZone.PriceCategory, Amount: 90000,
		}); err != nil {
			t.Fatal(err)
		}
		// A2 is taken on 2027-07-01 by a daily booking.
		e.MustCreate(e.Input(e.Spots[1], "2027-07-01", "2027-07-01"))
		customersBefore := e.Count((*store.Customer)(nil), "")
		opts := importer.Options{SeasonID: e.Season.ID, PackID: e.Base.ID}

		bad := e.upload(store.ImportKindSeasonPasses, "cognome;nome;email;posto;fascia\n"+
			"Blu;Ada;ada@example.org;A1;giornata\n"+
			"Blu;Ada;ada@example.org;a2;\n"+
			"Rosa;Ivo;ivo@example.org;Z9;\n")
		bad, rep := e.preview(bad, opts)
		if rep.ToCreate != 1 || rep.Invalid != 2 || !hasIssue(rep.Errors, 3, "posto", "spot_unavailable") || !hasIssue(rep.Errors, 4, "posto", "unknown_spot") {
			t.Fatalf("preview %+v", rep)
		}
		if _, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, bad.ID, &bad.Version); !isInvalid(err, "rows_refused") {
			t.Fatalf("apply bad: %v", err)
		}
		if n := e.Count((*store.Booking)(nil), "is_season_pass = ?", true); n != 0 {
			t.Fatalf("passes after rollback: %d", n)
		}
		if n := e.Count((*store.Customer)(nil), ""); n != customersBefore {
			t.Fatalf("customers after rollback: %d, want %d", n, customersBefore)
		}

		good := "cognome;nome;email;posto;dal;al;note\n" +
			"Blu;Ada;ada@example.org;A1;;;fila davanti\n" +
			"Blu;Ada;ada@example.org;A3;01/07/2027;31/07/2027;\n"
		j := e.upload(store.ImportKindSeasonPasses, good)
		j, rep = e.preview(j, opts)
		if rep.ToCreate != 2 || rep.Invalid != 0 {
			t.Fatalf("good preview %+v", rep)
		}
		applied, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version)
		if err != nil {
			t.Fatalf("apply good: %v", err)
		}
		if r := report(t, applied); *r.Created != 2 {
			t.Fatalf("applied %+v", r)
		}
		if n := e.Count((*store.Booking)(nil), "is_season_pass = ?", true); n != 2 {
			t.Fatalf("passes: %d", n)
		}
		if n := e.Count((*store.Customer)(nil), "email = ?", "ada@example.org"); n != 1 {
			t.Fatalf("ada customers: %d", n)
		}
		if n := e.Count((*store.Booking)(nil), "is_season_pass = ? AND notes_internal = ?", true, "fila davanti"); n != 1 {
			t.Fatalf("pass notes: %d", n)
		}

		// Same rows in another file (different bytes): skipped.
		j2 := e.upload(store.ImportKindSeasonPasses, good+"\n")
		_, rep = e.preview(j2, opts)
		if rep.ToSkip != 2 || rep.ToCreate != 0 {
			t.Fatalf("re-import preview %+v", rep)
		}
	})
}

// TestImport_RetentionAndDiscard: discard clears the content; the
// retention job clears what is older than 7 days.
func TestImport_RetentionAndDiscard(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		a := e.upload(store.ImportKindCustomers, "cognome\nRossi\n")
		b := e.upload(store.ImportKindCustomers, "cognome\nBianchi\n")
		if err := e.svc.Discard(e.Ctx, e.Owner, "", e.Est.ID, a.ID); err != nil {
			t.Fatal(err)
		}
		got, _ := e.svc.Get(e.Ctx, e.Owner, e.Est.ID, a.ID)
		if got.Status != store.ImportStatusDiscarded || got.Content != nil {
			t.Fatalf("discarded %+v", got)
		}
		e.Clock.Advance(8 * 24 * time.Hour)
		n, err := e.svc.ClearExpired(e.Ctx)
		if err != nil || n != 1 {
			t.Fatalf("retention: %d %v", n, err)
		}
		got, _ = e.svc.Get(e.Ctx, e.Owner, e.Est.ID, b.ID)
		if got.Status != store.ImportStatusDiscarded || got.Content != nil {
			t.Fatalf("expired %+v", got)
		}
		if _, err := e.svc.Preview(e.Ctx, e.Owner, "", e.Est.ID, b.ID, nil, importer.Options{}); err == nil {
			t.Fatal("preview of a discarded import")
		}
		page, err := e.svc.List(e.Ctx, e.Owner, e.Est.ID, "", 1)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != b.ID || page.NextCursor == "" || page.Items[0].Content != nil {
			t.Fatalf("list %+v %v", page, err)
		}
	})
}

func isInvalid(err error, code string) bool {
	var inv *importer.InvalidError
	return errors.As(err, &inv) && inv.Code == code
}

func scope(t *testing.T, e *env) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(e.Ctx, e.DB, e.Est.ID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func remarshal(src map[string]any, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// TestImport_SeasonPasses_ApprovalPackRefused (INV-78): a pack with
// requires_approval cannot be imported, neither as the default pack of the
// options (validation error) nor from the pack column (row refused with
// pack_requires_approval): an imported pass would otherwise wait for an
// approval nobody sees and expire with its hold.
func TestImport_SeasonPasses_ApprovalPackRefused(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		for _, pk := range []*store.Pack{e.Base, e.VIP} {
			pk.AllowedFormulas = append(pk.AllowedFormulas, "season")
			if _, err := db.NewUpdate().Model(pk).Column("allowed_formulas").WherePK().Exec(e.Ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Pricing.CreateFixedPrice(e.Ctx, e.Owner, e.Est.ID, pricingconfig.FixedPriceInput{
				SeasonID: e.Season.ID, PackID: pk.ID, Formula: pricing.FormulaSeason, PriceCategory: e.SeatZone.PriceCategory, Amount: 90000,
			}); err != nil {
				t.Fatal(err)
			}
		}
		j := e.upload(store.ImportKindSeasonPasses, "cognome;email;posto\nBlu;ada@example.org;A1\n")
		_, err := e.svc.Preview(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version, importer.Options{SeasonID: e.Season.ID, PackID: e.VIP.ID})
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("approval pack as default: %v", err)
		}
		j2 := e.upload(store.ImportKindSeasonPasses, "cognome;email;posto;pacchetto\nBlu;ada@example.org;A1;"+e.VIP.Code+"\nRosa;ivo@example.org;A2;"+e.Base.Code+"\n")
		_, rep := e.preview(j2, importer.Options{SeasonID: e.Season.ID})
		if rep.ToCreate != 1 || rep.Invalid != 1 || !hasIssue(rep.Errors, 2, "pacchetto", "pack_requires_approval") {
			t.Fatalf("preview %+v", rep)
		}
	})
}

// TestImport_TestModeBlocksAndFailedApplyClearsContent: no preview or
// apply while test mode is on (the rows would be test rows, deleted by the
// next purge); the uploaded content of a failed apply is cleared at once
// (personal data, spec 23).
func TestImport_TestModeBlocksAndFailedApplyClearsContent(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		es := scope(t, e)
		j := e.upload(store.ImportKindCustomers, "cognome;email\nRossi;rossi@example.org\n")
		row, err := store.EnsureTestModeSettings(e.Ctx, db, es, e.Clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		row.Enabled = true
		if ok, err := store.UpdateTestModeSettingsVersioned(e.Ctx, db, es, row, row.Version); err != nil || !ok {
			t.Fatalf("enable test mode: %v %v", ok, err)
		}
		if _, err := e.svc.Preview(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version, importer.Options{}); !errors.Is(err, importer.ErrTestModeOn) {
			t.Fatalf("preview in test mode: %v", err)
		}
		row.Enabled = false
		if ok, err := store.UpdateTestModeSettingsVersioned(e.Ctx, db, es, row, row.Version); err != nil || !ok {
			t.Fatalf("disable test mode: %v %v", ok, err)
		}
		j, _ = e.preview(j, importer.Options{})
		row.Enabled = true
		if ok, err := store.UpdateTestModeSettingsVersioned(e.Ctx, db, es, row, row.Version); err != nil || !ok {
			t.Fatalf("enable test mode: %v %v", ok, err)
		}
		if _, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version); !errors.Is(err, importer.ErrTestModeOn) {
			t.Fatalf("apply in test mode: %v", err)
		}
		if n := e.Count((*store.Customer)(nil), "email = ?", "rossi@example.org"); n != 0 {
			t.Fatalf("customers imported in test mode: %d", n)
		}
		row.Enabled = false
		if ok, err := store.UpdateTestModeSettingsVersioned(e.Ctx, db, es, row, row.Version); err != nil || !ok {
			t.Fatalf("disable test mode: %v %v", ok, err)
		}

		// A row refused at apply time: failed, content gone.
		e.Base.AllowedFormulas = append(e.Base.AllowedFormulas, "season")
		if _, err := db.NewUpdate().Model(e.Base).Column("allowed_formulas").WherePK().Exec(e.Ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Pricing.CreateFixedPrice(e.Ctx, e.Owner, e.Est.ID, pricingconfig.FixedPriceInput{
			SeasonID: e.Season.ID, PackID: e.Base.ID, Formula: pricing.FormulaSeason, PriceCategory: e.SeatZone.PriceCategory, Amount: 90000,
		}); err != nil {
			t.Fatal(err)
		}
		opts := importer.Options{SeasonID: e.Season.ID, PackID: e.Base.ID}
		p := e.upload(store.ImportKindSeasonPasses, "cognome;email;posto\nBlu;ada@example.org;A1\n")
		p, _ = e.preview(p, opts)
		e.MustCreate(e.Input(e.Spots[0], "2027-07-01", "2027-07-01")) // A1 taken after the preview
		if _, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, p.ID, &p.Version); !isInvalid(err, "rows_refused") {
			t.Fatalf("apply: %v", err)
		}
		got, _ := e.svc.Get(e.Ctx, e.Owner, e.Est.ID, p.ID)
		if got.Status != store.ImportStatusFailed || got.Content != nil {
			t.Fatalf("failed import keeps its content: %+v", got)
		}
	})
}

// TestConcurrency_ImportApplySameFile (INV-79): the same file uploaded as
// 100 jobs and applied in parallel is applied exactly once; every loser
// gets ErrAlreadyApplied (the unique index of applied files) and writes
// nothing.
func TestConcurrency_ImportApplySameFile(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		const parallel = 100
		jobs := make([]*store.ImportJob, parallel)
		for i := range jobs {
			j := e.upload(store.ImportKindCustomers, "cognome;email\nRossi;rossi@example.org\nBianchi;bianchi@example.org\n")
			jobs[i], _ = e.preview(j, importer.Options{})
		}
		var (
			wg               sync.WaitGroup
			mu               sync.Mutex
			applied, already int
			other            []error
			start            = make(chan struct{})
		)
		for _, j := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := e.svc.Apply(e.Ctx, e.Owner, "", e.Est.ID, j.ID, &j.Version)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					applied++
				case errors.Is(err, importer.ErrAlreadyApplied):
					already++
				default:
					other = append(other, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		if applied != 1 || already != parallel-1 {
			t.Fatalf("applied %d, already applied %d", applied, already)
		}
		if n := e.Count((*store.Customer)(nil), "email IN (?)", bun.List([]string{"rossi@example.org", "bianchi@example.org"})); n != 2 {
			t.Fatalf("customers %d, want 2", n)
		}
		if n := e.Count((*store.ImportJob)(nil), "status = ?", store.ImportStatusApplied); n != 1 {
			t.Fatalf("applied jobs %d", n)
		}
	})
}
