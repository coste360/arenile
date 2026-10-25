//go:build integration

package acceptance

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// resetSurvivors are the establishment-scoped tables that may still have
// rows after a reset "everything" (the rest of the list of
// internal/store/reset_integration_test.go, resetKeptTables, has no rows in
// this scenario).
var resetSurvivors = map[string]bool{
	"audit_log":      true, // insert only (INV-30), setup.reset included
	"outbox_events":  true, // committed events, delivered at least once (INV-32)
	"cash_registers": true, // the default register survives (INV-73)
}

// TestAcceptance_SetupResetInvariants checks the invariants around a reset
// "everything" of the demo establishment, on both dialects:
//
//  1. tenancy (INV-26): another organization's establishment with its own
//     demo data cannot be reset by this owner and is untouched, row by row,
//     by the reset, by the stale events and by the jobs that follow;
//  2. nothing is left behind: after dispatching the events committed before
//     the reset (outbox not drained on purpose) and running every job, the
//     only establishment rows left are resetSurvivors, so no allocation,
//     pool counter, payment or hold outlives its booking (INV-01..INV-03,
//     INV-20, INV-22 hold trivially and no job recreates rows);
//  3. the establishment starts over cleanly: the demo seeded again on the
//     reset establishment satisfies the same invariants as a fresh demo
//     (checkInvariants: INV-01, 02, 03, 05, 06, 10, 18, 20, 22, 24, 25).
func TestAcceptance_SetupResetInvariants(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ctx := context.Background()
		owner, err := store.GetUserByEmail(ctx, e.db, ownerEmail)
		if err != nil {
			t.Fatal(err)
		}
		principal := func(orgID string) *auth.Principal {
			return &auth.Principal{
				Kind: auth.PrincipalUser, UserID: owner.ID, Name: owner.Name,
				Grants: []auth.Grant{{OrganizationID: orgID, Role: auth.RoleOwner}},
			}
		}

		// Another organization with the demo configuration and operations.
		now := e.clock.Now()
		org2 := &store.Organization{ID: store.NewID(), Name: "Altro Lido Srl", CreatedAt: now, UpdatedAt: now}
		if err := store.InsertOrganization(ctx, e.db, org2); err != nil {
			t.Fatal(err)
		}
		est2 := store.NewEstablishmentDefaults(org2.ID, "altro-lido", "Altro Lido", now)
		est2.BookingEnabled = true
		if err := store.InsertEstablishment(ctx, e.db, est2); err != nil {
			t.Fatal(err)
		}
		if res, err := e.app.Setup.SeedDemo(ctx, est2.ID); err != nil || !res.Applied {
			t.Fatalf("demo configuration of the other establishment: %+v %v", res, err)
		}
		if res, err := e.app.Setup.SeedDemoOperations(ctx, est2.ID, e.app.DemoOperations(principal(org2.ID))); err != nil || !res.Applied {
			t.Fatalf("demo operations of the other establishment: %+v %v", res, err)
		}
		e.runJobs()

		// Operations on the establishment to reset. Its events are put back
		// as pending (test-side write): the reset commits before the
		// dispatcher passes, which then delivers events about deleted rows
		// (at least once, INV-32).
		if res, err := e.app.Setup.SeedDemoOperations(ctx, e.estID, e.app.DemoOperations(principal(e.es.OrganizationID()))); err != nil || !res.Applied {
			t.Fatalf("demo operations: %+v %v", res, err)
		}
		res, err := e.db.NewUpdate().TableExpr("outbox_events").Set("dispatched_at = NULL").Where("establishment_id = ?", e.estID).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			t.Fatal("no events before the reset")
		}

		tables := scopedTables(ctx, t, e)
		counts := func(estID string) map[string]int {
			out := map[string]int{}
			for _, tb := range tables {
				n, err := e.db.NewSelect().TableExpr(tb).Where("establishment_id = ?", estID).Count(ctx)
				if err != nil {
					t.Fatalf("count %s: %v", tb, err)
				}
				out[tb] = int(n)
			}
			return out
		}
		before2 := counts(est2.ID)
		if before2["bookings"] == 0 || before2["spot_allocations"] == 0 || before2["payments"] == 0 {
			t.Fatalf("other establishment without operations: %v", before2)
		}

		c := newClient(t, e.srv.URL)
		c.login(ownerEmail, ownerPassword)
		resetOf := func(estID, name string) result {
			return c.post("resetEstablishmentSetup", "/api/admin/v1/establishments/"+estID+"/setup/reset",
				api.SetupResetRequest{Scope: api.SetupResetRequestScopeEverything, ConfirmName: name})
		}

		// 1. Another organization's establishment: refused, nothing deleted.
		if r := resetOf(est2.ID, est2.Name); r.Status != http.StatusNotFound && r.Status != http.StatusForbidden {
			t.Fatalf("cross-tenant reset: %d %s", r.Status, r.Body)
		}
		est, err := store.GetEstablishment(ctx, e.db, e.es)
		if err != nil {
			t.Fatal(err)
		}
		c.must(resetOf(e.estID, est.Name), "reset everything")

		// 2. The events committed before the reset and every job run on
		// the emptied establishment without errors (runJobs fails the test
		// otherwise) and without recreating rows.
		e.runJobs()
		var left []string
		for tb, n := range counts(e.estID) {
			if n > 0 && !resetSurvivors[tb] {
				left = append(left, tb)
			}
		}
		sort.Strings(left)
		if len(left) > 0 {
			t.Errorf("rows left after the reset in %v", left)
		}
		after2 := counts(est2.ID)
		for _, tb := range tables {
			if before2[tb] != after2[tb] {
				t.Errorf("other establishment, %s: %d rows before, %d after", tb, before2[tb], after2[tb])
			}
		}

		// 3. The demo again, on the same establishment.
		if res, err := e.app.Setup.SeedDemo(ctx, e.estID); err != nil || !res.Applied {
			t.Fatalf("demo configuration after the reset: %+v %v", res, err)
		}
		if res, err := e.app.Setup.SeedDemoOperations(ctx, e.estID, e.app.DemoOperations(principal(e.es.OrganizationID()))); err != nil || !res.Applied {
			t.Fatalf("demo operations after the reset: %+v %v", res, err)
		}
		e.loadWorld(ctx)
		e.runJobs()
		d := newDay(t, e, 1)
		var customers []store.Customer
		if err := e.db.NewSelect().Model(&customers).Where("cu.establishment_id = ?", e.estID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		for _, cu := range customers {
			d.personalData = append(d.personalData, deref(cu.Email), deref(cu.Phone))
		}
		checkInvariants(t, e, d)
	})
}

// scopedTables lists the tables with an establishment_id column.
func scopedTables(ctx context.Context, t *testing.T, e *env) []string {
	t.Helper()
	q := `SELECT m.name FROM sqlite_master m JOIN pragma_table_info(m.name) p
		WHERE m.type = 'table' AND p.name = 'establishment_id'`
	if e.db.Kind == store.Postgres {
		q = `SELECT table_name FROM information_schema.columns
			WHERE column_name = 'establishment_id' AND table_schema = current_schema()`
	}
	rows, err := e.db.QueryContext(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) < 40 {
		t.Fatalf("schema discovery found only %d tables", len(out))
	}
	sort.Strings(out)
	return out
}
