//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/coste360/arenile/internal/migrations"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00008_CreditBackfillAndDown rolls 00008 back on a migrated
// database, writes credits with the 00003 schema, applies 00008 again and
// checks that customer_credit_balances is backfilled with the ledger sum and
// that the old rows survive the customer_credits rebuild (SQLite) / the
// constraint swap (PostgreSQL); then the down migration runs once more.
func TestMigration00008_CreditBackfillAndDown(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		fsys, err := migrations.FS(db.Kind)
		if err != nil {
			t.Fatal(err)
		}
		gd := goose.DialectSQLite3
		if db.Kind == store.Postgres {
			gd = goose.DialectPostgres
		}
		p, err := goose.NewProvider(gd, db.DB.DB, fsys)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.DownTo(ctx, 7); err != nil {
			t.Fatalf("down to 7: %v", err)
		}
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-bf", "Lido BF", t0)
		if err != nil {
			t.Fatal(err)
		}
		cust := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, LastName: "Rossi", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		if _, err := db.NewInsert().Model(cust).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		for _, row := range []struct {
			amount int64
			source string
		}{{3000, "cancellation"}, {-1000, "redemption"}, {500, "manual"}} {
			if _, err := db.ExecContext(ctx,
				"INSERT INTO customer_credits (id, establishment_id, customer_id, amount, source, note, created_at) VALUES (?, ?, ?, ?, ?, '', ?)",
				store.NewID(), est.ID, cust.ID, row.amount, row.source, t0); err != nil {
				t.Fatalf("insert credit: %v", err)
			}
		}
		if _, err := p.Up(ctx); err != nil {
			t.Fatalf("up: %v", err)
		}
		var bal store.CustomerCreditBalance
		if err := db.NewSelect().Model(&bal).Where("customer_id = ?", cust.ID).Scan(ctx); err != nil {
			t.Fatalf("balance: %v", err)
		}
		if bal.Balance != 2500 || bal.EstablishmentID != est.ID {
			t.Fatalf("backfilled balance = %+v, want 2500", bal)
		}
		n, err := db.NewSelect().Model((*store.CustomerCredit)(nil)).Where("customer_id = ?", cust.ID).Count(ctx)
		if err != nil || n != 3 {
			t.Fatalf("ledger rows = %d (%v), want 3", n, err)
		}
		if _, err := p.DownTo(ctx, 7); err != nil {
			t.Fatalf("down to 7 after backfill: %v", err)
		}
		if _, err := p.Up(ctx); err != nil {
			t.Fatalf("up again: %v", err)
		}
	})
}
