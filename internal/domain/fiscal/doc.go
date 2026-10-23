// Package fiscal is the fiscal module (spec 14.2): the corrispettivi export
// (daily or monthly takings by VAT rate and payment method, JSON and CSV)
// and the tracking of fiscal documents issued by the configured
// ext.FiscalDriver ("none" by default, builtin.NoneFiscal).
//
// Ownership (M3): the "cash" builder owns this package (with cash), the
// handlers in internal/http/admin/fiscal.go and the repository functions in
// internal/store/fiscal.go. Model: store.FiscalDocument.
//
// Corrispettivi: computed from payments (status succeeded) by business_date
// and method using the frozen payments.vat_breakdown (refunds negative);
// per rate taxable + tax = gross (INV-16). CSV uses ';' as separator, amounts
// in euro with comma decimals (it-IT, for the accountant), UTF-8 with BOM.
//
// Fiscal documents: RecordPayment runs in the payment transaction. With the
// driver "none" nothing is issued and no row is required (the operator uses
// their own registered cash register); other drivers write a
// fiscal_documents row (pending), and device I/O happens after commit
// (status issued/failed updated by a job), never inside the transaction.
// It implements payment.Fiscal.
//
// Permissions: the export and the document list require
// auth.PermReportsRead.
package fiscal
