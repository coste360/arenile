// Package booking holds the booking domain: lifecycle statuses and their
// allowed transitions (spec 10.2), and the booking service that performs each
// transition together with the audit log entry and the outbox event in the same
// transaction.
//
// status.go is pure: it only knows which status changes are legal. The service
// calls Transition before writing, so an illegal change is refused with
// ErrInvalidTransition before touching the database.
//
// Ownership (M3): the "bookings" builder owns service.go and the files it
// adds here, the handlers in internal/http/admin/bookings.go (bookings,
// quote, actions, notes, auto-assignment) and the repository functions in
// internal/store/booking.go (bookings, booking_lines, booking_line_extras).
// Models: internal/store/models_booking.go. status.go is shared and stable.
//
// Use cases (spec 10, 11.3): create from the panel (draft | held |
// confirmed; inline customer; panel hold conversion), quote preview (no
// writes), modify dates/lines/notes with re-pricing and re-allocation
// (If-Match, dry run), confirm (manual driver only, never from a browser
// return URL: INV-19), approve/reject, check-in, no-show, cancel with refund
// or credit computed on the policy snapshot (dry run), move a line to
// another spot (or assign a pending one), notes, auto-assignment of pending
// zone-mode lines (pure availability.AutoAssign), expiry of booking holds
// and automatic completion (Jobs).
//
// Unit of work: every use case that changes a booking runs ONE transaction
// opened by this service with store.RunInTx and passes the same tx to every
// port and writer:
//
//	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
//		b, err := store.GetBookingForUpdate(ctx, tx, es, id)    // lock + version check (If-Match)
//		...
//		if err := Transition(Status(b.Status), StatusCancelled); err != nil { return err }
//		freed, err := s.d.Inventory.Release(ctx, tx, es, inventory.ReleaseRequest{BookingID: b.ID})
//		...
//		refund, err := s.d.Payments.RecordRefund(ctx, tx, p, es, payment.RefundRequest{...})
//		... update the bookings row with a conditional UPDATE on version ...
//		if err := s.d.Audit.Record(ctx, tx, audit.Entry{...}); err != nil { return err }
//		_, err = s.d.Outbox.Write(ctx, tx, event.BookingCancelled, es.ID(), 1, event.BookingChange{...})
//		return err
//	})
//
// so that allocations, booking rows, payments, audit log and outbox event
// commit or roll back together (INV-18, INV-20). Ports never open their own
// transaction when given tx; constraint violations (unique spot allocation,
// conditional pool UPDATE) surface as *availability.ConflictError and roll
// everything back. Services composed by another service (pass creating a
// season pass) receive the caller's tx through CreateInTx.
//
// Ledger is the stateless booking side of the payments ledger used by the
// payment service (paid_amount), kept separate from Service to avoid a
// construction cycle (payment <- booking.Ledger, booking -> payment).
//
// Permissions (spec 16.1): reads auth.PermBookingsRead; create, modify,
// confirm, cancel, move, notes auth.PermBookingsWrite; check-in and no-show
// auth.PermOperationsRun; the refund part of a cancellation
// auth.PermRefundsIssue (otherwise the cancellation records no refund and
// leaves it to a manager).
package booking
