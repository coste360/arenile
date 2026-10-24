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

// TestMigration00009_CashRegisterBackfill rolls 00009 back, writes cash
// sessions with the 00003 schema (register codes main and bar), applies
// 00009 again and checks the backfill: a default "Cassa principale" (main)
// per establishment, one register per other code, register_id on every
// session and at most one open session per register.
func TestMigration00009_CashRegisterBackfill(t *testing.T) {
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
		if _, err := p.DownTo(ctx, 8); err != nil {
			t.Fatalf("down to 8: %v", err)
		}
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-cr", "Lido CR", t0)
		if err != nil {
			t.Fatal(err)
		}
		mainID, barID := store.NewID(), store.NewID()
		for _, s := range []struct {
			id, register string
			closed       bool
		}{{mainID, "main", false}, {barID, "bar", false}, {store.NewID(), "bar", true}} {
			var closedAt, counted, diff any
			if s.closed {
				closedAt, counted, diff = t0, "{}", 0
			}
			if _, err := db.ExecContext(ctx,
				`INSERT INTO cash_sessions (id, establishment_id, register, opened_at, closed_at, opening_float,
				 expected_totals, counted_totals, difference, notes, version, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, 0, '{}', ?, ?, '', 1, ?, ?)`,
				s.id, est.ID, s.register, t0, closedAt, counted, diff, t0, t0); err != nil {
				t.Fatalf("insert session: %v", err)
			}
		}
		if _, err := p.Up(ctx); err != nil {
			t.Fatalf("up: %v", err)
		}

		var regs []store.CashRegister
		if err := db.NewSelect().Model(&regs).Where("establishment_id = ?", est.ID).Order("sort_order").Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if len(regs) != 2 || regs[0].Code != "main" || !regs[0].IsDefault || regs[0].Name != store.DefaultCashRegisterName ||
			regs[1].Code != "bar" || regs[1].IsDefault || !regs[1].Active {
			t.Fatalf("registers = %+v", regs)
		}
		var sessions []store.CashSession
		if err := db.NewSelect().Model(&sessions).Where("establishment_id = ?", est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		for _, s := range sessions {
			want := regs[0].ID
			if s.Register == "bar" {
				want = regs[1].ID
			}
			if s.RegisterID == nil || *s.RegisterID != want {
				t.Fatalf("session %s (%s): register_id = %v, want %s", s.ID, s.Register, s.RegisterID, want)
			}
		}

		// A second open session on the same register violates the new index
		// even under another legacy code.
		dup := &store.CashSession{
			ID: store.NewID(), EstablishmentID: est.ID, Register: "other", RegisterID: &regs[0].ID, OpenedAt: t0,
			ExpectedTotals: store.MethodTotals{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		if _, err := db.NewInsert().Model(dup).Exec(ctx); !store.IsUniqueViolation(err) {
			t.Fatalf("second open session: want unique violation, got %v", err)
		}
		// INV-73: a session without a register, or with a register of
		// another establishment, is refused by the schema.
		if _, err := db.ExecContext(ctx,
			`INSERT INTO cash_sessions (id, establishment_id, register, opened_at, opening_float,
			 expected_totals, notes, version, created_at, updated_at)
			 VALUES (?, ?, 'ghost', ?, 0, '{}', '', 1, ?, ?)`, store.NewID(), est.ID, t0, t0, t0); err == nil {
			t.Fatal("session without register_id accepted")
		}
		_, other, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org 2", "lido-cr2", "Lido CR2", t0)
		if err != nil {
			t.Fatal(err)
		}
		foreign := &store.CashSession{
			ID: store.NewID(), EstablishmentID: other.ID, Register: "main", RegisterID: &regs[1].ID, OpenedAt: t0,
			ExpectedTotals: store.MethodTotals{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		if _, err := db.NewInsert().Model(foreign).Exec(ctx); err == nil {
			t.Fatal("session on a register of another establishment accepted")
		}
		// A second default register is rejected.
		def2 := &store.CashRegister{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "spiaggia", Name: "Spiaggia",
			Active: true, IsDefault: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		if _, err := db.NewInsert().Model(def2).Exec(ctx); !store.IsUniqueViolation(err) {
			t.Fatalf("second default register: want unique violation, got %v", err)
		}
	})
}

// TestMigration00009_ModelsRoundtrip inserts and reloads the 00009 models
// and checks the keys their services rely on.
func TestMigration00009_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-d9", "Lido D9", t0)
		if err != nil {
			t.Fatal(err)
		}
		insert := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		expectConflict := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); !store.IsUniqueViolation(err) {
				t.Fatalf("insert %T: want unique violation, got %v", m, err)
			}
		}
		expectError := func(what string, m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err == nil {
				t.Fatalf("insert %s: want constraint error", what)
			}
		}

		insert(&store.TestModeSettings{EstablishmentID: est.ID, Enabled: true, PublicEnabled: true, EnabledAt: &t0, Version: 1, CreatedAt: t0, UpdatedAt: t0})
		var tms store.TestModeSettings
		if err := db.NewSelect().Model(&tms).Where("establishment_id = ?", est.ID).Scan(ctx); err != nil || !tms.Enabled || !tms.PublicEnabled {
			t.Fatalf("test mode settings = %+v, %v", tms, err)
		}
		_, est2, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org2", "lido-d9b", "Lido D9B", t0)
		if err != nil {
			t.Fatal(err)
		}
		expectError("public without enabled", &store.TestModeSettings{EstablishmentID: est2.ID, PublicEnabled: true, Version: 1, CreatedAt: t0, UpdatedAt: t0})

		n := &store.AdminNotification{
			ID: store.NewID(), EstablishmentID: est.ID, Kind: "webhook_endpoint_disabled", Severity: "warning",
			Audience: store.NotificationAudienceOwner, Params: map[string]string{"endpoint_id": "x"}, DedupeKey: "webhook_disabled:x:1", CreatedAt: t0,
		}
		insert(n)
		dupN := *n
		dupN.ID = store.NewID()
		expectConflict(&dupN)

		job := &store.ImportJob{
			ID: store.NewID(), EstablishmentID: est.ID, Kind: store.ImportKindCustomers, Status: store.ImportStatusApplied,
			Filename: "clienti.csv", SizeBytes: 10, SHA256: "abc", Options: map[string]any{}, AppliedAt: &t0, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(job)
		again := *job
		again.ID = store.NewID()
		expectConflict(&again)
		again.Status, again.AppliedAt = store.ImportStatusUploaded, nil
		insert(&again)
		expectError("applied without applied_at", &store.ImportJob{
			ID: store.NewID(), EstablishmentID: est.ID, Kind: store.ImportKindCustomers,
			Status: store.ImportStatusApplied, SHA256: "def", Options: map[string]any{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
	})
}
