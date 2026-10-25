//go:build integration

package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// TestAcceptance_SetupReset runs the two levels of the setup reset on the
// full demo (configuration and real operational data, both dialects):
//
//  1. with operational data the configuration-only reset is a conflict, a
//     wrong name is refused, nothing is deleted;
//  2. "everything" empties every table of the establishment it owns, keeps
//     the establishment, its team and the audit log, and writes setup.reset;
//  3. the setup wizard starts over (template applied again), and a
//     configuration-only reset now succeeds.
func TestAcceptance_SetupReset(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ctx := context.Background()
		owner, err := store.GetUserByEmail(ctx, e.db, ownerEmail)
		if err != nil {
			t.Fatal(err)
		}
		p := &auth.Principal{
			Kind: auth.PrincipalUser, UserID: owner.ID, Name: owner.Name,
			Grants: []auth.Grant{{OrganizationID: e.es.OrganizationID(), Role: auth.RoleOwner}},
		}
		if res, err := e.app.Setup.SeedDemoOperations(ctx, e.estID, e.app.DemoOperations(p)); err != nil || !res.Applied {
			t.Fatalf("demo operations: %+v %v", res, err)
		}
		est, err := store.GetEstablishment(ctx, e.db, e.es)
		if err != nil {
			t.Fatal(err)
		}
		c := newClient(t, e.srv.URL)
		c.login(ownerEmail, ownerPassword)
		status := func() api.SetupStatus {
			return decode[api.SetupStatus](t, c.must(c.get("getSetupStatus", e.path("/setup/status")), "setup status"))
		}
		reset := func(scope api.SetupResetRequestScope, name string) result {
			return c.post("resetEstablishmentSetup", e.path("/setup/reset"), api.SetupResetRequest{Scope: scope, ConfirmName: name})
		}
		rows := func(table string) int64 {
			n, err := e.db.NewSelect().TableExpr(table).Where("establishment_id = ?", e.estID).Count(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return int64(n)
		}

		// 1. Operational data: configuration only is refused; a wrong name too.
		if st := status(); !st.HasOperations || !st.HasSeason {
			t.Fatalf("status before: %+v", st)
		}
		if r := reset(api.SetupResetRequestScopeConfiguration, est.Name); r.Status != http.StatusConflict || r.Problem.Type != "conflict" {
			t.Fatalf("configuration reset with operations: %d %s", r.Status, r.Body)
		}
		if r := reset(api.SetupResetRequestScopeEverything, "Un altro lido"); r.Status != http.StatusBadRequest || r.Problem.Type != "validation_failed" {
			t.Fatalf("wrong name: %d %s", r.Status, r.Body)
		}
		bookings, customers, spots := rows("bookings"), rows("customers"), rows("spots")
		if bookings == 0 || customers == 0 || spots == 0 {
			t.Fatalf("demo rows: bookings %d customers %d spots %d", bookings, customers, spots)
		}

		// 2. Everything, with the name typed in another case and spaces.
		r := c.must(reset(api.SetupResetRequestScopeEverything, "  "+est.Name+" "), "reset everything")
		var out api.SetupResetResult
		if err := json.Unmarshal(r.Body, &out); err != nil {
			t.Fatal(err)
		}
		if r.Status != http.StatusOK || out.Scope != api.SetupResetResultScopeEverything ||
			out.Deleted["bookings"] != bookings || out.Deleted["customers"] != customers || out.Deleted["spots"] != spots {
			t.Fatalf("reset everything: %d %+v", r.Status, out)
		}
		for _, table := range []string{
			"bookings", "booking_lines", "spot_allocations", "pool_allocations", "pool_usage", "season_releases",
			"payments", "fiscal_documents", "cash_sessions", "cash_movements", "tickets", "customers",
			"seasons", "rate_periods", "price_rules", "fixed_duration_prices", "booking_policies", "promo_codes",
			"agreements", "maps", "spots", "map_decorations", "zones", "pools", "components", "packs", "extras",
		} {
			if n := rows(table); n != 0 {
				t.Errorf("%s: %d rows left", table, n)
			}
		}
		if _, err := store.GetEstablishment(ctx, e.db, e.es); err != nil {
			t.Fatalf("establishment gone: %v", err)
		}
		if n, err := e.db.NewSelect().TableExpr("memberships").Where("user_id = ?", owner.ID).Count(ctx); err != nil || n == 0 {
			t.Fatalf("team deleted: %d %v", n, err)
		}
		audited, err := e.db.NewSelect().TableExpr("audit_log").Where("establishment_id = ?", e.estID).Where("action = ?", "setup.reset").Count(ctx)
		if err != nil || audited != 1 {
			t.Fatalf("audit: %d %v", audited, err)
		}

		// 3. The wizard starts over.
		if st := status(); st.HasOperations || st.HasSeason || st.HasMap || st.SpotCount != 0 {
			t.Fatalf("status after: %+v", st)
		}
		c.must(c.post("applySetupTemplate", e.path("/setup/apply-template"), api.ApplyTemplateRequest{Template: "classic_rows"}), "template again")
		if st := status(); !st.HasMap || st.SpotCount == 0 {
			t.Fatalf("status after the template: %+v", st)
		}
		r = c.must(reset(api.SetupResetRequestScopeConfiguration, est.Name), "reset configuration")
		if r.Status != http.StatusOK {
			t.Fatalf("configuration reset without operations: %d %s", r.Status, r.Body)
		}
		if st := status(); st.HasMap || st.HasSeason {
			t.Fatalf("status after the configuration reset: %+v", st)
		}
	})
}
