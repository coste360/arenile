// Package dailysale is the quick walk-in sale ("Vendita giornaliera", spec
// 14.3, ADR 0020): one request chooses where (a spot, or only a zone), the
// slot (full day, morning, afternoon), the pack and the extras, prices them
// with the pricing engine for that day and, in one transaction:
//
//  1. resolves the customer (optional holder name, else the walk-in customer
//     of daily_sale_settings);
//  2. creates a confirmed booking on channel desk through
//     internal/domain/booking (audit + outbox in the same transaction;
//     availability by spot_allocations / pool_usage constraints);
//  3. records the manual payment (cash needs an open cash session, card
//     does not);
//  4. issues the ticket with the next daily number (internal/domain/ticketing).
//
// Entry sales (entry ticket types on quantity zones) follow the same steps
// with one line per ticket type.
//
// # Structure
//
//   - plan.go is pure: slot to formula, target validation (exactly one of
//     spot or zone), holder name normalisation.
//   - ports.go: what the service consumes.
package dailysale
