// Package booking holds the booking domain: lifecycle statuses and their
// allowed transitions (spec 10.2), and the booking service that performs each
// transition together with the audit log entry and the outbox event in the same
// transaction.
//
// status.go is pure: it only knows which status changes are legal. The service
// calls Transition before writing, so an illegal change is refused with
// ErrInvalidTransition before touching the database.
package booking
