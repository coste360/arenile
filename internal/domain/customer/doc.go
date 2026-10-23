// Package customer manages the customers of an establishment (spec 6.5, 23)
// and their credit ledger (spec 13.4): CRUD, search by name, phone, email
// and tag, GDPR export and anonymization.
//
// Ownership (M3): the "customers" builder owns this package, the handlers in
// internal/http/admin/customers.go and the repository functions in
// internal/store/customer.go. Models live in internal/store/models_customer.go.
//
// Permissions (spec 16.1, checked here, never only in the handler): reads
// require auth.PermBookingsRead; create, update, export and anonymize
// require auth.PermBookingsWrite.
//
// Rules:
//   - email is stored lowercase, phone in E.164; search_text (lowercase,
//     accent-free "last first email phone") is recomputed on every write and
//     searched with LIKE on both dialects;
//   - updates use optimistic concurrency (customers.version, If-Match);
//   - anonymization replaces first/last name, email, phone, notes, tags and
//     marketing consent irreversibly, sets anonymized_at, keeps bookings and
//     payments (accounting) and emits event.CustomerAnonymized; an
//     anonymized customer cannot be updated (configerr.ErrConflict);
//   - every write records the audit log and an outbox event
//     (event.CustomerCreated / CustomerUpdated / CustomerAnonymized, payload
//     event.CustomerChange: ids only, never personal data) in the same
//     transaction; logs never contain emails or phones (spec 22, 23).
//
// Ports implemented for other packages (see their ports.go):
//   - booking.Customers: Resolve (existing customer or inline creation inside
//     the booking transaction);
//   - payment.Credits and pass.Credits: GrantCredit / CreditBalance on the
//     customer_credits ledger.
package customer
