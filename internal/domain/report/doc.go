// Package report computes the manager reports of spec 15.3 and the
// year-over-year comparison (spec 28, M5): occupancy, revenue, bookings by
// channel and lead time, cancellations and no-shows, season passes and
// release resales, top customers, open balances, a KPI summary, and the CSV
// export of each.
//
// Ownership (M5A): the "reports backend" builder owns this package, the
// handlers in internal/http/admin/reports.go and the read-only queries in
// internal/store/report.go (new file). The frontend lives in
// web/admin/src/features/reports.
//
// # Read model, no writes
//
// Reports are plain SQL aggregations over the operational tables
// (bookings, booking_lines, spot_allocations, season_releases, payments,
// spots, zones, closure_days, seasons): nothing is materialised, nothing is
// written, no audit or outbox. Every query filters on establishment_id from
// the store.EstablishmentScope returned by auth.Principal.Authorize with
// auth.PermReportsRead (API keys need the reports.read scope). Queries use
// only the SQL subset common to SQLite and PostgreSQL; grouping by week or
// month and other calendar arithmetic is done in Go on calendar.Date (the
// stored dates are already YYYY-MM-DD in the establishment timezone; sale
// dates come from instants converted with the establishment timezone).
//
// # Units and definitions (shared with the OpenAPI descriptions)
//
//   - Money is money.Cents (int64, VAT included); ratios are basis points
//     (int64, 10000 = 100%) computed with Ratio (truncated), never floats.
//   - Occupancy unit = (spot, date, slot AM/PM). Capacity = active spots of
//     the establishment maps x open days (inside a season, not a closure
//     day) x 2, minus blocked units. Occupied = booking and season
//     allocations, minus released-not-resold season units; holds never count.
//   - Revenue basis "collected": succeeded payments by business_date, refunds
//     negative, VAT split from payments.vat_breakdown. Basis "booked":
//     bookings in confirmed, checked_in, completed or no_show by the date of
//     confirmed_at in the establishment timezone, from the current price
//     snapshot (requote snapshot when present).
//   - Bookings report: sale date = created_at in the establishment timezone;
//     draft, held, expired and rejected are excluded. Lead time = starts_on
//     minus sale date, bucketed by LeadTimeBucketOf.
//   - Cancellations: bookings whose starts_on is in the period.
//   - Top customers expose only the display name (first and last name);
//     anonymised customers are labelled "Cliente anonimizzato". Never select
//     email, phone, address or notes in this package (spec 23).
//   - Year comparison: the same calendar span of the previous year (or of
//     compare_year), see PreviousSpan; 29 February maps to 28 February.
//
// # Periods
//
// Every report takes a Period validated by NewPeriod: to >= from, at most
// MaxPeriodDays days. A longer span is a configerr validation error (400).
//
// # CSV
//
// CSVWriter writes the shared format (UTF-8 BOM, ';' separator, CRLF,
// decimal comma via FormatEuro, Italian headers), the same convention as
// the corrispettivi export of internal/domain/fiscal, so the files open
// correctly in Italian spreadsheet programs.
package report
