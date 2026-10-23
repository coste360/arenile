// Package fiscal is the fiscal module (spec 14.2): the corrispettivi export
// (daily or monthly takings by VAT rate and payment method, JSON and CSV)
// and the tracking of fiscal documents issued by the configured
// ext.FiscalDriver ("none" by default, builtin.NoneFiscal).
//
// Ownership (M3): the "cash" builder owns this package (with cash), the
// handlers in internal/http/admin/fiscal.go and the repository functions in
// internal/store/fiscal.go. Model: store.FiscalDocument.
//
// Corrispettivi (functional core, corrispettivi.go): BuildReport aggregates
// succeeded payments by business_date (day or month), VAT rate and method
// using the gross per rate frozen in payments.vat_breakdown (refunds
// negative; payments without a split go to the establishment default
// rate); each row is split with money.VATSplit (half-even) so that taxable
// + tax = gross (INV-16); per-rate totals are the sums of their rows and
// the grand total always equals payments - refunds of the range
// (reconciliation, property-tested). WriteCSV produces the accountant file:
// UTF-8 with BOM, ';' separator, euro amounts with decimal comma, Italian
// headers and method labels, per-rate totals and the grand total.
//
// Fiscal documents (strategy/registry): RecordPayment runs in the payment
// transaction. It reads the driver code from establishments.settings
// (settings.fiscal.driver, default "none"), assigns the next daily sequence
// number of the establishment (YYYYMMDD-NNNN, serialized with
// store.LockFiscalSequence) and writes a fiscal_documents row plus
// fiscal_document.recorded. The "none" driver performs no I/O, so it is
// called inline and the document is stored as issued with the internal
// number (tracking only: the operator issues receipts with their own
// registered cash register). Any other driver leaves the document pending;
// the job of Jobs (IssuePending) calls the device after commit, never
// inside a transaction, and stores the outcome with a conditional UPDATE.
// It implements payment.Fiscal.
//
// Permissions: the export and the document list require
// auth.PermReportsRead.
package fiscal
