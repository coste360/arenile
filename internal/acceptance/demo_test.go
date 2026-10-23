//go:build integration

package acceptance

import (
	"context"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
)

// TestAcceptance_DemoSeed runs the M3 demo seed through the real services
// and checks the same invariants as the simulated day: the demo data is as
// consistent as data entered from the panel, and a second run is a no-op.
func TestAcceptance_DemoSeed(t *testing.T) {
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
		res, err := e.app.Setup.SeedDemoOperations(ctx, e.estID, e.app.DemoOperations(p))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("demo: %+v", *res)
		d := newDay(t, e, 1)
		d.c.failIfNotImplemented()
		if !res.Applied || len(res.Skipped) != 0 {
			t.Fatalf("demo not fully applied: %+v", res)
		}
		if res.Customers != 40 || res.SeasonPasses != 25 || res.Bookings < 60 || res.Releases != 15 || res.Resales != 2 ||
			res.CashSessions != 2 || res.Payments == 0 || res.Cancelled == 0 {
			t.Fatalf("demo volumes: %+v", res)
		}
		e.runJobs()

		// The fake people must not leak into events or the stream.
		var customers []store.Customer
		if err := e.db.NewSelect().Model(&customers).Where("cu.establishment_id = ?", e.estID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		for _, c := range customers {
			d.personalData = append(d.personalData, deref(c.Email), deref(c.Phone))
		}
		checkInvariants(t, e, d)

		again, err := e.app.Setup.SeedDemoOperations(ctx, e.estID, e.app.DemoOperations(p))
		if err != nil || again.Applied {
			t.Fatalf("second demo run: %+v %v", again, err)
		}
	})
}

func deref(s *string) string {
	if s == nil {
		return "\x00"
	}
	return *s
}
