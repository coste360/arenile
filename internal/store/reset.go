package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Reset of an establishment (setup "Ricomincia" / "Azzera"): the rows of its
// configuration and, optionally, of its operations, deleted table by table
// in foreign key order (children before the rows they reference). Profile,
// team, payment and email settings, devices, cash registers, printers, the
// outbox and the audit log are never touched.

// operationTables are the operational data of an establishment, in delete
// order: what references bookings, payments and cash sessions first, then
// bookings, holds and cash sessions, customers last.
var operationTables = []string{
	"waitlist_offers",
	"waitlist_entries",
	"renewal_proposals",
	"renewal_campaigns",
	"refund_requests",
	"fiscal_bridge_jobs",
	"fiscal_documents",
	"tickets",
	"daily_ticket_counters",
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

// ResetEstablishment deletes the configuration of the establishment and,
// with operations, its operational data first. It returns the deleted rows
// per table (tables with none are omitted). Run it in a transaction: a
// failure halfway must leave everything in place.
func ResetEstablishment(ctx context.Context, tx bun.Tx, es EstablishmentScope, operations bool) (map[string]int64, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	tables := configurationTables
	if operations {
		tables = append(append([]string{}, operationTables...), configurationTables...)
	}
	deleted := map[string]int64{}
	for _, t := range tables {
		res, err := tx.NewDelete().TableExpr(t).Where("establishment_id = ?", es.ID()).Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("store: reset establishment (%s): %w", t, err)
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			deleted[t] += n
		}
	}
	return deleted, nil
}
