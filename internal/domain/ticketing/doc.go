// Package ticketing owns the daily tickets (spec 14.3, ADR 0020): the
// progressive number of the day, the ticket of each walk-in or entry sale,
// the "da assegnare" queue of zone-first sales, print data, void, move and
// the entry gate.
//
// # Invariants (enforced by the database, migration 00010)
//
//   - The number restarts at 1 every day (establishment timezone) and is
//     unique per establishment across registers and devices: it comes from
//     store.NextTicketNumber (one atomic upsert on daily_ticket_counters in
//     the sale transaction) and tickets carries UNIQUE (establishment_id,
//     ticket_date, ticket_number). A voided ticket keeps its number.
//   - A ticket owns exactly one booking (UNIQUE booking_id). Every state
//     change of that booking (creation, move, cancellation on void) goes
//     through internal/domain/booking, which writes the audit log and the
//     outbox event in the same transaction.
//   - Zone-first capacity is consumed at sale time by the existing
//     constraints: pending line on the implicit zone pool (zone mode) or a
//     provisional spot allocation (seat mode, tickets.provisional_spot_id).
//     Assignment is a booking Move: atomic, 409 when the spot is taken.
//   - One entry per ticket: entered_at is set with a conditional update
//     (WHERE entered_at IS NULL AND status = 'valid').
//
// # Structure
//
//   - label.go is pure: number labels ("#037") and the derived state shown
//     to the staff (assigned, unassigned, issued, entered, voided).
//   - errors.go: sentinel errors mapped by internal/http/problem.
//   - ports.go: what the service consumes (bookings, payments, auto-assign).
//
// The service (issue, list, print, void, assign, move, auto-assign, gate
// check-in, entry ticket types) is built on these ports and wired by
// internal/app.
package ticketing
