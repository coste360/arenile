package admin

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Reports of spec 15.3 and the year comparison (M5A). Every handler builds
// report.Period with report.NewPeriod(params.From, params.To), calls the
// service, then answers JSON or, when params.Format is csv (or, without
// format, the Accept header contains text/csv), the CSV export written by
// report.Write<Name>CSV with Content-Type "text/csv; charset=utf-8",
// Content-Disposition attachment "<report>_<from>_<to>.csv" and
// Cache-Control no-store (as GetCorrispettivi in fiscal.go).

// wantsCSV reports whether the client asked for the CSV export.
func wantsCSV(r *http.Request, format *api.ReportFormat) bool {
	if format != nil {
		return *format == api.ReportFormatCSV
	}
	return strings.Contains(r.Header.Get("Accept"), "text/csv")
}

// writeReport answers res as JSON (toAPI) or CSV (writeCSV).
func writeReport[R any](h *Handlers, w http.ResponseWriter, r *http.Request, format *api.ReportFormat, name string, p report.Period,
	res *R, toAPI func(*R) any, writeCSV func(io.Writer, *R) error,
) {
	if !wantsCSV(r, format) {
		writeJSON(w, http.StatusOK, toAPI(res))
		return
	}
	var buf bytes.Buffer
	if err := writeCSV(&buf, res); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	file := name + "_" + p.From.String() + "_" + p.To.String() + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+file+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// reportPeriod validates from/to, answering 400 on failure.
func (h *Handlers) reportPeriod(w http.ResponseWriter, r *http.Request, from, to calendar.Date) (report.Period, bool) {
	p, err := report.NewPeriod(from, to)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return report.Period{}, false
	}
	return p, true
}

// GetReportSummary implements GET /api/admin/v1/establishments/{establishmentId}/reports/summary.
func (h *Handlers) GetReportSummary(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetReportSummaryParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	res, err := h.Reports.Summary(r.Context(), principalOf(r), establishmentID, p)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "riepilogo", p, res, func(s *report.Summary) any { return ReportSummaryToAPI(s) }, report.WriteSummaryCSV)
}

// GetOccupancyReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/occupancy.
func (h *Handlers) GetOccupancyReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetOccupancyReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	q := report.OccupancyQuery{Period: p}
	if params.GroupBy != nil {
		q.GroupBy = report.OccupancyGroupBy(*params.GroupBy)
	}
	if params.ZoneId != nil {
		q.ZoneID = *params.ZoneId
	}
	if params.MapId != nil {
		q.MapID = *params.MapId
	}
	res, err := h.Reports.Occupancy(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "occupazione", p, res, func(o *report.Occupancy) any { return OccupancyReportToAPI(o) }, report.WriteOccupancyCSV)
}

// GetRevenueReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/revenue.
func (h *Handlers) GetRevenueReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetRevenueReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	q := report.RevenueQuery{Period: p}
	if params.GroupBy != nil {
		q.GroupBy = report.RevenueGroupBy(*params.GroupBy)
	}
	if params.Basis != nil {
		q.Basis = report.RevenueBasis(*params.Basis)
	}
	res, err := h.Reports.Revenue(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "ricavi", p, res, func(x *report.Revenue) any { return RevenueReportToAPI(x) }, report.WriteRevenueCSV)
}

// GetBookingsReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/bookings.
func (h *Handlers) GetBookingsReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetBookingsReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	var groupBy report.BookingsGroupBy
	if params.GroupBy != nil {
		groupBy = report.BookingsGroupBy(*params.GroupBy)
	}
	res, err := h.Reports.Bookings(r.Context(), principalOf(r), establishmentID, p, groupBy)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "prenotazioni", p, res, func(x *report.Bookings) any { return BookingsReportToAPI(x) }, report.WriteBookingsCSV)
}

// GetCancellationsReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/cancellations.
func (h *Handlers) GetCancellationsReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetCancellationsReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	var groupBy report.CancellationsGroupBy
	if params.GroupBy != nil {
		groupBy = report.CancellationsGroupBy(*params.GroupBy)
	}
	res, err := h.Reports.Cancellations(r.Context(), principalOf(r), establishmentID, p, groupBy)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "cancellazioni", p, res, func(x *report.Cancellations) any { return CancellationsReportToAPI(x) }, report.WriteCancellationsCSV)
}

// GetSeasonPassesReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/season-passes.
func (h *Handlers) GetSeasonPassesReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetSeasonPassesReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	var groupBy report.SeasonPassesGroupBy
	if params.GroupBy != nil {
		groupBy = report.SeasonPassesGroupBy(*params.GroupBy)
	}
	res, err := h.Reports.SeasonPasses(r.Context(), principalOf(r), establishmentID, p, groupBy)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "abbonamenti", p, res, func(x *report.SeasonPasses) any { return SeasonPassesReportToAPI(x) }, report.WriteSeasonPassesCSV)
}

// GetTopCustomersReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/top-customers.
func (h *Handlers) GetTopCustomersReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetTopCustomersReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	var sort report.TopCustomersSort
	if params.Sort != nil {
		sort = report.TopCustomersSort(*params.Sort)
	}
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
		if limit == 0 {
			limit = -1 // an explicit 0 is out of range, not the default
		}
	}
	res, err := h.Reports.TopCustomers(r.Context(), principalOf(r), establishmentID, p, sort, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "clienti", p, res, func(x *report.TopCustomers) any { return TopCustomersReportToAPI(x) }, report.WriteTopCustomersCSV)
}

// GetOpenBalancesReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/open-balances.
func (h *Handlers) GetOpenBalancesReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetOpenBalancesReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	res, err := h.Reports.OpenBalances(r.Context(), principalOf(r), establishmentID, p)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "saldi-aperti", p, res, func(x *report.OpenBalances) any { return OpenBalancesReportToAPI(x) }, report.WriteOpenBalancesCSV)
}

// GetYearComparisonReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/comparison.
func (h *Handlers) GetYearComparisonReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetYearComparisonReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	q := report.ComparisonQuery{Period: p}
	if params.CompareYear != nil {
		q.CompareYear = *params.CompareYear
	}
	if params.Granularity != nil {
		q.Granularity = report.Granularity(*params.Granularity)
	}
	if params.Align != nil {
		q.Align = report.Align(*params.Align)
	}
	res, err := h.Reports.Comparison(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "confronto", p, res, func(x *report.Comparison) any { return YearComparisonReportToAPI(x) }, report.WriteComparisonCSV)
}

// ---- mapping ------------------------------------------------------------------

func reportPeriodToAPI(p report.Period) api.ReportPeriod {
	return api.ReportPeriod{From: p.From, To: p.To, Days: p.Days()}
}

// ReportSummaryToAPI maps the KPI summary.
func ReportSummaryToAPI(s *report.Summary) api.ReportSummary {
	return api.ReportSummary{
		Period: reportPeriodToAPI(s.Period), OccupancyBp: s.OccupancyBP, OccupiedUnits: s.OccupiedUnits, CapacityUnits: s.CapacityUnits,
		RevenueCollected: int64(s.RevenueCollected), RevenueBooked: int64(s.RevenueBooked), Bookings: s.Bookings,
		AverageBookingValue: int64(s.AverageBookingValue), Cancellations: s.Cancellations, NoShows: s.NoShows,
		SeasonPassesSold: s.SeasonPassesSold, OpenBalanceTotal: int64(s.OpenBalanceTotal), OpenBalanceBookings: s.OpenBalanceBookings,
		GeneratedAt: s.GeneratedAt,
	}
}

func occupancyRowToAPI(r report.OccupancyRow) api.OccupancyRow {
	out := api.OccupancyRow{
		Key: r.Key, Label: r.Label, CapacityUnits: r.CapacityUnits, OccupiedUnits: r.OccupiedUnits, SeasonUnits: r.SeasonUnits,
		ReleasedUnits: r.ReleasedUnits, BlockedUnits: r.BlockedUnits, OccupancyBp: r.OccupancyBP,
	}
	if r.ZoneID != "" {
		z := r.ZoneID
		out.ZoneId = &z
	}
	return out
}

// OccupancyReportToAPI maps the occupancy report.
func OccupancyReportToAPI(o *report.Occupancy) api.OccupancyReport {
	out := api.OccupancyReport{
		Period: reportPeriodToAPI(o.Period), GroupBy: api.OccupancyGroupBy(o.GroupBy),
		Rows: make([]api.OccupancyRow, 0, len(o.Rows)), Totals: occupancyRowToAPI(o.Totals),
	}
	for _, r := range o.Rows {
		out.Rows = append(out.Rows, occupancyRowToAPI(r))
	}
	return out
}

func revenueRowToAPI(r report.RevenueRow) api.RevenueRow {
	return api.RevenueRow{
		Key: r.Key, Label: r.Label, Count: r.Count, Gross: int64(r.Gross), Taxable: int64(r.Taxable), Tax: int64(r.Tax),
		Refunds: int64(r.Refunds),
	}
}

// RevenueReportToAPI maps the revenue report.
func RevenueReportToAPI(x *report.Revenue) api.RevenueReport {
	out := api.RevenueReport{
		Period: reportPeriodToAPI(x.Period), GroupBy: api.RevenueGroupBy(x.GroupBy), Basis: api.RevenueBasis(x.Basis),
		Rows: make([]api.RevenueRow, 0, len(x.Rows)), Totals: revenueRowToAPI(x.Totals),
	}
	for _, r := range x.Rows {
		out.Rows = append(out.Rows, revenueRowToAPI(r))
	}
	return out
}

func bookingsRowToAPI(r report.BookingsRow) api.BookingsReportRow {
	return api.BookingsReportRow{
		Key: r.Key, Label: r.Label, Bookings: r.Bookings, Value: int64(r.Value), AverageValue: int64(r.AverageValue),
		ShareBp: r.ShareBP, AverageLeadDays: r.AverageLeadDays,
	}
}

// BookingsReportToAPI maps the bookings report.
func BookingsReportToAPI(x *report.Bookings) api.BookingsReport {
	out := api.BookingsReport{
		Period: reportPeriodToAPI(x.Period), GroupBy: api.BookingsReportGroupBy(x.GroupBy),
		Rows: make([]api.BookingsReportRow, 0, len(x.Rows)), Totals: bookingsRowToAPI(x.Totals),
	}
	for _, r := range x.Rows {
		out.Rows = append(out.Rows, bookingsRowToAPI(r))
	}
	return out
}

func cancellationsRowToAPI(r report.CancellationsRow) api.CancellationsRow {
	return api.CancellationsRow{
		Key: r.Key, Label: r.Label, Bookings: r.Bookings, Cancelled: r.Cancelled, NoShows: r.NoShows, CancelledBp: r.CancelledBP,
		NoShowBp: r.NoShowBP, LostValue: int64(r.LostValue), Refunded: int64(r.Refunded),
	}
}

// CancellationsReportToAPI maps the cancellations report.
func CancellationsReportToAPI(x *report.Cancellations) api.CancellationsReport {
	out := api.CancellationsReport{
		Period: reportPeriodToAPI(x.Period), GroupBy: api.CancellationsGroupBy(x.GroupBy),
		Rows: make([]api.CancellationsRow, 0, len(x.Rows)), Totals: cancellationsRowToAPI(x.Totals),
	}
	for _, r := range x.Rows {
		out.Rows = append(out.Rows, cancellationsRowToAPI(r))
	}
	return out
}

func seasonPassesRowToAPI(r report.SeasonPassesRow) api.SeasonPassesRow {
	return api.SeasonPassesRow{
		Key: r.Key, Label: r.Label, ReleasedUnits: r.ReleasedUnits, ResoldUnits: r.ResoldUnits, ResaleBp: r.ResaleBP,
		ResaleRevenue: int64(r.ResaleRevenue), CreditsIssued: int64(r.CreditsIssued),
	}
}

// SeasonPassesReportToAPI maps the season passes report.
func SeasonPassesReportToAPI(x *report.SeasonPasses) api.SeasonPassesReport {
	out := api.SeasonPassesReport{
		Period: reportPeriodToAPI(x.Period), GroupBy: api.SeasonPassesGroupBy(x.GroupBy), PassesSold: x.PassesSold,
		PassesValue: int64(x.PassesValue), Renewals: x.Renewals, Rows: make([]api.SeasonPassesRow, 0, len(x.Rows)),
		Totals: seasonPassesRowToAPI(x.Totals),
	}
	for _, r := range x.Rows {
		out.Rows = append(out.Rows, seasonPassesRowToAPI(r))
	}
	return out
}

// TopCustomersReportToAPI maps the top customers report.
func TopCustomersReportToAPI(x *report.TopCustomers) api.TopCustomersReport {
	out := api.TopCustomersReport{Period: reportPeriodToAPI(x.Period), Sort: api.TopCustomersSort(x.Sort), Rows: make([]api.TopCustomerRow, 0, len(x.Rows))}
	for _, r := range x.Rows {
		out.Rows = append(out.Rows, api.TopCustomerRow{
			Rank: r.Rank, CustomerId: r.CustomerID, DisplayName: r.DisplayName, Bookings: r.Bookings, SpotDays: r.SpotDays,
			Revenue: int64(r.Revenue), LastBookingOn: r.LastBookingOn,
		})
	}
	return out
}

// OpenBalancesReportToAPI maps the open balances report.
func OpenBalancesReportToAPI(x *report.OpenBalances) api.OpenBalancesReport {
	out := api.OpenBalancesReport{Period: reportPeriodToAPI(x.Period), Rows: make([]api.OpenBalanceRow, 0, len(x.Rows)), TotalBalance: int64(x.TotalBalance)}
	for _, r := range x.Rows {
		out.Rows = append(out.Rows, api.OpenBalanceRow{
			BookingId: r.BookingID, Code: r.Code, CustomerId: r.CustomerID, DisplayName: r.DisplayName, Status: api.BookingStatus(r.Status),
			StartsOn: r.StartsOn, EndsOn: r.EndsOn, TotalAmount: int64(r.Total), PaidAmount: int64(r.Paid), BalanceDue: int64(r.Balance),
		})
	}
	return out
}

// YearComparisonReportToAPI maps the year comparison.
func YearComparisonReportToAPI(x *report.Comparison) api.YearComparisonReport {
	align := api.ComparisonAlign(x.Align)
	out := api.YearComparisonReport{
		Current: reportPeriodToAPI(x.Current), Previous: reportPeriodToAPI(x.Previous), Granularity: api.ComparisonGranularity(x.Granularity),
		Align: &align, Metrics: make([]api.ComparisonMetric, 0, len(x.Metrics)), Series: make([]api.ComparisonPoint, 0, len(x.Series)),
	}
	for _, m := range x.Metrics {
		out.Metrics = append(out.Metrics, api.ComparisonMetric{
			Metric: api.ComparisonMetricKey(m.Metric), Unit: api.ComparisonUnit(m.Unit), Current: m.Current, Previous: m.Previous,
			Delta: m.Delta, DeltaBp: m.DeltaBP,
		})
	}
	for _, p := range x.Series {
		out.Series = append(out.Series, api.ComparisonPoint{
			Key: p.Key, Label: p.Label, CurrentFrom: p.CurrentFrom, PreviousFrom: p.PreviousFrom,
			CurrentRevenue: int64(p.CurrentRevenue), PreviousRevenue: int64(p.PreviousRevenue),
			CurrentOccupancyBp: p.CurrentOccupancyBP, PreviousOccupancyBp: p.PreviousOccupancyBP,
			CurrentBookings: p.CurrentBookings, PreviousBookings: p.PreviousBookings,
		})
	}
	return out
}
