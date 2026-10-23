// Package importer imports customers and season passes from CSV files (ADR
// 0019, feature F) to help managers switching from other software.
//
// Ownership (deferred features, builder F2 "imports"): this package, the
// handlers in internal/http/admin/imports.go, the repository functions on
// store.ImportJob (new file internal/store/importjob.go) and the admin
// feature web/admin/src/features/imports.
//
// # Flow
//
//  1. createImportJob: text/csv body (max 5 MB, 20000 rows, UTF-8 with or
//     without BOM, ';' or ',' detected), stored in import_jobs.content with
//     its SHA-256; status uploaded. Nothing else is written.
//  2. previewImportJob (dry run, repeatable): parses and validates every row
//     with ImportOptions and computes create/update/skip against the
//     current data; stores the ImportReport (row, column, code only: never
//     the values) and moves to validated.
//  3. applyImportJob: one transaction, all or nothing; any error rolls back
//     and the job becomes failed with the report. Customers go through the
//     customer service (encryption, search hashes, GDPR consent fields with
//     source "import" and the import date: marketing consent is never
//     presumed); season passes go through internal/domain/booking (DB
//     allocations, audit, outbox; test flag when test mode is on).
//     Idempotent: rows matched by ImportOptions.match_by are updated or
//     skipped; a second apply of an applied job returns it; the same file
//     already applied answers 409 import_already_applied (unique index).
//  4. discardImportJob or the retention job (7 days) clears content.
//
// Column headers (Italian, case-insensitive, English aliases accepted) are
// documented by getImportTemplate; the docs site page "Importare i dati"
// lists them with an example file.
package importer
