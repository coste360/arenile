//go:build integration

package store_test

import (
	"context"
	"slices"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// resetKeptTables are the establishment-scoped tables that
// store.ResetEstablishment never touches, each with its reason. A new table
// with an establishment_id column must be added either to the reset lists
// of reset.go or here: TestResetEstablishment_EveryTableClassified fails
// otherwise, so a reset never leaves operational rows behind by omission.
var resetKeptTables = map[string]string{
	"memberships":               "team (INV-36)",
	"api_keys":                  "integrations",
	"audit_log":                 "insert only (INV-30); setup.reset is recorded there",
	"outbox_events":             "already committed events, delivered at least once (INV-32)",
	"webhook_endpoints":         "integrations",
	"webhook_delivery_attempts": "delivery history of committed events",
	"webhook_events":            "provider event dedupe (INV-23): a redelivery after the reset stays a no-op",
	"terms_versions":            "online settings",
	"online_settings":           "online settings",
	"payment_provider_settings": "payment settings",
	"notification_templates":    "email settings",
	"email_template_overrides":  "email settings",
	"devices":                   "paired devices",
	"device_pairing_codes":      "paired devices",
	"fiscal_bridges":            "paired fiscal bridge",
	"waitlist_settings":         "settings",
	"test_mode_settings":        "settings",
	"cash_registers":            "cash registers (INV-73: the default register survives)",
	"printers":                  "printers",
	"printer_routes":            "printers",
	"print_jobs":                "print queue (no foreign keys); with everything the jobs not printed yet go",
	"admin_notifications":       "panel notifications (no foreign keys); with everything those about a resource go",
	"daily_ticket_counters":     "the numbering of the day goes on, never repeating a ticket handed out (INV-81)",
}

// TestResetEstablishment_EveryTableClassified checks the reset table lists
// of reset.go against the migrated schema of both dialects:
//
//   - every listed table exists and has establishment_id (the delete is
//     always scoped by establishment, INV-26);
//   - every table with establishment_id is either reset or kept on purpose
//     (resetKeptTables);
//   - the configuration-only guard (operationMarkers) names operational
//     tables;
//   - the delete order respects every restrictive foreign key between reset
//     tables (child first), and no kept table blocks the reset with a
//     restrictive foreign key to a reset table.
func TestResetEstablishment_EveryTableClassified(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		scoped := establishmentScopedTables(ctx, t, db)

		full := append(append([]string{}, store.ResetOperationTables...), store.ResetConfigurationTables...)
		for _, tb := range full {
			if !scoped[tb] {
				t.Errorf("reset table %s does not exist or has no establishment_id", tb)
			}
			if _, kept := resetKeptTables[tb]; kept {
				t.Errorf("table %s is both reset and kept", tb)
			}
		}
		for _, tb := range store.ResetOperationMarkers {
			if !slices.Contains(store.ResetOperationTables, tb) {
				t.Errorf("operation marker %s is not an operational table", tb)
			}
		}
		for tb := range scoped {
			if _, kept := resetKeptTables[tb]; !kept && !slices.Contains(full, tb) {
				t.Errorf("table %s has establishment_id but is neither reset nor kept: classify it in reset.go or resetKeptTables", tb)
			}
		}
		for tb := range resetKeptTables {
			if !scoped[tb] {
				t.Errorf("kept table %s does not exist or has no establishment_id", tb)
			}
		}

		// Restrictive foreign keys: the child is deleted before its parent,
		// in the full order and in the configuration-only order.
		firstIndex := slices.Index[[]string]
		lastIndex := func(list []string, tb string) int {
			for i := len(list) - 1; i >= 0; i-- {
				if list[i] == tb {
					return i
				}
			}
			return -1
		}
		for _, fk := range foreignKeys(ctx, t, db) {
			if fk.child == fk.parent || fk.onDelete == "CASCADE" || fk.onDelete == "SET NULL" {
				continue
			}
			parentReset := slices.Contains(full, fk.parent)
			if !parentReset {
				continue
			}
			if !slices.Contains(full, fk.child) {
				t.Errorf("%s references the reset table %s with ON DELETE %s: the reset would fail once it has rows", fk.child, fk.parent, fk.onDelete)
				continue
			}
			// The last delete of the child must come before the last delete
			// of the parent (a table may appear in both lists).
			if lastIndex(full, fk.child) > lastIndex(full, fk.parent) {
				t.Errorf("full reset deletes %s before its child %s (ON DELETE %s)", fk.parent, fk.child, fk.onDelete)
			}
			cfg := store.ResetConfigurationTables
			if p := firstIndex(cfg, fk.parent); p >= 0 {
				if c := lastIndex(cfg, fk.child); c > p {
					t.Errorf("configuration reset deletes %s before its child %s (ON DELETE %s)", fk.parent, fk.child, fk.onDelete)
				}
			}
		}
	})
}

// establishmentScopedTables lists the tables with an establishment_id
// column.
func establishmentScopedTables(ctx context.Context, t *testing.T, db *store.DB) map[string]bool {
	t.Helper()
	q := `SELECT m.name FROM sqlite_master m JOIN pragma_table_info(m.name) p
		WHERE m.type = 'table' AND p.name = 'establishment_id'`
	if db.Kind == store.Postgres {
		q = `SELECT table_name FROM information_schema.columns
			WHERE column_name = 'establishment_id' AND table_schema = current_schema()`
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) < 40 {
		t.Fatalf("schema discovery found only %d tables", len(out))
	}
	return out
}

type foreignKey struct{ child, parent, onDelete string }

// foreignKeys lists every foreign key of the schema with its ON DELETE rule
// (CASCADE, SET NULL, RESTRICT, NO ACTION, SET DEFAULT).
func foreignKeys(ctx context.Context, t *testing.T, db *store.DB) []foreignKey {
	t.Helper()
	q := `SELECT m.name, f."table", f.on_delete FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table'`
	if db.Kind == store.Postgres {
		q = `SELECT DISTINCT tc.table_name, ccu.table_name, rc.delete_rule
			FROM information_schema.referential_constraints rc
			JOIN information_schema.table_constraints tc
			  ON tc.constraint_name = rc.constraint_name AND tc.constraint_schema = rc.constraint_schema
			JOIN information_schema.constraint_column_usage ccu
			  ON ccu.constraint_name = rc.unique_constraint_name AND ccu.constraint_schema = rc.unique_constraint_schema
			WHERE tc.table_schema = current_schema()`
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []foreignKey
	for rows.Next() {
		var fk foreignKey
		if err := rows.Scan(&fk.child, &fk.parent, &fk.onDelete); err != nil {
			t.Fatal(err)
		}
		out = append(out, fk)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) < 50 {
		t.Fatalf("schema discovery found only %d foreign keys", len(out))
	}
	return out
}
