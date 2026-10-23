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
// # Architecture: functional core, imperative shell
//
// Service (service.go) authorises, validates and loads; the builders
// (occupancy.go, revenue.go, bookings.go, customers.go, comparison.go) are
// pure functions over the rows of the store and are table- and
// property-tested without a database; export.go formats the CSV files.
// Reports are plain SQL aggregations over the operational tables
// (internal/store/report.go): nothing is materialised, nothing is written,
// no audit or outbox. Every query filters on establishment_id from the
// store.EstablishmentScope returned by auth.Principal.Authorize with
// auth.PermReportsRead (owner, manager and viewer, spec 16.1; API keys need
// the reports.read scope). Queries use only the SQL subset common to SQLite
// and PostgreSQL and aggregate in SQL where no calendar arithmetic or JSON
// is involved; a report runs 1 to 6 data queries, the summary 10 and the
// comparison 18 (two spans), never one round trip per row. A whole season of a 300-spot lido (about 20k
// bookings, 60k allocations) answers in well under a second on both
// dialects (TestReportService_SeasonPerformance).
//
// Time: stored dates (starts_on, allocation dates, business_date) are
// already calendar dates of the establishment; instants (created_at,
// confirmed_at) are selected through a UTC half-open window [local
// midnight of from, local midnight after to) and converted to local dates
// with calendar.In, so DST days have 23 or 25 hours and nothing leaks across
// midnight.
//
// # Units and definitions (shared with the OpenAPI descriptions)
//
//   - Money is money.Cents (int64, VAT included); ratios are basis points
//     (int64, 10000 = 100%) computed with Ratio (truncated), never floats.
//     Averages are truncated integer divisions.
//   - Occupancy unit = (spot, date, slot AM/PM). Capacity = active spots of
//     the establishment (optionally of one zone or map) x open half-days
//     (inside any season, not closed by a closure day; a closure may close
//     only one slot) minus the blocked units (spot_blocks allocations).
//     Occupied = booking and season allocations on open half-days. A season
//     release deletes the season allocation (ADR 0010), so a resold unit is
//     a booking allocation and a free released unit only counts in
//     released_units. Holds and pending bookings never count. Current spot
//     state is used (no history of deactivated spots).
//   - Sold bookings: every status except draft, held, expired and rejected.
//     Booked statuses (booked revenue, open balances, passes sold):
//     confirmed, checked_in, completed, no_show.
//   - Revenue basis "collected": succeeded payments by business_date,
//     refunds negative (RevenueRow.Refunds is their positive total, already
//     subtracted from Gross); channel of the linked booking, "counter" for
//     sales without a booking. VAT from payments.vat_breakdown, split per
//     cell (row, business date, rate, method) with money.VATSplit
//     (half-even): grouped by time, method or rate the report equals the
//     daily corrispettivi export to the cent (INV-16,
//     TestBuildRevenueCollected_ReconcilesWithCorrispettivi); grouped by
//     channel gross is exact and taxable/tax may differ by one cent per
//     split cell. Basis "booked": total_amount of the bookings in a booked
//     status by confirmation date, VAT split by the current price snapshot
//     (payment.SplitBySnapshot); pack = line total minus line extras,
//     extra = booking_line_extras totals, each at its catalog VAT rate (or
//     the default), before booking-level discounts and rounding.
//     payment_method needs collected; pack and extra need booked.
//   - Bookings report: sold bookings by sale date (created_at in the
//     establishment timezone). Lead time = starts_on minus sale date,
//     negative values count as 0 (same day); LeadTimeBucketOf buckets it.
//   - Cancellations: sold bookings whose starts_on is in the period (the
//     denominator); lost value = total - net paid of the cancelled ones
//     (the penalty retained is not lost); refunded = their succeeded
//     refunds.
//   - Season passes: passes in a booked status confirmed in the period
//     (renewals = renewal_of_booking_id set); releases (not revoked) dated
//     in the period, resold when a booking allocation now holds the unit;
//     resale revenue prorates the resold line total over its allocated
//     units (remainder cents to the earliest units); credits =
//     season_releases.credit_amount.
//   - Top customers: sold bookings starting in the period; bookings excludes
//     cancelled ones, revenue = net paid amount (paid_amount, INV-22),
//     spot-days = distinct (spot, date) allocated in the period. Only the
//     display name (first and last name) is exposed; anonymised customers
//     are labelled "Cliente anonimizzato". Never select email, phone,
//     address or notes in this package (spec 23).
//   - Open balances: bookings in a booked status starting in the period
//     with total_amount > paid_amount.
//   - Year comparison: ComparisonSpan with align "calendar" (default, the
//     same day and month of the previous year or of compare_year,
//     PreviousSpan; 29 February maps to 28 February) or "weekday" (same
//     length, starting on the same weekday nearest to the calendar start:
//     364 days back for the previous year, WeekdaySpan). Both spans are
//     measured with the summary definitions; the series aligns day i of the
//     current span with day i of the previous one and adds an extra
//     previous day (leap years) to the last bucket, so the series always
//     sums to the metrics. delta_bp is absent when the previous value is 0.
//   - Test bookings (spec 15.4) do not exist yet; once they do, every
//     query here must exclude them.
//
// # Periods
//
// Every report takes a Period validated by NewPeriod: to >= from, at most
// MaxPeriodDays days. A longer span is a configerr validation error (400).
//
// # CSV
//
// CSVWriter writes the shared format (UTF-8 BOM, ';' separator, CRLF,
// decimal comma via FormatEuro, percentages via FormatBP, dates dd/mm/yyyy,
// Italian headers), the same convention as the corrispettivi export of
// internal/domain/fiscal, so the files open correctly in Italian
// spreadsheet programs. Free text from the database is neutralised against
// formula injection. Row labels are Italian (the source language); the
// stable keys let a client translate them.
package report
