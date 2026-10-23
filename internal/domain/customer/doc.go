// Package customer manages the customers of an establishment (spec 6.5, 23)
// and their credit ledger (spec 13.4): CRUD, search by name, phone, email
// and tag, dedupe hint, GDPR export and anonymization.
//
// Ownership (M3): the "customers" builder owns this package, the handlers in
// internal/http/admin/customers.go and the repository functions in
// internal/store/customer.go. Models live in internal/store/models_customer.go.
//
// Structure (functional core / imperative shell):
//   - normalize.go is pure: validation and normalisation of the input
//     (E.164 phone with +39 default, lowercase email, tags, consent), the
//     search_text builder, query terms and accent folding;
//   - customers.go and service.go are the shell: authorization, one
//     transaction per use case (unit of work), repository calls, audit log
//     and outbox event in the same transaction (INV-18).
//
// Permissions (spec 16.1, checked here, never only in the handler): reads
// require auth.PermBookingsRead; create, update, export and anonymize
// require auth.PermBookingsWrite (owner, manager, cashier; viewer and beach
// staff read only).
//
// Rules:
//   - email is stored lowercase, phone in E.164 (separators removed, "00"
//     -> "+", national numbers get +39). search_text is the lowercase,
//     accent-free "last first email phone national-phone" followed by one
//     "|tag|" marker per tag; it is recomputed on every write and searched
//     with LIKE ... ESCAPE on both dialects. Free text terms must all match;
//     a phone-like query is reduced to its digits; the tag filter matches a
//     "|tag|" marker exactly. Lists are ordered by search_text (last name,
//     first name) with a keyset cursor on (search_text, id);
//   - dedupe hint: Create refuses a customer whose email or phone matches a
//     non-anonymized customer of the same establishment with
//     *DuplicateError (409, detail "existing_customer_id=<id>") unless force
//     is set. Inline creation from a booking (Resolve) never blocks;
//   - updates use optimistic concurrency (customers.version, If-Match) and
//     PUT semantics (an empty field clears it, a missing consent withdraws
//     it; an unchanged consent keeps its timestamp);
//   - anonymization replaces first/last name, email, phone, notes, tags and
//     marketing consent irreversibly, sets anonymized_at, clears the
//     free-text notes and manage token of the customer's bookings, keeps
//     bookings, lines, payments and credits (accounting) and emits
//     event.CustomerAnonymized; an anonymized customer cannot be updated
//     or attached to a new booking (ErrAnonymized, 409);
//   - every write records the audit log and an outbox event
//     (event.CustomerCreated / CustomerUpdated / CustomerAnonymized, payload
//     event.CustomerChange: ids only) in the same transaction. Audit diffs
//     carry field NAMES only, never values: audit_log is insert-only
//     (INV-30) and must hold no personal data after an anonymization. The
//     GDPR export is audited as customer.exported. Logs never contain names,
//     emails or phones (spec 22, 23).
//
// Ports implemented for other packages (see their ports.go):
//   - booking.Customers: Resolve (existing customer or inline creation inside
//     the booking transaction; validation fields are prefixed "customer.");
//   - payment.Credits and pass.Credits: GrantCredit / CreditBalance on the
//     customer_credits ledger (balance = sum of the non-expired movements).
//
// Integrator notes: the "force" flag of the dedupe hint is read from the
// query string (?force=true) and the existing id is only in the problem
// detail; both should be declared in api/openapi.yaml (createCustomer
// parameter and a Problem extension existing_customer_id). Automatic
// anonymization after N years (spec 23, retention) is not implemented yet.
package customer
