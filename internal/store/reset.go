package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Reset of an establishment (setup "Ricomincia" / "Azzera"): the rows of its
// configuration and, optionally, of its operations, deleted table by table
// in foreign key order (children before the rows they reference). Profile,
// team, payment and email settings, devices, cash registers, printers, the
// outbox and the audit log are never touched.

// operationTables are the operational data of an establishment, in delete
// order: what references bookings, payments and cash sessions first, then
// bookings, holds and cash sessions, customers last. The daily ticket
// counters stay: the numbering of the day goes on and never repeats a paper
// ticket already handed out (INV-81).
var operationTables = []string{
	"waitlist_offers",
	"waitlist_entries",
	"renewal_proposals",
	"renewal_campaigns",
	"refund_requests",
	"fiscal_bridge_jobs",
	"fiscal_documents",
	"tickets",
	"payment_intents",
	"checkout_sessions",
	"customer_credits",
	"customer_credit_balances",
	"message_log",
	"season_releases",
	"spot_allocations",
	"pool_allocations",
	"pool_usage",
	"spot_blocks",
	"payments",
	"magic_link_tokens",
	"booking_line_extras",
	"booking_lines",
	"bookings",
	"holds",
	"cash_movements",
	"cash_sessions",
	"import_jobs",
	"customers",
}

// operationMarkers are the operational tables whose rows make a
// configuration-only reset impossible (they reference the configuration
// with restrictive foreign keys, or would lose their meaning).
var operationMarkers = []string{
	"bookings", "holds", "payments", "payment_intents", "cash_sessions", "tickets",
	"waitlist_entries", "renewal_campaigns", "customer_credits",
}

// configurationTables are the configuration of an establishment, in delete
// order: entry ticket types (packs, zones), the season and its price list,
// spots before zones, pools (their usage cascades) before components, maps
// last.
var configurationTables = []string{
	"entry_ticket_types",
	"booking_policies",
	"price_rules",
	"fixed_duration_prices",
	"promo_codes",
	"agreements",
	"season_rollovers",
	"closure_days",
	"rate_periods",
	"seasons",
	"spot_blocks",
	"spots",
	"map_decorations",
	"pool_usage",
	"pools",
	"zones",
	"components",
	"packs",
	"extras",
	"maps",
	"daily_sale_settings",
}

// HasOperations reports whether the establishment has operational data
// (operationMarkers).
func HasOperations(ctx context.Context, db bun.IDB, es EstablishmentScope) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	for _, t := range operationMarkers {
		ok, err := db.NewSelect().TableExpr(t).Where("establishment_id = ?", es.ID()).Exists(ctx)
		if err != nil {
			return false, fmt.Errorf("store: has operations (%s): %w", t, err)
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// HasPaymentsInFlight reports whether money is moving at the payment
// provider for the establishment: an online payment not yet final
// (created, pending) or a refund not yet done (pending, processing, failed
// and to retry). Deleting them would lose a payment or a refund the
// customer is owed (INV-51, INV-75).
func HasPaymentsInFlight(ctx context.Context, db bun.IDB, es EstablishmentScope) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	for _, c := range []struct {
		table    string
		statuses []string
	}{
		{"payment_intents", []string{"created", "pending"}},
		{"refund_requests", []string{"pending", "processing", "failed"}},
	} {
		ok, err := db.NewSelect().TableExpr(c.table).Where("establishment_id = ?", es.ID()).
			Where("status IN (?)", bun.List(c.statuses)).Exists(ctx)
		if err != nil {
			return false, fmt.Errorf("store: payments in flight (%s): %w", c.table, err)
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// HasFiscalRecords reports whether the establishment has fiscal documents
// of a real fiscal driver (the registratore telematico behind the fiscal
// bridge; "none", builtin.NoneFiscal, performs no I/O): issued or voided
// receipts the law requires to keep, documents still pending at the device,
// or bridge jobs queued or leased (a job may have no document linked). A
// reset must not delete them. Failed documents do not count: nothing was
// issued. Test payments never get a fiscal document.
func HasFiscalRecords(ctx context.Context, db bun.IDB, es EstablishmentScope) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	ok, err := db.NewSelect().TableExpr("fiscal_documents").Where("establishment_id = ?", es.ID()).
		Where("driver <> ?", "none").Where("status IN (?)", bun.List([]string{FiscalPending, FiscalIssued, FiscalVoid})).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("store: has fiscal records: %w", err)
	}
	if ok {
		return true, nil
	}
	ok, err = db.NewSelect().TableExpr("fiscal_bridge_jobs").Where("establishment_id = ?", es.ID()).
		Where("status IN (?)", bun.List([]string{"queued", "leased"})).Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("store: has fiscal records (bridge jobs): %w", err)
	}
	return ok, nil
}

// ResetEstablishment deletes the configuration of the establishment and,
// with operations, its operational data first. It returns the deleted rows
// per table (tables with none are omitted). Run it in a transaction: a
// failure halfway must leave everything in place.
func ResetEstablishment(ctx context.Context, tx bun.Tx, es EstablishmentScope, operations bool, now time.Time) (map[string]int64, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	tables := configurationTables
	if operations {
		tables = append(append([]string{}, operationTables...), configurationTables...)
	}
	deleted := map[string]int64{}
	count := func(t string, res sql.Result) {
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			deleted[t] += n
		}
	}
	for _, t := range tables {
		res, err := tx.NewDelete().TableExpr(t).Where("establishment_id = ?", es.ID()).Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("store: reset establishment (%s): %w", t, err)
		}
		count(t, res)
	}
	if operations {
		// Kept tables with rows about the deleted data: the print jobs not
		// printed yet (their payload names the holder) and the panel
		// notifications pointing at a booking, payment or customer.
		res, err := tx.NewDelete().TableExpr("print_jobs").Where("establishment_id = ?", es.ID()).
			Where("status IN (?)", bun.List([]string{"queued", "failed"})).Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("store: reset establishment (print_jobs): %w", err)
		}
		count("print_jobs", res)
		res, err = tx.NewDelete().TableExpr("admin_notifications").Where("establishment_id = ?", es.ID()).
			Where("resource_type IS NOT NULL").Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("store: reset establishment (admin_notifications): %w", err)
		}
		count("admin_notifications", res)
	}
	// Without a season or a map nothing can be booked: the public page goes
	// off until the setup is done again.
	if _, err := tx.NewUpdate().TableExpr("establishments").Set("booking_enabled = ?", false).
		Set("version = version + 1").Set("updated_at = ?", now).Where("id = ?", es.ID()).Where("booking_enabled = ?", true).Exec(ctx); err != nil {
		return nil, fmt.Errorf("store: reset establishment (booking off): %w", err)
	}
	return deleted, nil
}
