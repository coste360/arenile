// Package booking holds the booking domain: lifecycle statuses and their
// allowed transitions (spec 10.2), and the booking service that performs each
// use case together with the audit log entry and the outbox event in the same
// transaction.
//
// # Structure (functional core, imperative shell)
//
//   - status.go is pure: it only knows which status changes are legal. Every
//     use case calls Transition before writing, so an illegal change is
//     refused with *TransitionError (409 invalid_transition, INV-17) before
//     touching the database.
//   - plan.go is the rest of the pure core: initial status and allocation
//     kind of a new booking (DecideInitialState), booking codes
//     (CodePrefix, NextCode: "LD-2027-000123", unique per establishment),
//     expansion of quantity lines for the pricing engine and the way back
//     (PricingLines, LineTotals), refund of a cancellation on the policy
//     snapshot (ComputeRefund, never above the paid amount: INV-24).
//   - service.go, modify.go, pricing.go and jobs.go are the shell: they load
//     data, call the ports and the core, and write in one transaction.
//   - ledger.go is the stateless booking side of the payments ledger
//     (paid_amount, INV-22) used by the payment service, kept apart from
//     Service to avoid a construction cycle (payment <- booking.Ledger,
//     booking -> payment.Payments).
//   - bookingtest/ builds a service on a real database with DB-backed test
//     adapters for the ports (integration tests here and in internal/http/admin).
//
// # Ports (ports.go, wired by internal/app/m3.go)
//
// Inventory (allocations, holds, pools), Pricer (pricing engine + policy
// inside the transaction), Customers (existing or inline customer), Payments
// (refunds of cancellations, ledger rows), Releases (resale of released
// season days; optional).
//
// # Use cases (spec 9, 10, 11.3, 14.1)
//
//   - Create / CreateInTx: panel creation (channel desk by default) in ONE
//     transaction: customer, catalog validation, price and policy (violations
//     refused unless Force, which is audited), status (draft: no resources;
//     held: hold with ARENILE_HOLD_TTL; confirmed by default: the manual
//     driver, money collected at the till; approval required ->
//     awaiting_approval with a long hold, Deps.ApprovalTTL), human code,
//     rows, promo usage (conditional UPDATE), allocation or panel hold
//     conversion, audit, outbox (booking.created + booking.confirmed /
//     booking.held / booking.approval_requested). Snapshots of price and
//     policy are stored on the booking.
//   - Quote: same computation without writes plus Inventory.Check.
//   - Update (PATCH, If-Match): dates, slots, formula, lines, extras,
//     customer, notes; re-pricing and atomic re-allocation (release old +
//     allocate new in the same transaction; a conflict rolls back and leaves
//     the original intact). Snapshots of a confirmed booking never change
//     (INV-21): the new total gives the difference to collect or refund, the
//     new breakdown goes into the audit diff.
//   - Move: one line to another spot, whole stay or from a date; for a
//     pending line of a zone-mode zone it is the assignment.
//   - AutoAssign: pending zone-mode lines placed with the pure
//     availability.AutoAssign on the free spots; each placement is a Move.
//   - Confirm (manual driver only, never from a browser return URL: INV-19),
//     Approve, Reject, CheckIn, MarkNoShow (releases from today), AddNote
//     (text not copied into the audit log), Cancel (releases; refund or
//     credit computed on the policy snapshot; dry run).
//   - Jobs: expiry of booking holds (every 30 s) and completion of past
//     bookings (hourly).
//
// Dry runs (Update, Move) execute the real change and roll the transaction
// back (errDryRun), so a preview can never disagree with the outcome.
//
// # Unit of work and concurrency
//
// Every use case that changes a booking runs ONE transaction opened by this
// service with store.RunInTx and passes the same tx to every port and
// writer (mutate / mutateInTx):
//
//	store.LockBooking(tx)                 // row lock (no-op UPDATE), latest version
//	configerr.CheckVersion(If-Match)      // 412 version_conflict
//	Transition(from, to)                  // 409 invalid_transition
//	Inventory.Release / Allocate (tx)     // DB constraints decide availability
//	store.UpdateBookingVersioned(tx)      // conditional UPDATE ... WHERE version = ?
//	Payments.RecordRefund (tx)            // after the row update (paid_amount)
//	Audit.Record + Outbox.Write (tx)      // INV-18
//
// so that allocations, booking rows, payments, audit log and outbox event
// commit or roll back together (INV-18, INV-20). Availability is never
// checked by the application: the unique (spot_id, date, slot) index and the
// conditional pool UPDATE of the inventory reject a double allocation with
// *availability.ConflictError (409 with the conflicting half-days). Booking
// codes are allocated with MAX + 1 inside a savepoint retried on the unique
// (establishment_id, code) index. Services composed by another service
// (pass creating a season pass) receive the caller's tx through CreateInTx.
//
// # Permissions (spec 16.1)
//
// Reads (Get, List, Quote) auth.PermBookingsRead; create, modify, confirm,
// approve, reject, cancel, move, notes, auto-assignment
// auth.PermBookingsWrite; check-in and no-show auth.PermOperationsRun;
// recording the refund of a cancellation auth.PermRefundsIssue (otherwise
// the cancellation leaves the refund due, visible in the audit diff, for a
// manager or for the cashier through the payments API).
//
// # Events
//
// Payload event.BookingChange (no personal data) with spot_ids and dates
// hints for the SSE stream: booking.created, booking.held,
// booking.confirmed, booking.approval_requested, booking.modified (also
// moves and notes), booking.cancelled, booking.checked_in, booking.no_show,
// booking.rejected, booking.expired, booking.completed.
package booking
