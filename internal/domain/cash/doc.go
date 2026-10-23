// Package cash runs the cash desk (spec 14.1): sessions opened with a float,
// cash movements in/out, closing with the counted amounts and the
// difference against the expected totals (INV-25).
//
// Ownership (M3): the "cash" builder owns this package, the handlers in
// internal/http/admin/cash.go and the repository functions in
// internal/store/cash.go. Models: store.CashSession, store.CashMovement.
//
// Rules:
//   - at most one open session per (establishment, register): enforced by
//     the unique partial index cash_sessions_open_uidx; a violation is
//     ErrSessionAlreadyOpen (409 cash_session_already_open);
//   - movements and payments on a closed session fail with ErrSessionClosed
//     (409 cash_session_closed);
//   - expected totals per method = opening float (cash) + succeeded payments
//     of the session (refunds negative) + movements in - movements out
//     (cash); difference = sum(counted) - sum(expected), stored on close
//     with a conditional UPDATE (closed_at IS NULL, version = If-Match);
//   - every write records the audit log and an outbox event
//     (cash_session.opened, cash_movement.recorded, cash_session.closed,
//     payload event.CashChange) in the same transaction.
//
// The session's payments are read with store queries on payments
// (cash_session_id): cash does not depend on the payment package. It
// implements payment.CashDesk (SessionForPayment).
//
// Permissions: auth.PermCashOperate for every operation; history reads also
// accept auth.PermReportsRead.
package cash
