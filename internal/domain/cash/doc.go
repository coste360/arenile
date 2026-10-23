// Package cash runs the cash desk (spec 14.1): sessions opened with a float,
// cash movements in/out, closing with the counted amounts and the
// difference against the expected totals (INV-25), and the Z-style summary
// by method and VAT rate.
//
// Ownership (M3): the "cash" builder owns this package, the handlers in
// internal/http/admin/cash.go and the repository functions in
// internal/store/cash.go. Models: store.CashSession, store.CashMovement.
//
// Functional core / imperative shell: Summarize (summary.go) is pure and
// computes takings, expected, counted and differences from the float, the
// session payments and the movements; service.go loads, locks and writes.
//
// Rules:
//   - at most one open session per (establishment, register): enforced by
//     the unique partial index cash_sessions_open_uidx; a violation (also
//     under concurrency) is ErrSessionAlreadyOpen (409
//     cash_session_already_open);
//   - SessionForPayment, AddMovement and Close take the row lock of the
//     open session (no-op conditional UPDATE ... WHERE closed_at IS NULL),
//     so a payment either lands before the close and is in its expected
//     totals, or fails with ErrSessionClosed (409 cash_session_closed);
//   - expected totals per method = opening float (cash) + succeeded
//     payments of the session (refunds negative) + movements in - movements
//     out (cash); for open sessions they are computed live, on close they
//     are frozen with counted totals (missing methods count as 0) and
//     difference = sum(counted) - sum(expected), with a conditional UPDATE
//     (still open, version = If-Match when given); a withdrawal larger than
//     the expected cash is refused;
//   - every write records the audit log and an outbox event
//     (cash_session.opened, cash_movement.recorded, cash_session.closed,
//     payload event.CashChange) in the same transaction.
//
// The session's payments are read with store queries on payments
// (cash_session_id): cash does not depend on the payment package. It
// implements payment.CashDesk (SessionForPayment).
//
// Permissions: auth.PermCashOperate for every write; reads (current
// session, detail, history) accept auth.PermCashOperate or
// auth.PermReportsRead.
package cash
