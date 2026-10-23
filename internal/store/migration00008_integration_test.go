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
		// Legacy data: a negative sum (uncapped reversal) and an expired
		// grant never materialised.
		neg := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, LastName: "Negativo", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		old := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, LastName: "Scaduto", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		if _, err := db.NewInsert().Model(&[]*store.Customer{neg, old}).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		for _, row := range []struct {
			cust    string
			amount  int64
			source  string
			expires any
		}{{neg.ID, 2000, "season_release", nil}, {neg.ID, -2000, "redemption", nil}, {neg.ID, -2000, "season_release", nil}, {old.ID, 800, "cancellation", "2020-01-01"}} {
			if _, err := db.ExecContext(ctx,
				"INSERT INTO customer_credits (id, establishment_id, customer_id, amount, source, note, expires_on, created_at) VALUES (?, ?, ?, ?, ?, '', ?, ?)",
				store.NewID(), est.ID, row.cust, row.amount, row.source, row.expires, t0); err != nil {
				t.Fatalf("insert legacy credit: %v", err)
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
		// Negative legacy sum: an adjustment row brings the ledger to the
		// stored balance (0), so balance = SUM(ledger) holds (INV-70).
		var adj store.CustomerCredit
		if err := db.NewSelect().Model(&adj).Where("cc.customer_id = ?", neg.ID).Where("cc.source = ?", store.CreditSourceAdjustment).Scan(ctx); err != nil {
			t.Fatalf("adjustment row: %v", err)
		}
		if adj.Amount != 2000 || adj.ID == "" {
			t.Fatalf("adjustment %+v", adj)
		}
		var negBal store.CustomerCreditBalance
		if err := db.NewSelect().Model(&negBal).Where("customer_id = ?", neg.ID).Scan(ctx); err != nil || negBal.Balance != 0 {
			t.Fatalf("negative legacy balance = %+v (%v), want 0", negBal, err)
		}
		var negSum int64
		if err := db.NewSelect().TableExpr("customer_credits").ColumnExpr("SUM(amount)").Where("customer_id = ?", neg.ID).Scan(ctx, &negSum); err != nil || negSum != 0 {
			t.Fatalf("negative legacy ledger sum = %d (%v), want 0", negSum, err)
		}
		// An expired legacy grant stays in the balance (SUM of the ledger):
		// the expiry is materialised by the credits.expire job, available
		// already excludes it.
		var oldBal store.CustomerCreditBalance
		if err := db.NewSelect().Model(&oldBal).Where("customer_id = ?", old.ID).Scan(ctx); err != nil || oldBal.Balance != 800 {
			t.Fatalf("expired legacy balance = %+v (%v), want 800", oldBal, err)
		}
		if _, err := p.DownTo(ctx, 7); err != nil {
			t.Fatalf("down to 7 after backfill: %v", err)
		}
		if _, err := p.Up(ctx); err != nil {
			t.Fatalf("up again: %v", err)
		}
	})
}
